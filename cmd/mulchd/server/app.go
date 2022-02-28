package server

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"path"
	"runtime/pprof"
	"strconv"
	"syscall"
	"time"

	"github.com/OnitiFR/mulch/common"
	libvirtxml "gopkg.in/libvirt/libvirt-go-xml.v5"
	libvirt "gopkg.in/libvirt/libvirt-go.v5"
)

// Mulch storage and network names, see the following usages:
// - App.initLibvirtStorage()
// - Libvirt.GetConnection()
const (
	AppStorageSeeds   = "mulch-seeds"
	AppStorageDisks   = "mulch-disks"
	AppStorageBackups = "mulch-backups"

	AppNetwork  = "mulch"
	AppNWFilter = "mulch-filter"
)

// LogHistorySize is the maximum number of messages in app log history
// ~128kB / 1000 messages (very rough approx!)
const LogHistorySize = 20000 // ~2.5mB

// App describes an (the?) application
type App struct {
	StartTime      time.Time
	Config         *AppConfig
	Libvirt        *Libvirt
	Hub            *Hub
	PhoneHome      *PhoneHomeHub
	Log            *Log
	LogHistory     *LogHistory
	MuxInternal    *http.ServeMux
	MuxAPI         *http.ServeMux
	Rand           *rand.Rand
	SSHPairDB      *SSHPairDatabase
	VMDB           *VMDatabase
	VMStateDB      *VMStateDatabase
	BackupsDB      *BackupDatabase
	APIKeysDB      *APIKeyDatabase
	AlertSender    *AlertSender
	Seeder         *SeedDatabase
	routesInternal map[string][]*Route
	routesAPI      map[string][]*Route
	sshClients     *sshServerClients
	Operations     *OperationList
	ProxyReloader  *ProxyReloader
}

// NewApp creates a new application
func NewApp(config *AppConfig, trace bool) (*App, error) {
	app := &App{
		StartTime:      time.Now(),
		Config:         config,
		Rand:           rand.New(rand.NewSource(time.Now().UnixNano())),
		routesInternal: make(map[string][]*Route),
		routesAPI:      make(map[string][]*Route),
	}

	if os.Getenv("TMPDIR") == "" {
		os.Setenv("TMPDIR", config.TempPath)
	}

	app.Hub = NewHub(trace)
	go app.Hub.Run()

	app.LogHistory = NewLogHistory(LogHistorySize)
	app.Log = NewLog("", app.Hub, app.LogHistory)
	app.Log.Trace("log system available")

	err := app.checkDataPath()
	if err != nil {
		return nil, err
	}

	app.initSigQUITHandler()

	app.Libvirt, err = NewLibvirt(config.LibVirtURI)
	if err != nil {
		return nil, err
	}
	app.Log.Info(fmt.Sprintf("libvirt connection to '%s' OK", config.LibVirtURI))

	err = app.initLibvirtStorage()
	if err != nil {
		return nil, err
	}

	err = app.initLibvirtNetwork()
	if err != nil {
		return nil, err
	}

	err = app.initLibvirtNWFilter()
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

	// clean DHCP leases
	err = app.Libvirt.RebuildDHCPStaticLeases(app)
	if err != nil {
		return nil, fmt.Errorf("RebuildDHCPStaticHost: %s", err)
	}

	err = app.initBackupDB()
	if err != nil {
		return nil, err
	}

	err = app.initAPIKeysDB()
	if err != nil {
		return nil, err
	}

	app.AlertSender, err = NewAlertSender(app.Config.configPath, app.Log)
	if err != nil {
		return nil, err
	}
	app.AlertSender.RunKeepAlive(5)

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

	app.initWebServers()

	go app.VMStateDB.Run()

	go AutoRebuildSchedule(app)

	return app, nil
}

func (app *App) checkDataPath() error {
	if !common.PathExist(app.Config.DataPath) {
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

	SSHSuperUserPair := app.Config.MulchSuperUserSSHKey
	if pairdb.GetByName(SSHSuperUserPair) == nil {
		app.Log.Infof("generating super user SSH key pair '%s'", SSHSuperUserPair)
		err = pairdb.AddNew(SSHSuperUserPair)
		if err != nil {
			return err
		}
	}

	if pairdb.GetByName(SSHProxyPair) == nil {
		app.Log.Info("generating SSH Proxy key pair")
		err = pairdb.AddNew(SSHProxyPair)
		if err != nil {
			return err
		}
	}

	app.SSHPairDB = pairdb

	app.Log.Infof("found %d SSH pair(s) in database %s", app.SSHPairDB.Count(), dbPath)
	return nil
}

func (app *App) initVMDB() error {
	dbPath := app.Config.DataPath + "/mulch-vm-v2.db"
	domainDbPath := app.Config.DataPath + "/mulch-proxy-domains.db"
	portsDbPath := app.Config.DataPath + "/mulch-proxy-ports.db"

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

	app.ProxyReloader = NewProxyReloader(app)
	vmdb, err := NewVMDatabase(dbPath, domainDbPath, portsDbPath, app.ProxyReloader.Request, app)
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
			func() { // use an anonymous func for the defer
				vm, err2 := app.VMDB.GetByName(name)
				uuid, err1 := dom.GetUUIDString()
				defer dom.Free()

				if err1 != nil || err2 != nil {
					app.Log.Errorf("database checking failure: %s / %s (%s)", err1, err2, name)
				}

				if uuid != vm.LibvirtUUID {
					app.Log.Warningf("libvirt UUID mismatch for VM '%s'", name)
				}

				// upgrade new VM fields
				if vm.AssignedIPv4 == "" {
					vm.AssignedIPv4 = vm.LastIP
				}
				if vm.AssignedMAC == "" {
					xmldoc, err := dom.GetXMLDesc(0)
					if err != nil {
						app.Log.Errorf("VM %s: %s", name, err)
						return
					}

					domcfg := &libvirtxml.Domain{}
					err = domcfg.Unmarshal(xmldoc)
					if err != nil {
						app.Log.Errorf("VM %s: %s", name, err)
						return
					}
					foundInterfaces := 0
					for _, intf := range domcfg.Devices.Interfaces {
						if intf.Alias != nil && intf.Alias.Name == VMNetworkAliasBridge {
							vm.AssignedMAC = intf.MAC.Address
							foundInterfaces++
						}
					}
					if foundInterfaces != 1 {
						app.Log.Errorf("VM %s: can't find exactly one %s network interface", name, VMNetworkAliasBridge)
						return
					}

				}
				if vm.MulchSuperUserSSHKey == "" {
					vm.MulchSuperUserSSHKey = app.Config.MulchSuperUserSSHKey
				}

				// + "rebuild" parts of the VM in the DB
				vm.App = app
				vm.WIP = VMOperationNone
			}()
		}
	}

	app.VMDB.Update() // save anything that happend during the previous loop
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

