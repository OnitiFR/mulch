package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"
)

type App struct {
	// config
	// TODO: deal with multiple connections!
	lvconn *LibvirtConnection
	hub    *Hub
	log    *Log
}

func NewApp() (*App, error) {
	uri := "qemu:///system"

	lc, err := NewLibvirtConnection(uri)
	if err != nil {
		return nil, err
	}

	hub := NewHub()
	go hub.Run()

	log := NewLog("", hub)

	app := &App{
		lvconn: lc,
		hub:    hub,
		log:    log,
	}

	app.log.Info(fmt.Sprintf("libvirt connection to '%s' OK", uri))

	// dirty log broadcast est
	go func() {
		rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
		for {
			delay := rnd.Intn(12000)
			time.Sleep(time.Duration(delay) * time.Millisecond)
			app.log.Info(fmt.Sprintf("Test %d", delay))
		}
	}()

	return app, nil
}

func (app *App) Run() {
	// check storage & network
	// get storage & network? (or do it each time it's needed ?)

	// "hub" to broadcast logs per vm

	// probably wrap all this into a simple helper (route, method(s), controller)
	// wishlist:
	// - controller should be interface based
	// - headers must be automatic
	// - must deal with usual response and streams
	// - for streams, should deal with the hub in the background (global AND instances!)

	http.HandleFunc("/phone", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		phoneController(w, r, app)
	})

	http.HandleFunc("/log", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		logController(w, r, app)
	})

	http.HandleFunc("/instances", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		// instancesController(w, r)
	})

	app.log.Info(fmt.Sprintf("Mulch listening on %s", *addr))
	err := http.ListenAndServe(*addr, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
