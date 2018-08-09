package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Xfennec/mulch"
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
	IsRestricted bool
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

func routeStreamPrepare(w http.ResponseWriter, r *http.Request, hub *Hub) (*Log, *HubClient, error) {
	cn, ok := w.(http.CloseNotifier)
	if !ok {
		return nil, nil, errors.New("CloseNotifier failed")
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, nil, errors.New("Flusher failed")
	}

	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	enc := json.NewEncoder(w)

	// plug ourselves into the hub
	client := hub.Register("me", mulch.MessageNoTarget)
	go func() {
		for {
			select {
			case <-cn.CloseNotify():
				client.Unregister()
				return
			// TODO: make timeout configurable
			case <-time.After(10 * time.Second):
				// Keep-alive
				m := mulch.NewMessage(mulch.MessageNoop, mulch.MessageNoTarget, "")

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
	}()
	return NewLog(mulch.MessageNoTarget, hub), client, nil
}

// AddRoute adds a new route to the muxer
func AddRoute(route *Route, app *App) error {

	if route.Path == "" {
		return errors.New("route path is not set")
	}

	// remove * (if any) from route.Path end
	route.Path = strings.TrimRight(route.Path, "*")

	app.Mux.HandleFunc(route.Path, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")

		if route.IsRestricted {
			// TODO: API key checking (or a better challenge-based auth)
		}

		if !isRouteMethodAllowed(r.Method, route.Methods) {
			http.Error(w, "Invalid request method.", 405)
			return
		}

		// extract relative path
		subPath := r.URL.Path[len(route.Path):]

		request := &Request{
			Route:    route,
			SubPath:  subPath,
			HTTP:     r,
			Response: w,
		}

		// prepare Stream if needed
		if route.Type == RouteTypeStream {
			stream, hubClient, err := routeStreamPrepare(w, r, app.Hub)
			if err != nil {
				errMsg := fmt.Sprintf("stream preparation: %s", err)
				app.Log.Error(errMsg)
				http.Error(w, errMsg, 500)
				return
			}
			request.Stream = stream
			request.HubClient = hubClient
		}

		route.Handler(request)
	})
	return nil
}

// wishlist:
// - authorized API keys checking

/*func AddRouteHandler(app *App, methods []string, route string, controller func(w http.ResponseWriter, r *http.Request, app *App)) {
	var mux *http.ServeMux
	mux = app.mux
	fmt.Println(mux)
}

func AddRouteStreamHandler(app *App, methods []string, route string, controller func(w http.ResponseWriter, r *http.Request, app *App)) {
}*/

/*

/log
/log/ ← add urlArg?

// Route Type: custom
func(w http.ResponseWriter, r *http.Request, urlArgs string, app *App)

// Route Type: stream (with optionnal target)
func(log *Log, r *http.Request, urlArgs string, app *App)


*/
