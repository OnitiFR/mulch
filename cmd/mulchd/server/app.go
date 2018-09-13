package server

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"
)

// App describes an (the?) application
type App struct {
	Config    *AppConfig
	Libvirt   *Libvirt
	Hub       *Hub
	PhoneHome *PhoneHomeHub
	Log       *Log
	Mux       *http.ServeMux
	Rand      *rand.Rand
	VMDB      *VMDatabase
	APIKeysDB *APIKeyDatabase
	Seeder    *SeedDatabase
	routes    map[string][]*Route
}

// NewApp creates a new application
func NewApp(config *AppConfig, trace bool) (*App, error) {
	app := &App{
		Config: config,
		Rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
		routes: make(map[string][]*Route),
	}

	app.Hub = NewHub(trace)
	go app.Hub.Run()

	app.Log = NewLog("", app.Hub)
	app.Log.Trace("log system available")

	lv, err := NewLibvirt(config.LibVirtURI)
	if err != nil {
		return nil, err
	}
	app.Log.Info(fmt.Sprintf("libvirt connection to '%s' OK", config.LibVirtURI))
	app.Libvirt = lv

	err = app.checkDataPath()
	if err != nil {
		return nil, err
	}

	err = app.initVMDB()
	if err != nil {
		return nil, err
	}

	err = app.initAPIKeysDB()
	if err != nil {
		return nil, err
	}

	err = app.initSSH()
	if err != nil {
		return nil, err
	}

	err = app.initLibvirtStorage()
	if err != nil {
		return nil, err
	}

	err = app.initLibvirtNetwork()
	if err != nil {
		return nil, err
	}

	err = app.initSeedsDB()
	if err != nil {
		return nil, err
	}
	go app.Seeder.Run()

	app.PhoneHome = NewPhoneHomeHub()

	app.Mux = http.NewServeMux()

	// dirty log broadcast tests
	// go func() {
	// 	for {
	// 		delay := app.Rand.Intn(12000)
	// 		time.Sleep(time.Duration(delay) * time.Millisecond)
	// 		app.Log.Tracef("Test %d", delay)
	// 	}
	// }()
	// go func() {
	// 	for {
	// 		delay := app.Rand.Intn(12000)
	// 		time.Sleep(time.Duration(delay) * time.Millisecond)
	// 		fmt.Printf("INFO(): test instance 1 (%d)\n", delay)
	// 		app.Hub.Broadcast(mulch.NewMessage(mulch.MessageInfo, "instance-1", "Test instance 1"))
	// 	}
	// }()

	return app, nil
}

func (app *App) checkDataPath() error {
	if _, err := os.Stat(app.Config.DataPath); os.IsNotExist(err) {
		return fmt.Errorf("data path (%s) does not exist", app.Config.DataPath)
	}
	return nil
}

func (app *App) initVMDB() error {
	dbPath := app.Config.DataPath + "/mulch-vm.db"

	vmdb, err := NewVMDatabase(dbPath)
	if err != nil {
		return err
	}
	app.VMDB = vmdb

	// remove old entries from DB
	// + "rebuild" parts of the VM in the DB (ex : App)
	vmNames := app.VMDB.GetNames()
	for _, name := range vmNames {
		domainName := app.Config.VMPrefix + name
		dom, err := app.Libvirt.GetDomainByName(domainName)
		if err != nil {
			return err
		}
		if dom == nil {
			app.Log.Warningf("VM '%s' does not exists in libvirt, deleting from Mulch DB", name)
			app.VMDB.Delete(name)
		} else {
			vm, err2 := app.VMDB.GetByName(name)
			uuid, err1 := dom.GetUUIDString()
			dom.Free()

			if err1 != nil || err2 != nil {
				app.Log.Errorf("database checking failure: %s / %s", err1, err2)
			}

			if uuid != vm.LibvirtUUID {
				app.Log.Warningf("libvirt UUID mismatch for VM '%s'", name)
			}

			// + "rebuild" parts of the VM in the DB? (ex : App)
			// we are erasing original values like vm.App.Config that can be useful, no ?
			vm.App = app
		}
	}

	app.Log.Infof("found %d VM(s) in database %s", app.VMDB.Count(), dbPath)

	// detect missing entries from DB?
	return nil
}

