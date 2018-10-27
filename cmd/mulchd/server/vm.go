package server

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"time"

	"github.com/Xfennec/mulch/common"
	"github.com/libvirt/libvirt-go"
	"github.com/libvirt/libvirt-go-xml"
	"github.com/satori/go.uuid"
	"golang.org/x/crypto/ssh"
)

// Aliases for vm.xml file
const (
	VMStorageAliasDisk      = "ua-mulch-disk"
	VMStorageAliasCloudInit = "ua-mulch-cloudinit"
	VMStorageAliasBackup    = "ua-mulch-backup"
	VMNetworkAliasBridge    = "ua-mulch-bridge"
)

// VMOperation defines heavy operations in the VM
type VMOperation string

// VMOperation values
const (
	VMOperationNone    = ""
	VMOperationBackup  = "backup"
	VMOperationRestore = "restore"
)

// VM defines a virtual machine ("domain")
type VM struct {
	LibvirtUUID string
	SecretUUID  string
	App         *App
	Config      *VMConfig
	AuthorKey   string
	LastIP      string
	Locked      bool
	WIP         VMOperation
}

// SetOperation change VM WIP
func (vm *VM) SetOperation(op VMOperation) {
	vm.WIP = op
}

func checkAllDomains(db *VMDatabase, domains []*common.Domain) error {
	domainMap := make(map[string]*VM)
	vmNames := db.GetNames()
	for _, vmName := range vmNames {
		vm, err := db.GetByName(vmName)
		if err != nil {
			return err
		}
		for _, domain := range vm.Config.Domains {
			domainMap[domain.Name] = vm
		}
	}

	for _, domain := range domains {
		vm, exist := domainMap[domain.Name]
		if exist == true {
			return fmt.Errorf("vm '%s' already registered domain '%s'", vm.Config.Name, domain.Name)
		}
	}

	return nil
}

// small helper to generate CloudImage name and main disk name
func vmGenVolumesNames(vmName string) (string, string) {
	ciName := "ci-" + vmName + ".img"
	diskName := vmName + ".qcow2"
	return ciName, diskName
}

