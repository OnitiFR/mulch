package server

import (
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"syscall"
	"time"

	"github.com/OnitiFR/mulch/common"
	libvirt "github.com/libvirt/libvirt-go"
)

// Mulch storage and network names, see the following usages:
// - App.initLibvirtStorage()
// - Libvirt.GetConnection()
const (
	AppStorageCloudInit = "mulch-cloud-init"
	AppStorageSeeds     = "mulch-seeds"
	AppStorageDisks     = "mulch-disks"
	AppStorageBackups   = "mulch-backups"

	AppNetwork = "mulch"
)

// App describes an (the?) application
type App struct {
	Config      *AppConfig
	Libvirt     *Libvirt
	Hub         *Hub
	PhoneHome   *PhoneHomeHub
	Log         *Log
	Mux         *http.ServeMux
	Rand        *rand.Rand
	SSHPairDB   *SSHPairDatabase
	VMDB        *VMDatabase
	VMStateDB   *VMStateDatabase
	BackupsDB   *BackupDatabase
	APIKeysDB   *APIKeyDatabase
	AlertSender *AlertSender
	Seeder      *SeedDatabase
	routes      map[string][]*Route
	sshClients  map[net.Addr]*sshServerClient
	Operations  *OperationList
}

// NewApp creates a new application
func NewApp(config *AppConfig, trace bool) (*App, error) {
	app := &App{
		Config: config,
		Rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
		routes: make(map[string][]*Route),
	}

	if os.Getenv("TMPDIR") == "" {
		os.Setenv("TMPDIR", config.TempPath)
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

	err = app.initSSHPairDB()
	if err != nil {
		return nil, err
	}

	err = app.initVMDB()
	if err != nil {
		return nil, err
	}

	err = app.initVMStateDB()
	if err != nil {
		return nil, err
	}

	err = app.initBackupDB()
	if err != nil {
		return nil, err
	}

	err = app.initAPIKeysDB()
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

	app.AlertSender, err = NewAlertSender(app.Config.configPath, app.Log)
	if err != nil {
		return nil, err
	}

	// err = app.AlertSender.Send(&Alert{
	// 	Type:    AlertTypeGood,
	// 	Subject: "Hello",
	// 	Content: "Please to meet you with this test",
	// })
	// fmt.Println(err)

	err = app.initSeedsDB()
	if err != nil {
		return nil, err
	}
	go app.Seeder.Run()

	err = NewSSHProxyServer(app)
	if err != nil {
		return nil, err
	}

	app.Operations = NewOperationList(app.Rand)

	app.PhoneHome = NewPhoneHomeHub()

	app.Mux = http.NewServeMux()

	go app.VMStateDB.Run()

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
	if common.PathExist(app.Config.DataPath) == false {
		return fmt.Errorf("data path (%s) does not exist", app.Config.DataPath)
	}
	return nil
}

func (app *App) initSSHPairDB() error {
	dbPath := app.Config.DataPath + "/mulch-ssh-pairs.db"

	pairdb, err := NewSSHPairDatabase(dbPath)
	if err != nil {
		return err
	}

	if pairdb.GetByName(SSHSuperUserPair) == nil {
		app.Log.Info("generating super user SSH key pair")
		pairdb.AddNew(SSHSuperUserPair)
	}

	if pairdb.GetByName(SSHProxyPair) == nil {
		app.Log.Info("generating SSH Proxy key pair")
		pairdb.AddNew(SSHProxyPair)
	}

	app.SSHPairDB = pairdb

	app.Log.Infof("found %d SSH pair(s) in database %s", app.SSHPairDB.Count(), dbPath)
	return nil
}

func (app *App) initVMDB() error {
	dbPath := app.Config.DataPath + "/mulch-vm-v2.db"
	domainDbPath := app.Config.DataPath + "/mulch-proxy-domains.db"

	dbPathV1 := app.Config.DataPath + "/mulch-vm.db"
	if common.PathExist(dbPathV1) && !common.PathExist(dbPath) {
		app.Log.Warning("will migrate VM database to V2 format")
		migrate := NewVMDatabaseMigrate()
		if err := migrate.loadv1(dbPathV1); err != nil {
			return err
		}
		migrate.migrate()
		if err := migrate.savev2(dbPath); err != nil {
			return err
		}
		if err := os.Remove(dbPathV1); err != nil {
			return err
		}
		app.Log.Infof("migrate completed (entry count: %d)", len(migrate.dbv2))
	}

	vmdb, err := NewVMDatabase(dbPath, domainDbPath, app.sendProxyReloadSignal)
	if err != nil {
		return err
	}
	app.VMDB = vmdb

	// remove old entries from DB
	// + "rebuild" parts of the VM in the DB (ex : App)
	vmNames := app.VMDB.GetNames()
	for _, name := range vmNames {
		domainName := name.LibvirtDomainName(app)
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

func (app *App) initVMStateDB() error {
	dbPath := app.Config.DataPath + "/mulch-vmstates.db"

	db, err := NewVMStateDatabase(dbPath, app)
	if err != nil {
		return err
	}
	app.VMStateDB = db
	return nil
}

func (app *App) initBackupDB() error {
	dbPath := app.Config.DataPath + "/mulch-backups.db"

	db, err := NewBackupDatabase(dbPath)
	if err != nil {
		return err
	}
	app.BackupsDB = db

	app.Log.Infof("found %d backup(s) in database %s", app.BackupsDB.Count(), dbPath)

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

func (app *App) initLibvirtStorage() error {
	var err error
	var pools = &app.Libvirt.Pools

	pools.CloudInit, pools.CloudInitXML, err = app.Libvirt.GetOrCreateStoragePool(
		AppStorageCloudInit,
		app.Config.StoragePath+"/cloud-init",
		app.Config.GetTemplateFilepath("storage.xml"),
		"0711",
		app.Log)
	if err != nil {
		return fmt.Errorf("initLibvirtStorage (cloud-init/): %s", err)
	}

	pools.Seeds, pools.SeedsXML, err = app.Libvirt.GetOrCreateStoragePool(
		AppStorageSeeds,
		app.Config.StoragePath+"/seeds",
		app.Config.GetTemplateFilepath("storage.xml"),
		"",
		app.Log)
	if err != nil {
		return fmt.Errorf("initLibvirtStorage (seeds): %s", err)
	}

	pools.Disks, pools.DisksXML, err = app.Libvirt.GetOrCreateStoragePool(
		AppStorageDisks,
		app.Config.StoragePath+"/disks",
		app.Config.GetTemplateFilepath("storage.xml"),
		"0711",
		app.Log)
	if err != nil {
		return fmt.Errorf("initLibvirtStorage (disks): %s", err)
	}

	pools.Backups, pools.BackupsXML, err = app.Libvirt.GetOrCreateStoragePool(
		AppStorageBackups,
		app.Config.StoragePath+"/backups",
		app.Config.GetTemplateFilepath("storage.xml"),
		"0711",
		app.Log)
	if err != nil {
		return fmt.Errorf("initLibvirtStorage (backups): %s", err)
	}

	return nil
}

func (app *App) initLibvirtNetwork() error {
	net, netcfg, err := app.Libvirt.GetOrCreateNetwork(
		AppNetwork,
		app.Config.GetTemplateFilepath("network.xml"),
		app.Log)

	if err != nil {
		return fmt.Errorf("initLibvirtNetwork: %s", err)
	}

	app.Log.Info(fmt.Sprintf("network '%s': %s (%s)", netcfg.Name, netcfg.IPs[0].Address, netcfg.Bridge.Name))

	app.Libvirt.Network = net
	app.Libvirt.NetworkXML = netcfg

	return nil
}

func (app *App) sendProxyReloadSignal() {
	lastPidFilename := path.Clean(app.Config.DataPath + "/mulch-proxy-last.pid")
	data, err := ioutil.ReadFile(lastPidFilename)
	if err != nil {
		app.Log.Errorf("reloading mulch-proxy config: %s", err)
		return
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		app.Log.Errorf("reloading mulch-proxy config: pid '%s': %s", data, err)
		return
	}

	p, err := os.FindProcess(pid)
	if err != nil {
		app.Log.Errorf("reloading mulch-proxy config: process: %s", err)
		return
	}

	err = p.Signal(syscall.SIGHUP)
	if err != nil {
		app.Log.Errorf("reloading mulch-proxy config: signal: %s", err)
		return
	}
	app.Log.Info("HUP signal sent to mulch-proxy")
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

// Status returns informations about Mulch server
func (app *App) Status() (*common.APIStatus, error) {
	var ret common.APIStatus
	conn, err := app.Libvirt.GetConnection()
	if err != nil {
		return nil, err
	}

	infos, err := conn.GetNodeInfo()
	if err != nil {
		return nil, err
	}

	err = app.Libvirt.Pools.Disks.Refresh(0)
	if err != nil {
		return nil, err
	}

	err = app.Libvirt.Pools.Backups.Refresh(0)
	if err != nil {
		return nil, err
	}

	disksInfos, err := app.Libvirt.Pools.Disks.GetInfo()
	if err != nil {
		return nil, err
	}

	backupsInfos, err := app.Libvirt.Pools.Backups.GetInfo()
	if err != nil {
		return nil, err
	}

	vmNames := app.VMDB.GetNames()
	vmTotal := len(vmNames)
	vmCPUs := 0
	vmActiveCPUs := 0
	vmActiveTotal := 0
	vmMem := 0
	vmActiveMem := 0
	provisionedDisks := 0
	allocatedDisks := 0

	for _, vmName := range vmNames {
		vm, err := app.VMDB.GetByName(vmName)
		if err != nil {
			return nil, fmt.Errorf("VM '%s': %s", vmName, err)
		}

		libvirtName := vmName.LibvirtDomainName(app)
		domain, err := app.Libvirt.GetDomainByName(libvirtName)
		if err != nil {
			return nil, err
		}
		if domain == nil {
			return nil, fmt.Errorf("VM '%s': does not exists in libvirt", vmName)
		}
		defer domain.Free()

		state, _, err := domain.GetState()
		if err != nil {
			return nil, fmt.Errorf("VM '%s': %s", vmName, err)
		}

		vmCPUs += vm.Config.CPUCount
		vmMem += int(vm.Config.RAMSize / 1024 / 1024)
		provisionedDisks += int(vm.Config.DiskSize / 1024 / 1024)

		if state == libvirt.DOMAIN_RUNNING {
			vmActiveTotal++
			vmActiveCPUs += vm.Config.CPUCount
			vmActiveMem += int(vm.Config.RAMSize / 1024 / 1024)
		}

		diskName, err := VMGetDiskName(vmName, app)
		if err != nil {
			return nil, fmt.Errorf("VM '%s': %s", vmName, err)
		}
		vInfos, err := app.Libvirt.VolumeInfos(diskName, app.Libvirt.Pools.Disks)
		if err != nil {
			return nil, fmt.Errorf("VM '%s': %s", vmName, err)
		}
		allocatedDisks += int(vInfos.Allocation / 1024 / 1024)
	}

	for _, client := range app.sshClients {
		entry, err := app.VMDB.GetEntryByVM(client.vm)
		if err != nil {
			continue
		}
		ret.SSHConnections = append(ret.SSHConnections, common.APISSHConnection{
			FromIP:    client.remoteAddr.String(),
			FromUser:  client.apiAuth,
			ToUser:    client.sshUser,
			ToVMName:  entry.Name.ID(),
			StartTime: client.startTime,
		})
	}

	for _, operation := range app.Operations.operations {
		ret.Operations = append(ret.Operations, common.APIOperation{
			Origin:        operation.Origin,
			Action:        operation.Action,
			Ressource:     operation.Ressource,
			RessourceName: operation.RessourceName,
		})
	}

	ret.VMs = vmTotal
	ret.ActiveVMs = vmActiveTotal
	ret.HostCPUs = int(infos.Cpus)
	ret.HostMemoryTotalMB = int(infos.Memory / 1024)
	ret.VMCPUs = vmCPUs
	ret.VMActiveCPUs = vmActiveCPUs
	ret.VMMemMB = vmMem
	ret.VMActiveMemMB = vmActiveMem
	ret.FreeStorageMB = int(disksInfos.Available / 1024 / 1024)
	ret.FreeBackupMB = int(backupsInfos.Available / 1024 / 1024)
	ret.ProvisionedDisksMB = provisionedDisks
	ret.AllocatedDisksMB = allocatedDisks

	return &ret, nil
}
