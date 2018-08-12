package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/Xfennec/mulch"
	libvirt "github.com/libvirt/libvirt-go"
)

// App describes an (the?) application
type App struct {
	Config  *AppConfig
	Libvirt *Libvirt
	Hub     *Hub
	Log     *Log
	Mux     *http.ServeMux
	Pools   LibvirtPools
}

// LibvirtPools stores needed libvirt Pools for mulchd
type LibvirtPools struct {
	CloudInit *libvirt.StoragePool
	Releases  *libvirt.StoragePool
	Disks     *libvirt.StoragePool
}

// NewApp creates a new application
func NewApp(config *AppConfig) (*App, error) {
	app := &App{
		Config: config,
	}

	app.Hub = NewHub()
	go app.Hub.Run()

	app.Log = NewLog("", app.Hub)

	lv, err := NewLibvirt(config.LibVirtURI)
	if err != nil {
		return nil, err
	}
	app.Log.Info(fmt.Sprintf("libvirt connection to '%s' OK", config.LibVirtURI))
	app.Libvirt = lv

	err = app.initLibvirtStorage()
	if err != nil {
		return nil, err
	}

	err = app.initLibvirtNetwork()
	if err != nil {
		return nil, err
	}

	app.Mux = http.NewServeMux()

	// dirty log broadcast tests
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
}

func (app *App) initLibvirtStorage() error {
	var err error

	app.Pools.CloudInit, err = app.Libvirt.GetOrCreateStoragePool(
		"mulch-cloud-init",
		app.Config.StoragePath+"/cloud-init",
		app.Config.configPath+"/templates/storage.xml",
		"",
		app.Log)
	if err != nil {
		return fmt.Errorf("initLibvirtStorage (cloud-init/): %s", err)
	}

	app.Pools.CloudInit, err = app.Libvirt.GetOrCreateStoragePool(
		"mulch-releases",
		app.Config.StoragePath+"/releases",
		app.Config.configPath+"/templates/storage.xml",
		"",
		app.Log)
	if err != nil {
		return fmt.Errorf("initLibvirtStorage (releases): %s", err)
	}

	app.Pools.CloudInit, err = app.Libvirt.GetOrCreateStoragePool(
		"mulch-disks",
		app.Config.StoragePath+"/disks",
		app.Config.configPath+"/templates/storage.xml",
		"0711",
		app.Log)
	if err != nil {
		return fmt.Errorf("initLibvirtStorage (disks): %s", err)
	}

	return nil
}

func (app *App) initLibvirtNetwork() error {
	return nil
}

// Run will start the app (in the foreground)
func (app *App) Run() {
	// do this in some sort of Setup()?
	// check storage & network
	// get storage & network? (or do it each time it's needed ?)
	app.setupRoutes()

	// "hub" to broadcast logs per vm

	app.Log.Info(fmt.Sprintf("Mulch listening on %s", app.Config.Listen))
	err := http.ListenAndServe(app.Config.Listen, app.Mux)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

// Close is not called yet
func (app *App) Close() {
	// close pools
	// close connection (app.Libvirt.CloseConnection())
}