// NewVM builds a new virtual machine from config
// TODO: this function is HUUUGE and needs to be splitted. It's tricky
// because there's a "transaction" here.
func NewVM(vmConfig *VMConfig, authorKey string, app *App, log *Log) (*VM, error) {
	log.Infof("creating new VM '%s'", vmConfig.Name)

	commit := false

	secretUUID, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}

	vm := &VM{
		App:        app,
		SecretUUID: secretUUID.String(),
		Config:     vmConfig, // copy()? (deep)
		AuthorKey:  authorKey,
		Locked:     false,
		WIP:        VMOperationNone,
	}

	conn, err := app.Libvirt.GetConnection()
	if err != nil {
		return nil, err
	}

	if !IsValidTokenName(vmConfig.Name) {
		return nil, fmt.Errorf("name '%s' is invalid (need only letters, numbers and underscore, do not start with a number)", vmConfig.Name)
	}

	_, err = app.VMDB.GetByName(vmConfig.Name)
	if err == nil {
		return nil, fmt.Errorf("VM '%s' already exists in database", vmConfig.Name)
	}

	domainName := app.Config.VMPrefix + vmConfig.Name

	_, err = conn.LookupDomainByName(domainName)
	if err == nil {
		return nil, fmt.Errorf("VM '%s' already exists in libvirt", domainName)
	}
	errDetails := err.(libvirt.Error)
	if errDetails.Domain != libvirt.FROM_QEMU || errDetails.Code != libvirt.ERR_NO_DOMAIN {
		return nil, fmt.Errorf("Unexpected error: %s", err)
	}

	ciName, diskName := vmGenVolumesNames(vmConfig.Name)

	seed, err := app.Seeder.GetByName(vmConfig.Seed)
	if err != nil {
		return nil, err
	}

	if seed.Ready == false {
		return nil, fmt.Errorf("seed %s is not ready", vmConfig.Seed)
	}

	// check for conclicting domains (will also be done later while saving vm database)
	err = checkAllDomains(app.VMDB, vmConfig.Domains)
	if err != nil {
		return nil, err
	}

	// check if backup exists (if a restore was requested)
	backup := app.BackupsDB.GetByName(vm.Config.RestoreBackup)
	if vm.Config.RestoreBackup != "" {
		if backup == nil {
			return nil, fmt.Errorf("backup '%s' not found in database", vm.Config.RestoreBackup)
		}
		if len(vm.Config.Restore) == 0 {
			return nil, errors.New("no restore script defined for this VM, can't restore")
		}
	}

	SSHSuperUserAuth, err := app.SSHPairDB.GetPublicKeyAuth(SSHSuperUserPair)
	if err != nil {
		return nil, err
	}

	// 1 - copy from reference image
	log.Infof("creating VM disk '%s'", diskName)
	err = app.Libvirt.CreateDiskFromSeed(
		seed.As,
		diskName,
		app.Config.GetTemplateFilepath("volume.xml"),
		log)

	if err != nil {
		return nil, err
	}

	// delete the created volume in case of failure of the rest of the VM creation
	defer func() {
		if !commit {
			log.Infof("rollback, deleting disk '%s'", diskName)
			vol, errDef := app.Libvirt.Pools.Disks.LookupStorageVolByName(diskName)
			if errDef != nil {
				log.Errorf("failed LookupStorageVolByName: %s (%s)", errDef, diskName)
				return
			}
			defer vol.Free()
			errDef = vol.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
			if errDef != nil {
				log.Errorf("failed Delete: %s (%s)", errDef, diskName)
				return
			}
		}
	}()

	// 2 - resize disk
	err = app.Libvirt.ResizeDisk(diskName, vmConfig.DiskSize, app.Libvirt.Pools.Disks, log)
	if err != nil {
		return nil, err
	}

	// 3 - Cloud-Init files
	log.Infof("creating Cloud-Init image for '%s'", vmConfig.Name)
	err = CloudInitCreate(ciName, vm, app, log)
	if err != nil {
		return nil, err
	}
	// delete the created volume in case of failure of the rest of the VM creation
	defer func() {
		if !commit {
			log.Infof("rollback, deleting cloud-init image '%s'", ciName)
			vol, errDef := app.Libvirt.Pools.CloudInit.LookupStorageVolByName(ciName)
			if errDef != nil {
				log.Errorf("failed LookupStorageVolByName: %s (%s)", errDef, ciName)
				return
			}
			defer vol.Free()
			errDef = vol.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
			if errDef != nil {
				log.Errorf("failed Delete: %s (%s)", errDef, ciName)
				return
			}
		}
	}()

	// 4 - define domain
	log.Infof("defining vm domain (%s)", domainName)
	xml, err := ioutil.ReadFile(app.Config.GetTemplateFilepath("vm.xml"))
	if err != nil {
		return nil, err
	}

	domcfg := &libvirtxml.Domain{}
	err = domcfg.Unmarshal(string(xml))
	if err != nil {
		return nil, err
	}

	domcfg.Name = domainName

	domcfg.Memory.Unit = "bytes"
	domcfg.Memory.Value = uint(vm.Config.RAMSize)
	domcfg.CurrentMemory.Unit = "bytes"
	domcfg.CurrentMemory.Value = uint(vm.Config.RAMSize)

	domcfg.VCPU.Value = vm.Config.CPUCount

	foundDisks := 0
	for _, disk := range domcfg.Devices.Disks {
		if disk.Alias != nil && disk.Alias.Name == VMStorageAliasDisk {
			disk.Source.File.File = app.Libvirt.Pools.DisksXML.Target.Path + "/" + diskName
			foundDisks++
		}
		if disk.Alias != nil && disk.Alias.Name == VMStorageAliasCloudInit {
			disk.Source.File.File = app.Libvirt.Pools.CloudInitXML.Target.Path + "/" + ciName
			foundDisks++
		}
	}

	if foundDisks != 2 {
		return nil, errors.New("vm xml file: disks with 'ua-mulch-disk' and 'ua-mulch-cloudinit' aliases are required, see sample file")
	}

	foundInterfaces := 0
	for _, intf := range domcfg.Devices.Interfaces {
		if intf.Alias != nil && intf.Alias.Name == VMNetworkAliasBridge {
			intf.Source.Bridge.Bridge = app.Libvirt.NetworkXML.Bridge.Name
			intf.MAC.Address = fmt.Sprintf("52:54:00:%02x:%02x:%02x", app.Rand.Intn(255), app.Rand.Intn(255), app.Rand.Intn(255))
			foundInterfaces++
		}
	}

	if foundInterfaces != 1 {
		return nil, fmt.Errorf("vm xml file: found %d interface(s) with 'ua-mulch-bridge' alias, one is needed", foundInterfaces)
	}

	xml2, err := domcfg.Marshal()
	if err != nil {
		return nil, err
	}

	dom, err := conn.DomainDefineXML(string(xml2))
	if err != nil {
		return nil, err
	}
	defer dom.Free() // remember: "deferred calls are executed in last-in-first-out order"

	defer func() {
		if !commit {
			log.Infof("rollback, deleting vm '%s'", vm.Config.Name)
			dom.Destroy() // stop (if needed)
			errDef := dom.Undefine()
			if errDef != nil {
				log.Errorf("can't delete vm: %s", errDef)
				return
			}
		}
	}()

	libvirtUUID, err := dom.GetUUIDString()
	if err != nil {
		return nil, err
	}
	vm.LibvirtUUID = libvirtUUID

	log.Infof("vm: first boot (cloud-init)")
	if vmConfig.InitUpgrade {
		log.Info("cloud-init will upgrade packages, it may take a while…")
	} else {
		log.Warning("security: VM packages will not be up to date (init_upgrade = false)")
	}
	err = dom.Create()
	if err != nil {
		return nil, err
	}

	phone := app.PhoneHome.Register(secretUUID.String())
	defer phone.Unregister()

	phoned := false
	for done := false; done == false; {
		select {
		case <-time.After(10 * time.Minute):
			return nil, errors.New("vm init is too long, something probably went wrong")
		case call := <-phone.PhoneCalls:
			phoned = true
			log.Info("vm phoned home, cloud-init was successful")
			vm.LastIP = call.RemoteIP
		case <-time.After(1 * time.Second):
			log.Trace("checking vm state")
			state, _, errG := dom.GetState()
			if errG != nil {
				return nil, errG
			}
			if state == libvirt.DOMAIN_CRASHED {
				return nil, errors.New("vm crashed! (said libvirt)")
			}
			if state == libvirt.DOMAIN_SHUTOFF {
				log.Info("vm is now down")
				done = true
			}
		}
	}

	if phoned == false {
		return nil, errors.New("vm is down but didn't phoned home, something went wrong during cloud-init")
	}

	// if all is OK, remove and delete cloud-init image
	// EDIT: no! Cloud-init service is screwed on next boot (at least on debian)
	// log.Infof("removing cloud-init filesystem and volume")
	// dom2, err := vmDeleteCloudInitDisk(dom, app.Libvirt.Pools.CloudInit, conn)
	// if err != nil {
	// 	return nil, err
	// }
	// defer dom2.Free()

	// start the VM again
	log.Infof("starting vm")
	err = dom.Create()
	if err != nil {
		return nil, err
	}

	// wait the vm's phone call
	for done := false; done == false; {
		select {
		case <-time.After(5 * time.Minute):
			dom.Destroy()
			return nil, errors.New("vm start is too long, something probably went wrong")
		case call := <-phone.PhoneCalls:
			done = true
			log.Info("vm phoned home, boot successful")
			if call.RemoteIP != vm.LastIP {
				log.Warningf("vm IP changed since cloud-init call (from '%s' to '%s')", vm.LastIP, call.RemoteIP)
				vm.LastIP = call.RemoteIP
			}
		}
	}

	// 5 - run prepare scripts
	log.Infof("running 'prepare' scripts")
	tasks := []*RunTask{}
	for _, confTask := range vm.Config.Prepare {
		stream, errG := GetScriptFromURL(confTask.ScriptURL)
		if errG != nil {
			return nil, fmt.Errorf("unable to get script '%s': %s", confTask.ScriptURL, errG)
		}
		defer stream.Close()

		task := &RunTask{
			ScriptName:   path.Base(confTask.ScriptURL),
			ScriptReader: stream,
			As:           confTask.As,
		}
		tasks = append(tasks, task)
	}

	run := &Run{
		SSHConn: &SSHConnection{
			User: vm.App.Config.MulchSuperUser,
			Host: vm.LastIP,
			Port: 22,
			Auths: []ssh.AuthMethod{
				SSHSuperUserAuth,
			},
			Log: log,
		},
		Tasks: tasks,
		Log:   log,
	}
	err = run.Go()
	if err != nil {
		return nil, err
	}

	if vm.Config.RestoreBackup != "" {
		// 6 - restore
		log.Infof("restoring from '%s'", vm.Config.RestoreBackup)

		// attach backup
		err = VMAttachBackup(vm.Config.Name, backup.DiskName, app)
		if err != nil {
			return nil, err
		}
		defer func() {
			// detach backup
			err = VMDetachBackup(vm.Config.Name, app)
			if err != nil {
				log.Errorf("VMDetachBackup: %s", err)
			} else {
				log.Info("backup disk detached")
			}
		}()

		log.Infof("running 'restore' scripts")
		// pre-restore + restore + post-restore
		pre, errO := os.Open(app.Config.GetTemplateFilepath("pre-restore.sh"))
		if errO != nil {
			return nil, errO
		}
		defer pre.Close()

		post, errO := os.Open(app.Config.GetTemplateFilepath("post-restore.sh"))
		if errO != nil {
			return nil, errO
		}
		defer post.Close()

		tasks := []*RunTask{}
		tasks = append(tasks, &RunTask{
			ScriptName:   "pre-restore.sh",
			ScriptReader: pre,
			As:           vm.App.Config.MulchSuperUser,
		})

		for _, confTask := range vm.Config.Restore {
			stream, errG := GetScriptFromURL(confTask.ScriptURL)
			if errG != nil {
				return nil, fmt.Errorf("unable to get script '%s': %s", confTask.ScriptURL, errG)
			}
			defer stream.Close()

			task := &RunTask{
				ScriptName:   path.Base(confTask.ScriptURL),
				ScriptReader: stream,
				As:           confTask.As,
			}
			tasks = append(tasks, task)
		}

		tasks = append(tasks, &RunTask{
			ScriptName:   "post-restore.sh",
			ScriptReader: post,
			As:           vm.App.Config.MulchSuperUser,
		})
		run := &Run{
			SSHConn: &SSHConnection{
				User: vm.App.Config.MulchSuperUser,
				Host: vm.LastIP,
				Port: 22,
				Auths: []ssh.AuthMethod{
					SSHSuperUserAuth,
				},
				Log: log,
			},
			Tasks: tasks,
			Log:   log,
		}
		err = run.Go()
		if err != nil {
			return nil, err
		}
		log.Info("restore completed")
	} else {
		// 6b - run install scripts
		log.Infof("running 'install' scripts")
		tasks := []*RunTask{}
		for _, confTask := range vm.Config.Install {
			stream, errG := GetScriptFromURL(confTask.ScriptURL)
			if errG != nil {
				return nil, fmt.Errorf("unable to get script '%s': %s", confTask.ScriptURL, errG)
			}
			defer stream.Close()

			task := &RunTask{
				ScriptName:   path.Base(confTask.ScriptURL),
				ScriptReader: stream,
				As:           confTask.As,
			}
			tasks = append(tasks, task)
		}

		run := &Run{
			SSHConn: &SSHConnection{
				User: vm.App.Config.MulchSuperUser,
				Host: vm.LastIP,
				Port: 22,
				Auths: []ssh.AuthMethod{
					SSHSuperUserAuth,
				},
				Log: log,
			},
			Tasks: tasks,
			Log:   log,
		}
		err = run.Go()
		if err != nil {
			return nil, err
		}
	}

	// all is OK, commit (= no defer) and save vm to DB
	log.Infof("saving VM in database")
	err = app.VMDB.Add(vm)
	if err != nil {
		return nil, err
	}
	commit = true
	return vm, nil
}