func (app *App) initAPIKeysDB() error {
	dbPath := app.Config.DataPath + "/mulch-api-keys.db"

	db, err := NewAPIKeyDatabase(dbPath, app.Log, app.Rand)
	if err != nil {
		return err
	}
	app.APIKeysDB = db
	return nil
}

func (app *App) initSeedsDB() error {
	dbPath := app.Config.DataPath + "/mulch-seeds.db"

	seeder, err := NewSeeder(dbPath, app)
	if err != nil {
		return err
	}
	app.Seeder = seeder

	return nil
}

func (app *App) initSSH() error {
	if _, err := os.Stat(app.Config.MulchSSHPrivateKey); os.IsNotExist(err) {
		app.Log.Warningf("SSH private key not found, mulch will fail to control VMs! (%s)", app.Config.MulchSSHPrivateKey)
	}
	if _, err := os.Stat(app.Config.MulchSSHPublicKey); os.IsNotExist(err) {
		app.Log.Warningf("SSH public key not found, VM creation will fail! (%s)", app.Config.MulchSSHPublicKey)
	}

	return nil
}

func (app *App) initLibvirtStorage() error {
	var err error
	var pools = &app.Libvirt.Pools

	pools.CloudInit, pools.CloudInitXML, err = app.Libvirt.GetOrCreateStoragePool(
		"mulch-cloud-init",
		app.Config.StoragePath+"/cloud-init",
		app.Config.configPath+"/templates/storage.xml",
		"0711",
		app.Log)
	if err != nil {
		return fmt.Errorf("initLibvirtStorage (cloud-init/): %s", err)
	}

	pools.Seeds, pools.SeedsXML, err = app.Libvirt.GetOrCreateStoragePool(
		"mulch-seeds",
		app.Config.StoragePath+"/seeds",
		app.Config.configPath+"/templates/storage.xml",
		"",
		app.Log)
	if err != nil {
		return fmt.Errorf("initLibvirtStorage (seeds): %s", err)
	}

	pools.Disks, pools.DisksXML, err = app.Libvirt.GetOrCreateStoragePool(
		"mulch-disks",
		app.Config.StoragePath+"/disks",
		app.Config.configPath+"/templates/storage.xml",
		"0711",
		app.Log)
	if err != nil {
		return fmt.Errorf("initLibvirtStorage (disks): %s", err)
	}

	pools.Backups, pools.BackupsXML, err = app.Libvirt.GetOrCreateStoragePool(
		"mulch-backups",
		app.Config.StoragePath+"/backups",
		app.Config.configPath+"/templates/storage.xml",
		"0711",
		app.Log)
	if err != nil {
		return fmt.Errorf("initLibvirtStorage (backups): %s", err)
	}

	return nil
}

func (app *App) initLibvirtNetwork() error {
	networkName := "mulch"

	net, netcfg, err := app.Libvirt.GetOrCreateNetwork(
		networkName,
		app.Config.configPath+"/templates/network.xml",
		app.Log)

	if err != nil {
		return fmt.Errorf("initLibvirtNetwork: %s", err)
	}

	app.Log.Info(fmt.Sprintf("network '%s': %s (%s)", netcfg.Name, netcfg.IPs[0].Address, netcfg.Bridge.Name))

	app.Libvirt.Network = net
	app.Libvirt.NetworkXML = netcfg

	return nil
}

// Run will start the app (in the foreground)
func (app *App) Run() {
	app.Log.Infof("API server listening on %s", app.Config.Listen)
	app.registerRouteHandlers()
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
