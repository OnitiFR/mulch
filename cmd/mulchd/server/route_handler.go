package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Xfennec/mulch/common"
)

// Route types
const (
	RouteTypeCustom = 0
	RouteTypeStream = 1
)

// Route describes a route to a handler
type Route struct {
	Methods      []string
	Path         string
	Type         int
	Public       bool
	NoProtoCheck bool
	Handler      func(*Request)
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

	if route.Path == "" {
		return errors.New("route path is not set")
	}

	// remove * (if any) from route.Path end
	route.Path = strings.TrimRight(route.Path, "*")

	app.Mux.HandleFunc(route.Path, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")

		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		app.Log.Tracef("API call: %s %s %s", ip, r.Method, route.Path)

		if route.NoProtoCheck == false {
			clientProto, _ := strconv.Atoi(r.FormValue("protocol"))
			if clientProto != ProtocolVersion {
				errMsg := fmt.Sprintf("Protocol mismatch, server requires version %d", ProtocolVersion)
				app.Log.Errorf("%d: %s", 400, errMsg)
				http.Error(w, errMsg, 400)
				return
			}
		}

		if !route.Public {
			// TODO: API key checking (or a better challenge-based auth)
		}

		if !isRouteMethodAllowed(r.Method, route.Methods) {
			errMsg := fmt.Sprintf("Method was %s", r.Method)
			app.Log.Errorf("%d: %s", 405)
			http.Error(w, errMsg, 405)
			return
		}

		// extract relative path
		subPath := r.URL.Path[len(route.Path):]

		request := &Request{
			Route:    route,
			SubPath:  subPath,
			HTTP:     r,
			Response: w,
			App:      app,
		}

		switch route.Type {
		case RouteTypeStream:
			routeStreamHandler(w, r, request)
		case RouteTypeCustom:
			route.Handler(request)
		}
	})
	return nil
}

// SetTarget define or change the default target for the request, for both
// sending (Stream) and receiving (HubClient)
func (req *Request) SetTarget(target string) {
	req.Stream.SetTarget(target)
	req.HubClient.SetTarget(target)
}

// Responsef is a Printf lile helper for req.Response.Write
func (req *Request) Responsef(format string, args ...interface{}) {
	req.Response.Write([]byte(fmt.Sprintf(format, args...)))
}