func vmDeleteCloudInitDisk(dom *libvirt.Domain, pool *libvirt.StoragePool, conn *libvirt.Connect) (*libvirt.Domain, error) {
	// 1 - remove filesystem from domain
	xmldoc, err := dom.GetXMLDesc(0)
	if err != nil {
		return nil, err
	}

	domcfg := &libvirtxml.Domain{}
	err = domcfg.Unmarshal(xmldoc)
	if err != nil {
		return nil, err
	}

	ciName := ""
	tmp := domcfg.Devices.Disks[:0]
	for _, disk := range domcfg.Devices.Disks {
		if disk.Alias != nil && disk.Alias.Name == VMStorageAliasCloudInit {
			ciName = path.Base(disk.Source.File.File)
		} else {
			tmp = append(tmp, disk)
		}
	}
	if ciName == "" {
		return nil, fmt.Errorf("clound-init clean: disk with '%s' alias not found", VMStorageAliasCloudInit)
	}

	domcfg.Devices.Disks = tmp

	out, err := domcfg.Marshal()
	if err != nil {
		return nil, err
	}

	// update the domain
	dom2, err := conn.DomainDefineXML(string(out))
	if err != nil {
		return nil, err
	}

	// 2 - delete volume
	vol, err := pool.LookupStorageVolByName(ciName)
	if err != nil {
		return nil, err
	}
	defer vol.Free()
	err = vol.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
	if err != nil {
		return nil, err
	}

	return dom2, nil
}

