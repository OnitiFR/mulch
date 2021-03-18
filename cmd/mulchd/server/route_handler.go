package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/OnitiFR/mulch/cmd/mulch/client"
	"github.com/OnitiFR/mulch/common"
)

// Route types
const (
	RouteTypeCustom = 0
	RouteTypeStream = 1
)

// Route muxer
const (
	RouteInternal = "internal"
	RouteAPI      = "api"
)

// Route describes a route to a handler
type Route struct {
	Route        string
	Type         int
	Public       bool
	NoProtoCheck bool
	Handler      func(*Request)

	// decomposed Route
	method string
	path   string
}

// extract a generic parameter from the request (API key, protocol, etc)
// from the headers (new way, "Mulch-Name") or from FormValue (old way, "name")
func requestGetMulchParam(r *http.Request, name string) string {
	headerName := "Mulch-" + strings.Title(name)

	val := r.Header.Get(headerName)
	if val == "" {
		val = r.FormValue(name)
	}
	return val
}

func routeStreamHandler(request *Request) {
	w := request.Response
	r := request.HTTP
	ctx := r.Context()

	flusher, ok := w.(http.Flusher)
	if !ok {
		errMsg := fmt.Sprintf("stream preparation: Flusher failed")
		request.App.Log.Error(errMsg)
		http.Error(w, errMsg, 500)
		return
	}

	trace := false
	if r.FormValue("trace") == "true" {
		trace = true
	}

	// Note: starting from here, request parameters are no more available
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Content-Type", "application/x-ndjson")

	enc := json.NewEncoder(w)

	// plug ourselves into the hub
	// TODO: use API key owner instead of "me"
	tmpTarget := fmt.Sprintf(".tmp-%d", request.App.Rand.Int31())
	client := request.App.Hub.Register("me", tmpTarget, trace)

	request.Stream = NewLog(tmpTarget, request.App.Hub, request.App.LogHistory)
	request.HubClient = client

	closer := make(chan bool)
	go func() {
		request.Route.Handler(request)
		// let's ensure the last message have time to be flushed
		time.Sleep(time.Duration(100) * time.Millisecond)
		select {
		case closer <- true:
		case <-ctx.Done():
		}
	}()

	// allow request.Route.Handler to update headers
	request.WaitStream()
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for {
		select {
		case <-closer:
			client.Unregister()
			return
		case <-ctx.Done():
			client.Unregister()
			return
		// TODO: make timeout configurable
		case <-time.After(10 * time.Second):
			// Keep-alive
			m := common.NewMessage(common.MessageNoop, common.MessageNoTarget, "")

			err := enc.Encode(m)
			if err != nil {
				fmt.Println(err)
			}
			flusher.Flush()

		case msg := <-client.Messages:
			err := enc.Encode(msg)
			if err != nil {
				fmt.Println(err)
			}
			flusher.Flush()
		}
	}
}

// AddRoute adds a new route to the given route muxer
func (app *App) AddRoute(route *Route, routeMuxer string) error {

	if route.Route == "" {
		return errors.New("field Route is not set")
	}

	parts := strings.Split(route.Route, " ")
	if len(parts) != 2 {
		return fmt.Errorf("invalid Route '%s'", route.Route)
	}

	method := parts[0]
	switch method {
	case "GET":
	case "PUT":
	case "POST":
	case "DELETE":
	case "OPTIONS":
	case "HEAD":
	default:
		return fmt.Errorf("unsupported method '%s'", method)
	}

	// remove * (if any) at the end of route path
	path := strings.TrimRight(parts[1], "*")
	if path == "" {
		return errors.New("field Route path is invalid")
	}

	route.method = method
	route.path = path

	switch routeMuxer {
	case RouteInternal:
		app.routesInternal[path] = append(app.routesInternal[path], route)
	case RouteAPI:
		app.routesAPI[path] = append(app.routesAPI[path], route)
	default:
		return fmt.Errorf("unknown muxer '%s'", routeMuxer)
	}

	return nil
}

func (app *App) registerRouteHandlers(mux *http.ServeMux, inRoutes map[string][]*Route) {
	for _path, _routes := range inRoutes {
		// capture _path, _routes in the closure
		go func(path string, routes []*Route) {
			// look for duplicated methods in routes
			methods := make(map[string]bool)
			for _, route := range routes {
				_, exists := methods[route.method]
				if exists {
					log.Fatalf("router: duplicated method '%s' for path '%s'", route.method, path)
				}
				methods[route.method] = true
			}

			mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
				ip, _, _ := net.SplitHostPort(r.RemoteAddr)
				app.Log.Tracef("HTTP call: %s %s %s [%s]", ip, r.Method, path, r.UserAgent())

				var validRoute *Route
				for _, route := range routes {
					if route.method == r.Method {
						validRoute = route
					}
				}

				if validRoute == nil {
					errMsg := fmt.Sprintf("Method was %s for route %s", r.Method, path)
					app.Log.Errorf("%d: %s", http.StatusMethodNotAllowed, errMsg)
					http.Error(w, errMsg, http.StatusMethodNotAllowed)
					return
				}
				routeHandleFunc(validRoute, w, r, app)
			})
		}(_path, _routes)
	}
}

func routeHandleFunc(route *Route, w http.ResponseWriter, r *http.Request, app *App) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Latest-Known-Client-Version", client.Version)

	ip, _, _ := net.SplitHostPort(r.RemoteAddr)

	if !route.NoProtoCheck {
		clientProto, _ := strconv.Atoi(requestGetMulchParam(r, "protocol"))
		if clientProto != ProtocolVersion {
			errMsg := fmt.Sprintf("Protocol mismatch, server requires version %d", ProtocolVersion)
			app.Log.Errorf("%d: %s", 400, errMsg)
			http.Error(w, errMsg, 400)
			return
		}
	}

	// extract relative path
	subPath := r.URL.Path[len(route.path):]

	request := &Request{
		Route:           route,
		SubPath:         subPath,
		HTTP:            r,
		Response:        w,
		App:             app,
		startStreamChan: make(chan bool),
		streamStarted:   false,
	}

	if !route.Public {
		valid, key := app.APIKeysDB.IsValidKey(requestGetMulchParam(r, "key"))
		if !valid {
			errMsg := "invalid key"
			app.Log.Errorf("%d: %s", http.StatusForbidden, errMsg)
			http.Error(w, errMsg, http.StatusForbidden)
			return
		}
		request.APIKey = key
		app.Log.Tracef("API call: %s %s %s (key: %s)", ip, r.Method, route.path, key.Comment)
	} else {
		app.Log.Tracef("API call: %s %s %s", ip, r.Method, route.path)
	}

	switch route.Type {
	case RouteTypeStream:
		routeStreamHandler(request)
	case RouteTypeCustom:
		request.Stream = NewLog("", app.Hub, app.LogHistory)
		request.streamStarted = true
		route.Handler(request)
	}
}