func (app *App) initLibvirtNWFilter() error {
	_, err := app.Libvirt.GetOrCreateNWFilter(
		AppNWFilter,
		app.Config.GetTemplateFilepath("nwfilter.xml"),
		app.Log)

	if err != nil {
		return fmt.Errorf("initLibvirtNWFilter: %s", err)
	}
	return nil
}

func (app *App) initWebServers() {
	app.MuxInternal = http.NewServeMux()
	app.MuxAPI = http.NewServeMux()
}

func writePprofProfile(profile string, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("unable to create %s: %s", filename, err)
	}

	defer file.Close()
	writer := bufio.NewWriter(file)
	defer writer.Flush()

	if err := pprof.Lookup(profile).WriteTo(writer, 0); err != nil {
		return fmt.Errorf("could not write profile: %s", err)
	}
	return nil
}

// write a pprof memory profile dump on SIGQUIT
// kill -QUIT $(pidof mulchd)
func (app *App) initSigQUITHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGQUIT)

	go func() {
		for range c {
			app.Log.Infof("QUIT Signal")

			ts := time.Now().Format("20060102-150405")
			rnd := strconv.Itoa(app.Rand.Int())

			pathHeap := path.Clean(os.TempDir() + "/" + "mulchd-heap" + ts + "-" + rnd + ".prof")
			app.Log.Infof("writing %s", pathHeap)
			err := writePprofProfile("heap", pathHeap)
			if err != nil {
				app.Log.Error(err.Error())
			}

			pathGoroutine := path.Clean(os.TempDir() + "/" + "mulchd-goroutine" + ts + "-" + rnd + ".prof")
			app.Log.Infof("writing %s", pathGoroutine)
			err = writePprofProfile("goroutine", pathGoroutine)
			if err != nil {
				app.Log.Error(err.Error())
			}
		}
	}()
}

// Run will start the app servers (foreground)
func (app *App) Run() {
	app.registerRouteHandlers(app.MuxInternal, app.routesInternal)
	app.registerRouteHandlers(app.MuxAPI, app.routesAPI)

	errChan := make(chan error)

	go func() {
		if app.Config.ListenHTTPSDomain == "" {
			// HTTP API Server
			app.Log.Infof("API server listening on %s (HTTP)", app.Config.Listen)
			err := http.ListenAndServe(app.Config.Listen, app.MuxAPI)
			errChan <- fmt.Errorf("ListenAndServe API server: %s", err)
		} else {
			// HTTPS API Server
			app.Log.Infof("API server listening on %s (HTTPS, %s)", app.Config.Listen, app.Config.ListenHTTPSDomain)

			manager := &CertManager{
				CertDir: app.Config.DataPath + "/certs",
				Domain:  app.Config.ListenHTTPSDomain,
				Log:     app.Log,
			}

			manager.ScheduleSelfCalls()

			httpsSrv := &http.Server{
				Handler:   app.MuxAPI,
				Addr:      app.Config.Listen,
				TLSConfig: &tls.Config{GetCertificate: manager.GetAPICertificate},
			}

			err := httpsSrv.ListenAndServeTLS("", "")
			if err != nil {
				errChan <- fmt.Errorf("ListendAndServeTLS API server: %s", err)
			}
		}
	}()

	go func() {
		listen := app.Libvirt.NetworkXML.IPs[0].Address + ":" + strconv.Itoa(app.Config.InternalServerPort)
		app.Log.Infof("Internal server listening on %s", listen)
		err := http.ListenAndServe(listen, app.MuxInternal)
		errChan <- fmt.Errorf("ListenAndServe internal server: %s", err)
	}()

	err := <-errChan
	log.Fatalf("error: %s", err)
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

	clients := app.sshClients.getClients()
	for _, client := range clients {
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
			StartTime:     operation.StartTime,
		})
	}

	ret.StartTime = app.StartTime
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