// VMStopByName stops a VM using its (libvirt) name
// and waits until the VM is shutoff. (or timeouts)
func VMStopByName(name string, app *App, log *Log) error {
	domain, err := app.Libvirt.GetDomainByName(name)
	if err != nil {
		return err
	}
	if domain == nil {
		return fmt.Errorf("VM '%s': does not exists in libvirt", name)
	}
	defer domain.Free()

	// get current state
	state, _, errG := domain.GetState()
	if errG != nil {
		return errG
	}
	if state != libvirt.DOMAIN_RUNNING {
		return errors.New("VM is not up")
	}

	// shutdown
	errS := domain.Shutdown()
	if errS != nil {
		return errS
	}

	// wait shutoff state
	for done := false; done == false; {
		select {
		case <-time.After(5 * time.Minute):
			return errors.New("vm shutdown is too long")
		case <-time.After(1 * time.Second):
			log.Trace("checking vm state")
			state, _, errG := domain.GetState()
			if errG != nil {
				return errG
			}
			if state == libvirt.DOMAIN_CRASHED {
				return errors.New("vm crashed! (said libvirt)")
			}
			if state == libvirt.DOMAIN_SHUTOFF {
				done = true
			}
		}
	}

	return nil
}

// VMStartByName starts a VM using its (libvirt) name
// and waits until the VM phones home. (or timeouts)
func VMStartByName(name string, secretUUID string, app *App, log *Log) error {
	domain, err := app.Libvirt.GetDomainByName(name)
	if err != nil {
		return err
	}
	if domain == nil {
		return fmt.Errorf("VM '%s': does not exists in libvirt", name)
	}
	defer domain.Free()

	// get current state
	state, _, errG := domain.GetState()
	if errG != nil {
		return errG
	}
	if state != libvirt.DOMAIN_SHUTOFF {
		return errors.New("VM is not down")
	}

	err = domain.Create()
	if err != nil {
		return err
	}

	log.Info("started, waiting phone call")

	phone := app.PhoneHome.Register(secretUUID)
	defer phone.Unregister()

	for done := false; done == false; {
		select {
		case <-time.After(10 * time.Minute):
			return errors.New("vm is too long to start, something probably went wrong")
		case <-phone.PhoneCalls:
			done = true
			log.Info("vm phoned home")
		}
	}

	return nil
}

