package main

import (
	"fmt"
	"net/http"
)

// wishlist:
// - controller should be interface based / class based?
// - headers must be automatic
// - must deal with usual response and streams
// - for streams, should deal with the hub in the background (global AND instances!)
// - authorized API keys checking

func AddRouteHandler(app *App, methods []string, route string, controller func(w http.ResponseWriter, r *http.Request, app *App)) {
	var mux *http.ServeMux
	mux = app.mux
	fmt.Println(mux)
}

func AddRouteStreamHandler(app *App, methods []string, route string, controller func(w http.ResponseWriter, r *http.Request, app *App)) {
}

/*

/log
/log/ ← add urlArg?

// Route Type: custom
func(w http.ResponseWriter, r *http.Request, urlArgs string, app *App)

// Route Type: stream (with optionnal target)
func(log *Log, r *http.Request, urlArgs string, app *App)

*/
