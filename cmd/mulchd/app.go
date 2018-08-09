package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/Xfennec/mulch"
)

type App struct {
	// config
	// TODO: deal with multiple connections!
	lvconn *LibvirtConnection
	hub    *Hub
	log    *Log
	mux    *http.ServeMux
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

	mux := http.NewServeMux()

	app := &App{
		lvconn: lc,
		hub:    hub,
		log:    log,
		mux:    mux,
	}

	app.log.Info(fmt.Sprintf("libvirt connection to '%s' OK", uri))

	// dirty log broadcast test
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	go func() {
		for {
			delay := rnd.Intn(12000)
			time.Sleep(time.Duration(delay) * time.Millisecond)
			app.log.Info(fmt.Sprintf("Test %d", delay))
		}
	}()
	go func() {
		for {
			delay := rnd.Intn(12000)
			time.Sleep(time.Duration(delay) * time.Millisecond)
			fmt.Printf("INFO(): test instance 1 (%d)\n", delay)
			app.hub.Broadcast(mulch.NewMessage(mulch.MessageInfo, "instance-1", "Test instance 1"))
		}
	}()

	return app, nil
}

func (app *App) Run() {
	// do this in some sort of Setup()?
	// check storage & network
	// get storage & network? (or do it each time it's needed ?)

	// "hub" to broadcast logs per vm

	// All this will soon use AddRouteHandler
	app.mux.HandleFunc("/phone", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		phoneController(w, r, app)
	})

	app.mux.HandleFunc("/log", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		logController(w, r, app)
	})

	app.mux.HandleFunc("/instances", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		// instancesController(w, r)
	})

	app.log.Info(fmt.Sprintf("Mulch listening on %s", *addr))
	err := http.ListenAndServe(*addr, app.mux)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