// VMLockUnlock will lock or unlock a VM, preventing it from deletion
func VMLockUnlock(vmName string, locked bool, vmdb *VMDatabase) error {
	vm, err := vmdb.GetByName(vmName)
	if err != nil {
		return err
	}

	vm.Locked = locked
	vmdb.Update()
	return nil
}

// VMDelete will delete a VM (using its name) and linked storages.
func VMDelete(vmName string, app *App, log *Log) error {
	vm, err := app.VMDB.GetByName(vmName)
	if err != nil {
		return err
	}

	if vm.Locked == true {
		return errors.New("VM is locked")
	}

	libvirtName := vm.App.Config.VMPrefix + vmName
	domain, err := app.Libvirt.GetDomainByName(libvirtName)
	if err != nil {
		return err
	}
	if domain == nil {
		return fmt.Errorf("VM '%s': does not exists in libvirt", libvirtName)
	}
	defer domain.Free()

	// destroy (if running)
	state, _, errG := domain.GetState()
	if errG != nil {
		return errG
	}
	if state != libvirt.DOMAIN_SHUTOFF {
		log.Info("forcing VM shutdown")
		if errD := domain.Destroy(); errD != nil {
			return errD
		}

		state, _, errG := domain.GetState()
		if errG != nil {
			return errG
		}
		if state != libvirt.DOMAIN_SHUTOFF {
			return errors.New("Unable to force stop (destroy) the VM")
		}
	}

	// undefine storages
	xmldoc, err := domain.GetXMLDesc(0)
	if err != nil {
		return err
	}

	domcfg := &libvirtxml.Domain{}
	err = domcfg.Unmarshal(xmldoc)
	if err != nil {
		return err
	}

	ciName := ""
	diskName := ""
	for _, disk := range domcfg.Devices.Disks {
		if disk.Alias != nil && disk.Alias.Name == VMStorageAliasCloudInit {
			ciName = path.Base(disk.Source.File.File)
		}
		if disk.Alias != nil && disk.Alias.Name == VMStorageAliasDisk {
			diskName = path.Base(disk.Source.File.File)
		}
	}

	// Casual refresh, without any error checking. Alacool.
	app.Libvirt.Pools.Disks.Refresh(0)
	app.Libvirt.Pools.CloudInit.Refresh(0)

	// 2 - delete Disk volume
	if diskName != "" {
		log.Infof("removing disk volume '%s'", diskName)
		diskVol, err := app.Libvirt.Pools.Disks.LookupStorageVolByName(diskName)
		if err != nil {
			return err
		}
		defer diskVol.Free()
		err = diskVol.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
		if err != nil {
			return err
		}
	}

	// 3 - delete CloudInit volume
	if ciName != "" {
		log.Infof("removing cloud-init volume '%s'", ciName)
		ciVol, err := app.Libvirt.Pools.CloudInit.LookupStorageVolByName(ciName)
		if err != nil {
			return err
		}
		defer ciVol.Free()
		err = ciVol.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
		if err != nil {
			return err
		}
	}

	log.Infof("removing VM from libvirt and database")

	// undefine domain
	errU := domain.Undefine()
	if errU != nil {
		return errU
	}

	// remove from database
	errD := app.VMDB.Delete(vmName)
	if errD != nil {
		return errD
	}

	return nil
}

