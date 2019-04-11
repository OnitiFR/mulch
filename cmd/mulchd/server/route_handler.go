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

	"github.com/OnitiFR/mulch/common"
)

// Route types
const (
	RouteTypeCustom = 0
	RouteTypeStream = 1
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

// Request describes a request and allows to build a response
type Request struct {
	Route     *Route
	SubPath   string
	HTTP      *http.Request
	Response  http.ResponseWriter
	App       *App
	Stream    *Log
	HubClient *HubClient
	APIKey    *APIKey
}

func isRouteMethodAllowed(method string, methods []string) bool {
	for _, m := range methods {
		if strings.ToUpper(m) == strings.ToUpper(method) {
			return true
		}
	}
	return false
}

func routeStreamHandler(w http.ResponseWriter, r *http.Request, request *Request) {
	cn, ok := w.(http.CloseNotifier)
	if !ok {
		errMsg := fmt.Sprintf("stream preparation: CloseNotifier failed")
		request.App.Log.Error(errMsg)
		http.Error(w, errMsg, 500)
		return
	}
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
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	enc := json.NewEncoder(w)

	// plug ourselves into the hub
	// TODO: use API key owner instead of "me"
	tmpTarget := fmt.Sprintf(".tmp-%d", request.App.Rand.Int31())
	client := request.App.Hub.Register("me", tmpTarget, trace)

	request.Stream = NewLog(tmpTarget, request.App.Hub)
	request.HubClient = client

	closer := make(chan bool)
	go func() {
		request.Route.Handler(request)
		// let's ensure the last message have time to be flushed
		time.Sleep(time.Duration(100) * time.Millisecond)
		closer <- true
	}()

	for {
		select {
		case <-closer:
			client.Unregister()
			return
		case <-cn.CloseNotify():
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
			break
		}
	}
}

// AddRoute adds a new route to the muxer
func (app *App) AddRoute(route *Route) error {

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

	app.routes[path] = append(app.routes[path], route)

	return nil
}

func (app *App) registerRouteHandlers() {
	for _path, _routes := range app.routes {
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

			app.Mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
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
					app.Log.Errorf("%d: %s", 405, errMsg)
					http.Error(w, errMsg, 405)
					return
				}
				routeHandleFunc(validRoute, w, r, app)
			})
		}(_path, _routes)
	}
}

func routeHandleFunc(route *Route, w http.ResponseWriter, r *http.Request, app *App) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")

	ip, _, _ := net.SplitHostPort(r.RemoteAddr)

	if route.NoProtoCheck == false {
		clientProto, _ := strconv.Atoi(r.FormValue("protocol"))
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
		Route:    route,
		SubPath:  subPath,
		HTTP:     r,
		Response: w,
		App:      app,
	}

	if route.Public == false {
		valid, key := app.APIKeysDB.IsValidKey(r.FormValue("key"))
		if valid == false {
			errMsg := "invalid key"
			app.Log.Errorf("%d: %s", 403, errMsg)
			http.Error(w, errMsg, 403)
			return
		}
		request.APIKey = key
		app.Log.Infof("API call: %s %s %s (key: %s)", ip, r.Method, route.path, key.Comment)
	} else {
		app.Log.Infof("API call: %s %s %s", ip, r.Method, route.path)
	}

	switch route.Type {
	case RouteTypeStream:
		routeStreamHandler(w, r, request)
	case RouteTypeCustom:
		route.Handler(request)
	}
}

// SetTarget define or change the default target for the request, for both
// sending (Stream) and receiving (HubClient)
func (req *Request) SetTarget(target string) {
	req.Stream.SetTarget(target)
	req.HubClient.SetTarget(target)
}

// Printf like helper for req.Response.Write
func (req *Request) Printf(format string, args ...interface{}) {
	req.Response.Write([]byte(fmt.Sprintf(format, args...)))
}

// Println like helper for req.Response.Write
func (req *Request) Println(message string) {
	req.Response.Write([]byte(fmt.Sprintf("%s\n", message)))
}
