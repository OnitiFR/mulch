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
	LVConn *LibvirtConnection
	Hub    *Hub
	Log    *Log
	Mux    *http.ServeMux
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
		LVConn: lc,
		Hub:    hub,
		Log:    log,
		Mux:    mux,
	}

	app.Log.Info(fmt.Sprintf("libvirt connection to '%s' OK", uri))

	// dirty log broadcast test
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	go func() {
		for {
			delay := rnd.Intn(12000)
			time.Sleep(time.Duration(delay) * time.Millisecond)
			app.Log.Info(fmt.Sprintf("Test %d", delay))
		}
	}()
	go func() {
		for {
			delay := rnd.Intn(12000)
			time.Sleep(time.Duration(delay) * time.Millisecond)
			fmt.Printf("INFO(): test instance 1 (%d)\n", delay)
			app.Hub.Broadcast(mulch.NewMessage(mulch.MessageInfo, "instance-1", "Test instance 1"))
		}
	}()

	return app, nil
}

func (app *App) setupRoutes() {

	AddRoute(&Route{
		Methods: []string{"POST"},
		Path:    "/phone",
		Type:    RouteTypeCustom,
		Handler: PhoneController,
	}, app)

	AddRoute(&Route{
		Methods:      []string{"GET"},
		Path:         "/log",
		Type:         RouteTypeStream,
		IsRestricted: true,
		Handler:      LogController,
	}, app)

	/*
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
	*/
}

// Run wil start the app (in the foreground)
func (app *App) Run() {
	// do this in some sort of Setup()?
	// check storage & network
	// get storage & network? (or do it each time it's needed ?)
	app.setupRoutes()

	// "hub" to broadcast logs per vm

	app.Log.Info(fmt.Sprintf("Mulch listening on %s", *addr))
	err := http.ListenAndServe(*addr, app.Mux)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