// VMIsRunning returns true if VM is up and running
func VMIsRunning(vmName string, app *App) (bool, error) {
	dom, err := app.Libvirt.GetDomainByName(app.Config.VMPrefix + vmName)
	if err != nil {
		return false, err
	}
	if dom == nil {
		return false, fmt.Errorf("can't find domain '%s'", vmName)
	}
	defer dom.Free()

	state, _, errG := dom.GetState()
	if errG != nil {
		return false, errG
	}
	if state == libvirt.DOMAIN_RUNNING {
		return true, nil
	}
	return false, nil
}

// VMCreateBackupDisk create a new backup volume
// TODO: make this function transactional: remove disk if we fail in last steps
func VMCreateBackupDisk(vmName string, volName string, volSize uint64, app *App, log *Log) error {
	dom, err := app.Libvirt.GetDomainByName(app.Config.VMPrefix + vmName)
	if err != nil {
		return err
	}
	if dom == nil {
		return fmt.Errorf("can't find domain '%s'", vmName)
	}
	defer dom.Free()

	err = app.Libvirt.UploadFileToLibvirt(
		app.Libvirt.Pools.Backups,
		app.Libvirt.Pools.BackupsXML,
		path.Clean(app.Config.GetTemplateFilepath("volume.xml")),
		path.Clean(app.Config.GetTemplateFilepath("empty.qcow2")),
		volName,
		log)
	if err != nil {
		return err
	}

	err = app.Libvirt.ResizeDisk(volName, volSize, app.Libvirt.Pools.Backups, log)
	if err != nil {
		return err
	}

	return nil
}

// VMAttachBackup attach a backup volume to the VM
func VMAttachBackup(vmName string, volName string, app *App) error {
	dom, err := app.Libvirt.GetDomainByName(app.Config.VMPrefix + vmName)
	if err != nil {
		return err
	}
	if dom == nil {
		return fmt.Errorf("can't find domain '%s'", vmName)
	}
	defer dom.Free()

	xml, err := ioutil.ReadFile(app.Config.GetTemplateFilepath("disk.xml"))
	if err != nil {
		return err
	}

	diskcfg := &libvirtxml.DomainDisk{}
	err = diskcfg.Unmarshal(string(xml))
	if err != nil {
		return err
	}
	diskcfg.Alias.Name = VMStorageAliasBackup
	diskcfg.Source.File.File = app.Libvirt.Pools.BackupsXML.Target.Path + "/" + volName
	diskcfg.Target.Dev = "vdb"

	xml2, err := diskcfg.Marshal()
	if err != nil {
		return err
	}

	err = dom.AttachDevice(string(xml2))
	if err != nil {
		return err
	}

	return nil
}

// VMDetachBackup detach the backup volume from the VM
func VMDetachBackup(vmName string, app *App) error {
	dom, err := app.Libvirt.GetDomainByName(app.Config.VMPrefix + vmName)
	if err != nil {
		return err
	}
	if dom == nil {
		return fmt.Errorf("can't find domain '%s'", vmName)
	}
	defer dom.Free()

	// get disk from domain XML
	xmldoc, err := dom.GetXMLDesc(0)
	if err != nil {
		return err
	}

	domcfg := &libvirtxml.Domain{}
	err = domcfg.Unmarshal(xmldoc)
	if err != nil {
		return err
	}

	diskcfg := &libvirtxml.DomainDisk{}
	found := false
	for _, disk := range domcfg.Devices.Disks {
		if disk.Alias != nil && disk.Alias.Name == VMStorageAliasBackup {
			found = true
			*diskcfg = disk
		}
	}

	if found == false {
		return errors.New("can't find backup disk")
	}

	xml2, err := diskcfg.Marshal()
	if err != nil {
		return err
	}

	err = dom.DetachDeviceFlags(xml2, libvirt.DOMAIN_DEVICE_MODIFY_CURRENT)
	if err != nil {
		return err
	}

	return nil
}

// VMBackup launch the backup proccess (returns backup filename)
func VMBackup(vmName string, app *App, log *Log) (string, error) {
	vm, err := app.VMDB.GetByName(vmName)
	if err != nil {
		return "", err
	}

	if vm.WIP != VMOperationNone {
		return "", fmt.Errorf("VM already have a work in progress (%s)", string(vm.WIP))
	}

	vm.SetOperation(VMOperationBackup)
	defer vm.SetOperation(VMOperationNone)

	running, _ := VMIsRunning(vm.Config.Name, app)
	if running == false {
		return "", errors.New("VM should be up and running to do a backup")
	}

	if len(vm.Config.Backup) == 0 {
		return "", errors.New("no backup script defined for this VM")
	}

	volName := fmt.Sprintf("%s-backup-%s.qcow2",
		vm.Config.Name,
		time.Now().Format("20060102-150405"),
	)

	if app.BackupsDB.GetByName(volName) != nil {
		return "", fmt.Errorf("a backup with the same name already exists (%s)", volName)
	}

	SSHSuperUserAuth, err := app.SSHPairDB.GetPublicKeyAuth(SSHSuperUserPair)
	if err != nil {
		return "", err
	}

	err = VMCreateBackupDisk(vm.Config.Name, volName, vm.Config.BackupDiskSize, app, log)
	if err != nil {
		return "", err
	}

	// NOTE: this attachement is transient
	err = VMAttachBackup(vm.Config.Name, volName, app)
	if err != nil {
		return "", err
	}
	log.Info("backup disk attached")

	// defer detach + vol delete in case of failure
	commit := false
	defer func() {
		if commit == false {
			log.Info("rollback backup disk creation")
			errDet := VMDetachBackup(vm.Config.Name, app)
			if errDet != nil {
				log.Errorf("failed trying VMDetachBackup: %s (%s)", errDet, volName)
				// no return, it may be already detached
			}
			vol, errDef := app.Libvirt.Pools.Backups.LookupStorageVolByName(volName)
			if errDef != nil {
				log.Errorf("failed LookupStorageVolByName: %s (%s)", errDef, volName)
				return
			}
			defer vol.Free()
			errDef = vol.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
			if errDef != nil {
				log.Errorf("failed Delete: %s (%s)", errDef, volName)
				return
			}
		}
	}()

	pre, err := os.Open(app.Config.GetTemplateFilepath("pre-backup.sh"))
	if err != nil {
		return "", err
	}
	defer pre.Close()

	post, err := os.Open(app.Config.GetTemplateFilepath("post-backup.sh"))
	if err != nil {
		return "", err
	}
	defer post.Close()

	before := time.Now()

	// pre-backup + backup + post-backup
	tasks := []*RunTask{}
	tasks = append(tasks, &RunTask{
		ScriptName:   "pre-backup.sh",
		ScriptReader: pre,
		As:           vm.App.Config.MulchSuperUser,
	})

	for _, confTask := range vm.Config.Backup {
		stream, errG := GetScriptFromURL(confTask.ScriptURL)
		if errG != nil {
			return "", fmt.Errorf("unable to get script '%s': %s", confTask.ScriptURL, errG)
		}
		defer stream.Close()

		task := &RunTask{
			ScriptName:   path.Base(confTask.ScriptURL),
			ScriptReader: stream,
			As:           confTask.As,
		}
		tasks = append(tasks, task)
	}

	tasks = append(tasks, &RunTask{
		ScriptName:   "post-backup.sh",
		ScriptReader: post,
		As:           vm.App.Config.MulchSuperUser,
	})

	run := &Run{
		SSHConn: &SSHConnection{
			User: vm.App.Config.MulchSuperUser,
			Host: vm.LastIP,
			Port: 22,
			Auths: []ssh.AuthMethod{
				SSHSuperUserAuth,
			},
			Log: log,
		},
		Tasks: tasks,
		Log:   log,
	}
	err = run.Go()
	if err != nil {
		return "", err
	}

	// detach backup disk
	// TODO: check if this operation is synchronous for QEMU!
	err = VMDetachBackup(vm.Config.Name, app)
	if err != nil {
		return "", err
	}
	log.Info("backup disk detached")

	err = app.Libvirt.BackupCompress(volName, app.Config.GetTemplateFilepath("volume.xml"), log)
	if err != nil {
		return "", err
	}

	app.BackupsDB.Add(&Backup{
		DiskName: volName,
		Created:  time.Now(),
		VM:       vm,
	})
	after := time.Now()

	log.Infof("backup: %s", after.Sub(before))
	commit = true
	return volName, nil
}

// VMRename will rename the VM in Mulch and in libvirt (including disks)
// Names are Mulch names (not libvirt ones)
// TODO: try to make some sort of transaction here
func VMRename(orgVMName string, newVMName string, app *App, log *Log) error {
	conn, err := app.Libvirt.GetConnection()
	if err != nil {
		return err
	}

	vm, err := app.VMDB.GetByName(orgVMName)
	if err != nil {
		return err
	}

	running, _ := VMIsRunning(orgVMName, app)
	if running == true {
		return errors.New("can't rename a running VM")
	}

	if vm.WIP != VMOperationNone {
		return fmt.Errorf("VM have a work in progress (%s)", string(vm.WIP))
	}

	orgLibvirtName := vm.App.Config.VMPrefix + orgVMName
	newLibvirtName := app.Config.VMPrefix + newVMName

	domain, err := app.Libvirt.GetDomainByName(orgLibvirtName)
	if err != nil {
		return err
	}
	if domain == nil {
		return fmt.Errorf("VM '%s': does not exists in libvirt", orgLibvirtName)
	}
	defer domain.Free()

	xmldoc, err := domain.GetXMLDesc(0)
	if err != nil {
		return err
	}

	domcfg := &libvirtxml.Domain{}
	err = domcfg.Unmarshal(xmldoc)
	if err != nil {
		return err
	}

	newCiName, newDiskName := vmGenVolumesNames(newVMName)

	ciName := ""
	diskName := ""
	for _, disk := range domcfg.Devices.Disks {
		if disk.Alias != nil && disk.Alias.Name == VMStorageAliasCloudInit {
			ciName = path.Base(disk.Source.File.File)
			dir := path.Dir(disk.Source.File.File)
			disk.Source.File.File = path.Clean(dir + "/" + newCiName)
		}
		if disk.Alias != nil && disk.Alias.Name == VMStorageAliasDisk {
			diskName = path.Base(disk.Source.File.File)
			dir := path.Dir(disk.Source.File.File)
			disk.Source.File.File = path.Clean(dir + "/" + newDiskName)
		}
	}

	diskTemplate := app.Config.GetTemplateFilepath("volume.xml")

	ciPool := app.Libvirt.Pools.CloudInit
	ciPoolXML := app.Libvirt.Pools.CloudInitXML

	diskPool := app.Libvirt.Pools.Disks
	diskPoolXML := app.Libvirt.Pools.DisksXML

	if ciName != "" {
		log.Infof("cloning volume '%s'", ciName)
		errC := app.Libvirt.CloneVolume(ciName, ciPool, newCiName, ciPool, ciPoolXML, diskTemplate, log)
		if errC != nil {
			return errC
		}
	}

	if diskName != "" {
		log.Infof("cloning volume '%s'", diskName)
		errC := app.Libvirt.CloneVolume(diskName, diskPool, newDiskName, diskPool, diskPoolXML, diskTemplate, log)
		if errC != nil {
			return errC
		}
	}

	err = app.Libvirt.DeleteVolume(ciName, ciPool)
	if err != nil {
		return err
	}

	err = app.Libvirt.DeleteVolume(diskName, diskPool)
	if err != nil {
		return err
	}

	// rename in libvirt
	domcfg.Name = newLibvirtName

	out, err := domcfg.Marshal()
	if err != nil {
		return err
	}

	// undefine old domain
	err = domain.Undefine()
	if err != nil {
		return err
	}

	// recreate updated domain
	dom2, err := conn.DomainDefineXML(string(out))
	if err != nil {
		return err
	}
	defer dom2.Free()

	// rename in app DB
	err = app.VMDB.Delete(orgVMName)
	if err != nil {
		return err
	}

	vm.Config.Name = newVMName

	err = app.VMDB.Add(vm)
	if err != nil {
		return err
	}

	return nil
}
