package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/OnitiFR/mulch/common"
	"github.com/gofrs/uuid"
	"golang.org/x/crypto/ssh"
	"libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"
)

// Aliases for vm.xml file
const (
	VMStorageAliasDisk   = "ua-mulch-disk"
	VMStorageAliasBackup = "ua-mulch-backup"
	VMNetworkAliasBridge = "ua-mulch-bridge"
)

// VMOperation defines heavy operations in the VM
type VMOperation string

// VMOperation values
const (
	VMOperationNone    = ""
	VMOperationBackup  = "backup"
	VMOperationRestore = "restore"
)

// Backup compression
const (
	BackupCompressAllow   = true
	BackupCompressDisable = false
)

// Backup expiration
const BackupNoExpiration = 0

// New VM : active or inactive
const (
	VMInactive = false
	VMActive   = true
)

// New VM : allow script failures?
const (
	VMStopOnScriptFailure = false // default, safe behavior
	VMAllowScriptFailure  = true
)

// BackupBlankRestore disables *install* scripts during a
// a VM creation (so we can restore backup a bit later)
const BackupBlankRestore = "-"

// VM defines a virtual machine ("domain")
type VM struct {
	App                  *App `json:"-"`
	LibvirtUUID          string
	SecretUUID           string
	Config               *VMConfig
	AuthorKey            string
	MulchSuperUserSSHKey string
	InitDate             time.Time
	LastIP               string
	Locked               bool
	WIP                  VMOperation
	LastRebuildDuration  time.Duration
	LastRebuildDowntime  time.Duration
	AssignedMAC          string
	AssignedIPv4         string
}

// SetOperation change VM WIP
func (vm *VM) SetOperation(op VMOperation) {
	vm.WIP = op
}

// GetSecretsMap returns a map of secrets for a VM
// The map contains all existing secrets, even if err is not nil
func (vm *VM) GetSecretsMap() (map[string]string, error) {
	res := make(map[string]string)

	missing := make([]string, 0)

	for _, secret := range vm.Config.Secrets {
		secretValue, err := vm.App.SecretsDB.Get(secret)
		if err != nil {
			missing = append(missing, secret)
		} else {
			res[secret] = secretValue.Value
		}
	}

	if len(missing) > 0 {
		return res, fmt.Errorf("missing secrets: %s", strings.Join(missing, ", "))
	}

	return res, nil
}

// small helper to generate main disk name
func vmGenDiskName(vmName *VMName) string {
	diskName := vmName.ID() + ".qcow2"
	return diskName
}

// vmGetConsoleDevice returns the console device of a **created** domain
func vmGetConsoleDevice(domain *libvirt.Domain) (string, error) {
	xmldoc, err := domain.GetXMLDesc(0)
	if err != nil {
		return "", err
	}

	domcfg := &libvirtxml.Domain{}
	err = domcfg.Unmarshal(xmldoc)
	if err != nil {
		return "", err
	}

	for _, serial := range domcfg.Devices.Serials {
		if serial.Alias != nil && serial.Alias.Name == "serial0" {
			if serial.Source != nil && serial.Source.Pty != nil {
				return serial.Source.Pty.Path, nil
			}
		}
	}

	return "", errors.New("serial device not found")
}

// NewVM builds a new virtual machine from config
// TODO: this function is HUUUGE and needs to be splitted. It's tricky
// because there's a "transaction" here.
func NewVM(vmConfig *VMConfig, active bool, allowScriptFailure bool, authorKey string, app *App, log *Log) (*VM, *VMName, error) {
	log.Infof("creating new VM '%s'", vmConfig.Name)

	commit := false

	secretUUID, err := uuid.NewV4()
	if err != nil {
		return nil, nil, err
	}

	vm := &VM{
		App:                  app,
		SecretUUID:           secretUUID.String(),
		Config:               vmConfig, // copy()? (deep)
		AuthorKey:            authorKey,
		MulchSuperUserSSHKey: app.Config.MulchSuperUserSSHKey,
		InitDate:             time.Now(),
		Locked:               false,
		WIP:                  VMOperationNone,
	}

	conn, err := app.Libvirt.GetConnection()
	if err != nil {
		return nil, nil, err
	}

	if !IsValidName(vmConfig.Name) {
		return nil, nil, fmt.Errorf("name '%s' is invalid (need only letters, numbers and underscore, do not start with a number)", vmConfig.Name)
	}

	// find next revision
	revision := app.VMDB.GetNextRevisionForName(vmConfig.Name)
	vmName := NewVMName(vmConfig.Name, revision)

	domainName := vmName.LibvirtDomainName(app)

	_, err = conn.LookupDomainByName(domainName)
	if err == nil {
		return nil, nil, fmt.Errorf("VM '%s' already exists in libvirt", domainName)
	}
	errDetails := err.(libvirt.Error)
	if errDetails.Domain != libvirt.FROM_QEMU || errDetails.Code != libvirt.ERR_NO_DOMAIN {
		return nil, nil, fmt.Errorf("unexpected error: %s", err)
	}

	// we assign static DHCP leases for network security reasons (see clean-traffic nwfilter)
	vm.AssignedMAC = RandomUniqueMAC(app)
	vm.AssignedIPv4, err = RandomUniqueIPv4(app)
	if err != nil {
		return nil, nil, err
	}

	app.VMDB.AddToGreenhouse(vm, vmName)
	defer app.VMDB.DeleteFromGreenhouse(vmName)

	diskName := vmGenDiskName(vmName)

	seed, err := app.Seeder.GetByName(vmConfig.Seed)
	if err != nil {
		return nil, nil, err
	}

	if !seed.Ready {
		return nil, nil, fmt.Errorf("seed %s is not ready", vmConfig.Seed)
	}

	if active {
		// check for conclicting domains (will also be done later while saving vm database)
		err = CheckDomainsConflicts(app.VMDB, vmConfig.Domains, vmName.Name, app.Config)
		if err != nil {
			return nil, nil, err
		}
		err = CheckPortsConflicts(app.VMDB, vmConfig.Ports, vmName.Name, log)
		if err != nil {
			return nil, nil, err
		}
	}

	// quick check for missing secrets
	_, err = vm.GetSecretsMap()
	if err != nil {
		return nil, nil, err
	}

	// check if backup exists (if a restore was requested)
	backup := app.BackupsDB.GetByName(vm.Config.RestoreBackup)
	if vm.Config.RestoreBackup != "" {
		if backup == nil && vm.Config.RestoreBackup != BackupBlankRestore {
			return nil, nil, fmt.Errorf("backup '%s' not found in database", vm.Config.RestoreBackup)
		}
		if len(vm.Config.Restore) == 0 {
			return nil, nil, errors.New("no restore script defined for this VM, can't restore")
		}
	}

	SSHSuperUserAuth, err := app.SSHPairDB.GetPublicKeyAuth(vm.MulchSuperUserSSHKey)
	if err != nil {
		return nil, nil, err
	}

	transientLease := &libvirtxml.NetworkDHCPHost{
		Name: vmName.LibvirtDomainName(app),
		MAC:  vm.AssignedMAC,
		IP:   vm.AssignedIPv4,
	}
	app.Libvirt.AddTransientDHCPHost(transientLease, app)
	defer app.Libvirt.RemoveTransientDHCPHost(transientLease, app)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1 - copy from reference image
	log.Infof("creating VM disk '%s'", diskName)
	err = app.Libvirt.CreateDiskFromSeed(
		seed.GetVolumeName(),
		diskName,
		app.Config.GetTemplateFilepath("volume.xml"),
		log)

	if err != nil {
		return nil, nil, err
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
		return nil, nil, err
	}

	// 3 - define domain
	log.Infof("defining vm domain (%s)", domainName)
	xml, err := os.ReadFile(app.Config.GetTemplateFilepath("vm.xml"))
	if err != nil {
		return nil, nil, err
	}

	domcfg := &libvirtxml.Domain{}
	err = domcfg.Unmarshal(string(xml))
	if err != nil {
		return nil, nil, err
	}

	domcfg.Name = domainName

	domcfg.Memory.Unit = "bytes"
	domcfg.Memory.Value = uint(vm.Config.RAMSize)
	domcfg.CurrentMemory.Unit = "bytes"
	domcfg.CurrentMemory.Value = uint(vm.Config.RAMSize)

	domcfg.VCPU.Value = uint(vm.Config.CPUCount)

	serial := "ds=nocloud-net;s=http://" + app.Libvirt.NetworkXML.IPs[0].Address + ":" + strconv.Itoa(app.Config.InternalServerPort) + "/cloud-init/" + vm.SecretUUID + "/"
	serialFound := false
	for s, sysinfo := range domcfg.SysInfo {
		for i, entry := range sysinfo.SMBIOS.System.Entry {
			if entry.Name == "version" {
				domcfg.SysInfo[s].SMBIOS.System.Entry[i].Value = Version
			}
			if entry.Name == "serial" {
				serialFound = true
				domcfg.SysInfo[s].SMBIOS.System.Entry[i].Value = serial
			}
		}
	}
	if !serialFound {
		return nil, nil, errors.New("vm xml file: <sysinfo type='smbios'><system><entry name='serial'> entry not found")
	}

	foundDisks := 0
	for _, disk := range domcfg.Devices.Disks {
		if disk.Alias != nil && disk.Alias.Name == VMStorageAliasDisk {
			disk.Source.File.File = app.Libvirt.Pools.DisksXML.Target.Path + "/" + diskName
			foundDisks++
		}
	}
	if foundDisks != 1 {
		return nil, nil, errors.New("vm xml file: a single disk with 'ua-mulch-disk' alias is required, see sample file")
	}

	foundInterfaces := 0
	for _, intf := range domcfg.Devices.Interfaces {
		if intf.Alias != nil && intf.Alias.Name == VMNetworkAliasBridge {
			// Source and MAC are pointer, we can modify values thru "intf"
			intf.Source.Bridge.Bridge = app.Libvirt.NetworkXML.Bridge.Name
			intf.MAC.Address = vm.AssignedMAC
			if intf.FilterRef == nil {
				return nil, nil, errors.New("vm xml file: no filterref found for network interface")
			}
			if intf.FilterRef.Filter != AppNWFilter {
				return nil, nil, fmt.Errorf("vm xml file: need filterref '%s'", AppNWFilter)
			}
			foundParamIP := 0
			foundParamGateway := 0
			for index, param := range intf.FilterRef.Parameters {
				// Parameters are not pointer, we need to use the index to modify values
				if param.Name == "IP" {
					intf.FilterRef.Parameters[index].Value = vm.AssignedIPv4
					foundParamIP++
				}
				if param.Name == "GATEWAY_MAC" {
					intf.FilterRef.Parameters[index].Value = app.Libvirt.NetworkXML.MAC.Address
					foundParamGateway++
				}
			}
			if foundParamIP != 1 {
				return nil, nil, fmt.Errorf("vm xml file: found %d IP parameter(s) for %s filter, exactly one is needed", foundParamIP, AppNWFilter)
			}
			if foundParamGateway != 1 {
				return nil, nil, fmt.Errorf("vm xml file: found %d GATEWAY_MAC parameter(s) for %s filter, exactly one is needed", foundParamGateway, AppNWFilter)
			}
			foundInterfaces++
		}
	}

	if foundInterfaces != 1 {
		return nil, nil, fmt.Errorf("vm xml file: found %d interface(s) with 'ua-mulch-bridge' alias, exactly one is needed", foundInterfaces)
	}

	xml2, err := domcfg.Marshal()
	if err != nil {
		return nil, nil, err
	}

	dom, err := conn.DomainDefineXML(string(xml2))
	if err != nil {
		return nil, nil, err
	}
	defer dom.Free() // remember: "deferred calls are executed in last-in-first-out order"

	defer func() {
		if !commit {
			log.Infof("rollback, deleting vm %s", vmName)
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
		return nil, nil, err
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
		return nil, nil, err
	}

	console, err := vmGetConsoleDevice(dom)
	if err != nil {
		log.Warningf("can't get console device: %s", err)
	} else {
		log.Infof("console: %s", console)
	}

	err = app.ConsoleManager.AddReader(vmName.ID())
	if err != nil {
		return nil, nil, err
	}

	phone := app.PhoneHome.Register(secretUUID.String())
	defer phone.Unregister()

	for done := false; !done; {
		select {
		case <-time.After(10 * time.Minute):
			return nil, nil, errors.New("vm init is too long, something probably went wrong")
		case call := <-phone.PhoneCalls:
			// seeders already have phone call service, let's filter it out
			if call.CloutInit {
				done = true
				log.Info("vm phoned home, cloud-init was successful")
				vm.LastIP = call.RemoteIP
			}
		case <-time.After(5 * time.Second):
			log.Trace("checking vm state")
			state, _, errG := dom.GetState()
			if errG != nil {
				return nil, nil, errG
			}
			if state == libvirt.DOMAIN_CRASHED {
				return nil, nil, errors.New("vm crashed! (said libvirt)")
			}
			if state == libvirt.DOMAIN_SHUTOFF {
				return nil, nil, errors.New("vm unexpectedly stopped")
			}
		}
	}

	log.Infof("REVISION=%d", revision)

	// 4 - run prepare scripts
	log.Infof("running 'prepare' scripts")
	tasks := []*RunTask{}
	for _, confTask := range vm.Config.Prepare {
		stream, errG := app.Origins.GetContent(confTask.ScriptURL)
		if errG != nil {
			return nil, nil, fmt.Errorf("unable to get script '%s': %s", confTask.ScriptURL, errG)
		}
		defer stream.Close()

		task := &RunTask{
			ScriptName:   path.Base(confTask.ScriptURL),
			ScriptReader: stream,
			As:           confTask.As,
		}
		tasks = append(tasks, task)
	}

	// empty action
	var vmDoAction VMDoAction
	var errDoAction error

	run := &Run{
		Caption: "prepare",
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
		StdoutCallback: func(line string) {
			var isVar bool
			var value string

			if errDoAction != nil {
				return
			}
			if isVar, value = common.StringIsVariable(line, "_MULCH_TAG_ADD"); isVar {
				if !IsValidWord(value) {
					errDoAction = fmt.Errorf("invalid action name '%s'", value)
					return
				}
				_, exists := vm.Config.Tags[value]
				if exists {
					errDoAction = fmt.Errorf("tag '%s' already exists for this VM", value)
					return
				}
				vm.Config.Tags[value] = VMTagFromScript
				log.Infof("tag '%s' added", value)
			}
			if isVar, value = common.StringIsVariable(line, "_MULCH_ACTION_NAME"); isVar {
				vmDoAction.Name = value
			}
			if isVar, value = common.StringIsVariable(line, "_MULCH_ACTION_SCRIPT"); isVar {
				stream, errG := app.Origins.GetContent(value)
				if errG != nil {
					errDoAction = fmt.Errorf("unable to get script '%s': %s", value, errG)
					return
				}
				stream.Close()

				vmDoAction.ScriptURL = value
			}
			if isVar, value = common.StringIsVariable(line, "_MULCH_ACTION_USER"); isVar {
				vmDoAction.User = value
			}
			if isVar, value = common.StringIsVariable(line, "_MULCH_ACTION_DESCRIPTION"); isVar {
				vmDoAction.Description = value
			}
			if isVar, value = common.StringIsVariable(line, "_MULCH_ACTION"); isVar {
				if value != "commit" {
					errDoAction = fmt.Errorf("invalid verb '%s' (only 'commit' is supported)", value)
					return
				}
				if vmDoAction.Name == "" || vmDoAction.User == "" || vmDoAction.ScriptURL == "" {
					errDoAction = fmt.Errorf("invalid action, missing information (need name, user and script)")
					return
				}
				if !IsValidWord(vmDoAction.Name) {
					errDoAction = fmt.Errorf("invalid action name '%s'", vmDoAction.Name)
					return
				}
				_, exists := vm.Config.DoActions[vmDoAction.Name]
				if exists {
					errDoAction = fmt.Errorf("action '%s' already exists for this VM", vmDoAction.Name)
					return
				}

				// add action
				newAction := vmDoAction // duplicate
				newAction.FromConfig = false
				vm.Config.DoActions[vmDoAction.Name] = &newAction
				log.Infof("action '%s' added", vmDoAction.Name)

				// reset action object
				vmDoAction = VMDoAction{}
			}
		},
	}
	err = run.Go(ctx)
	if err != nil {
		if !allowScriptFailure {
			return nil, nil, err
		}
		log.Error(err.Error())
	}

	if errDoAction != nil {
		return nil, nil, fmt.Errorf("can't add do action: %s", errDoAction)
	}

	if vm.Config.RestoreBackup != "" {
		if vm.Config.RestoreBackup != BackupBlankRestore {
			// 5a - restore backup
			err = VMRestoreNoChecks(vm, vmName, backup, app, log)
			if err != nil {
				if !allowScriptFailure {
					return nil, nil, err
				}
				log.Error(err.Error())
			}
		}
	} else {
		// 5b - run install scripts
		log.Infof("running 'install' scripts")
		tasks := []*RunTask{}
		for _, confTask := range vm.Config.Install {
			stream, errG := app.Origins.GetContent(confTask.ScriptURL)
			if errG != nil {
				return nil, nil, fmt.Errorf("unable to get script '%s': %s", confTask.ScriptURL, errG)
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
			Caption: "install",
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
			StdoutCallback: func(line string) {
				// 'install' step is not called during a rebuild
				if isVar, _ := common.StringIsVariable(line, "_MULCH_ACTION"); isVar {
					log.Warningf("Ignored: actions are supported only for 'prepare' scripts")
				}
			},
		}
		err = run.Go(ctx)
		if err != nil {
			if !allowScriptFailure {
				return nil, nil, err
			}
			log.Error(err.Error())
		}
	}

	// all is OK, commit (= no defer) and save vm to DB
	log.Infof("saving VM in database")
	err = app.VMDB.Add(vm, vmName, active)
	if err != nil {
		return nil, nil, err
	}
	commit = true
	return vm, vmName, nil
}

// VMGetDiskName return VM's disk filename
func VMGetDiskName(name *VMName, app *App) (string, error) {
	domain, err := app.Libvirt.GetDomainByName(name.LibvirtDomainName(app))
	if err != nil {
		return "", err
	}
	if domain == nil {
		return "", fmt.Errorf("VM '%s': does not exists in libvirt", name)
	}
	defer domain.Free()

	xmldoc, err := domain.GetXMLDesc(0)
	if err != nil {
		return "", err
	}

	domcfg := &libvirtxml.Domain{}
	err = domcfg.Unmarshal(xmldoc)
	if err != nil {
		return "", err
	}

	diskName := ""
	for _, disk := range domcfg.Devices.Disks {
		if disk.Alias != nil && disk.Alias.Name == VMStorageAliasDisk {
			diskName = path.Base(disk.Source.File.File)
		}
	}
	if diskName == "" {
		return "", fmt.Errorf("disk with '%s' alias not found", VMStorageAliasDisk)
	}

	return diskName, nil
}

// VMStopByName stops a VM using its name and waits until the VM is down. (or timeouts)
func VMStopByName(name *VMName, app *App, log *Log) error {
	domain, err := app.Libvirt.GetDomainByName(name.LibvirtDomainName(app))
	if err != nil {
		return err
	}
	if domain == nil {
		return fmt.Errorf("VM %s: does not exists in libvirt", name)
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
	for done := false; !done; {
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

// VMStartByName starts a VM using its name
// and waits until the VM phones home. (or timeouts)
func VMStartByName(name *VMName, secretUUID string, app *App, log *Log) error {
	domain, err := app.Libvirt.GetDomainByName(name.LibvirtDomainName(app))
	if err != nil {
		return err
	}
	if domain == nil {
		return fmt.Errorf("VM %s: does not exists in libvirt", name)
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

	log.Infof("starting %s", name)

	phone := app.PhoneHome.Register(secretUUID)
	defer phone.Unregister()

	err = domain.Create()
	if err != nil {
		return err
	}

	err = app.ConsoleManager.AddReader(name.ID())
	if err != nil {
		return err
	}

	log.Infof("started, waiting phone call from %s", name)

	for done := false; !done; {
		select {
		case <-time.After(10 * time.Minute):
			return fmt.Errorf("vm is too long to start, something probably went wrong (%s)", name)
		case <-phone.PhoneCalls:
			done = true
			log.Infof("vm %s phoned home", name)
		}
	}

	return nil
}

// VMLockUnlock will lock or unlock a VM, preventing it from deletion
func VMLockUnlock(vmName *VMName, locked bool, vmdb *VMDatabase) error {
	vm, err := vmdb.GetByName(vmName)
	if err != nil {
		return err
	}

	vm.Locked = locked
	vmdb.Update()
	return nil
}

// VMDelete will delete a VM (using its name) and linked storages.
func VMDelete(vmName *VMName, app *App, log *Log) error {
	vm, err := app.VMDB.GetByName(vmName)
	if err != nil {
		return err
	}

	if vm.Locked {
		return errors.New("VM is locked (see 'unlock' command)")
	}

	libvirtName := vmName.LibvirtDomainName(app)
	domain, err := app.Libvirt.GetDomainByName(libvirtName)
	if err != nil {
		return err
	}
	if domain == nil {
		return fmt.Errorf("VM %s: does not exists in libvirt", libvirtName)
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
			return errors.New("unable to force stop (destroy) the VM")
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

	diskName := ""
	for _, disk := range domcfg.Devices.Disks {
		if disk.Alias != nil && disk.Alias.Name == VMStorageAliasDisk {
			diskName = path.Base(disk.Source.File.File)
		}
	}

	// Casual refresh, without any error checking. Alacool.
	app.Libvirt.Pools.Disks.Refresh(0)

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

	errR := app.Libvirt.RebuildDHCPStaticLeases(app)
	if errR != nil {
		return errR
	}

	return nil
}

// VMIsRunning returns true if VM is up and running
func VMIsRunning(vmName *VMName, app *App) (bool, error) {
	dom, err := app.Libvirt.GetDomainByName(vmName.LibvirtDomainName(app))
	if err != nil {
		return false, err
	}
	if dom == nil {
		return false, fmt.Errorf("can't find domain for %s", vmName)
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
func VMCreateBackupDisk(vmName *VMName, volName string, volSize uint64, app *App, log *Log) error {
	dom, err := app.Libvirt.GetDomainByName(vmName.LibvirtDomainName(app))
	if err != nil {
		return err
	}
	if dom == nil {
		return fmt.Errorf("can't find domain for %s", vmName)
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
func VMAttachBackup(vmName *VMName, volName string, app *App) error {
	dom, err := app.Libvirt.GetDomainByName(vmName.LibvirtDomainName(app))
	if err != nil {
		return err
	}
	if dom == nil {
		return fmt.Errorf("can't find domain for %s", vmName)
	}
	defer dom.Free()

	xml, err := os.ReadFile(app.Config.GetTemplateFilepath("disk.xml"))
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
func VMDetachBackup(vmName *VMName, app *App) error {
	dom, err := app.Libvirt.GetDomainByName(vmName.LibvirtDomainName(app))
	if err != nil {
		return err
	}
	if dom == nil {
		return fmt.Errorf("can't find domain for %s", vmName)
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

	if !found {
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

// VMBackup launch the backup process (returns backup filename)
func VMBackup(vmName *VMName, authorKey string, app *App, log *Log, compressAllow bool, expire time.Duration) (string, error) {
	vm, err := app.VMDB.GetByName(vmName)
	if err != nil {
		return "", err
	}

	if vm.WIP != VMOperationNone {
		return "", fmt.Errorf("VM already have a work in progress (%s)", string(vm.WIP))
	}

	vm.SetOperation(VMOperationBackup)
	defer vm.SetOperation(VMOperationNone)

	running, _ := VMIsRunning(vmName, app)
	if !running {
		return "", errors.New("VM should be up and running to do a backup")
	}

	if len(vm.Config.Backup) == 0 {
		return "", errors.New("no backup script defined for this VM")
	}

	volName := fmt.Sprintf("%s-backup-%s.qcow2",
		vmName.ID(),
		time.Now().Format("20060102-150405"),
	)

	if app.BackupsDB.GetByName(volName) != nil {
		return "", fmt.Errorf("a backup with the same name already exists (%s)", volName)
	}

	SSHSuperUserAuth, err := app.SSHPairDB.GetPublicKeyAuth(vm.MulchSuperUserSSHKey)
	if err != nil {
		return "", err
	}

	before := time.Now()

	err = VMCreateBackupDisk(vmName, volName, vm.Config.BackupDiskSize, app, log)
	if err != nil {
		return "", err
	}

	// NOTE: this attachement is transient
	err = VMAttachBackup(vmName, volName, app)
	if err != nil {
		return "", err
	}
	log.Info("backup disk attached")

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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// defer detach + vol delete in case of failure
	commit := false
	defer func() {
		if !commit {
			log.Info("force post-backup")
			tasks := []*RunTask{}
			tasks = append(tasks, &RunTask{
				ScriptName:   "post-backup.sh",
				ScriptReader: post,
				As:           vm.App.Config.MulchSuperUser,
			})
			run := &Run{
				Caption: "",
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
			errRun := run.Go(ctx)
			if errRun != nil {
				log.Errorf("failed post-backup: %s", errRun)
				// continue anyway, it's not fatal
			}

			log.Info("rollback backup disk creation")
			errDet := VMDetachBackup(vmName, app)
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

	// pre-backup + backup + post-backup
	tasks := []*RunTask{}
	tasks = append(tasks, &RunTask{
		ScriptName:   "pre-backup.sh",
		ScriptReader: pre,
		As:           vm.App.Config.MulchSuperUser,
	})

	for _, confTask := range vm.Config.Backup {
		stream, errG := app.Origins.GetContent(confTask.ScriptURL)
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
		Caption: "backup",
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
	err = run.Go(ctx)
	if err != nil {
		return "", err
	}

	// detach backup disk
	// TODO: check if this operation is synchronous for QEMU!
	err = VMDetachBackup(vmName, app)
	if err != nil {
		return "", err
	}
	log.Info("backup disk detached")

	if vm.Config.BackupCompress && compressAllow {
		err = app.Libvirt.BackupCompress(
			volName,
			app.Config.GetTemplateFilepath("volume.xml"),
			app.Config.TempPath,
			log)
		if err != nil {
			return "", err
		}
	}

	backup := &Backup{
		DiskName:  volName,
		Created:   time.Now(),
		AuthorKey: authorKey,
		VM:        vm,
	}

	if expire > BackupNoExpiration {
		backup.Expire = time.Now().Add(expire)
		log.Warningf("backup will expire on %s", backup.Expire.Format(time.RFC3339))
	}

	app.BackupsDB.Add(backup)
	after := time.Now()

	log.Infof("BACKUP=%s", volName)
	log.Infof("backup: %s", after.Sub(before))
	commit = true
	return volName, nil
}

// VMRestoreNoChecks launch the restore process, this function is a symetric
// of VMBackup, since a few checks are missing because it's supposed to be
// called -during VM creation- (and not after)
func VMRestoreNoChecks(vm *VM, vmName *VMName, backup *Backup, app *App, log *Log) error {
	vm.SetOperation(VMOperationRestore)
	defer vm.SetOperation(VMOperationNone)

	if len(vm.Config.Restore) == 0 {
		return errors.New("no restore script defined for this VM")
	}

	// 6 - restore
	log.Infof("restoring from '%s'", backup.DiskName)

	before := time.Now()

	// attach backup
	err := VMAttachBackup(vmName, backup.DiskName, app)
	if err != nil {
		return err
	}
	defer func() {
		// detach backup
		err = VMDetachBackup(vmName, app)
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
		return errO
	}
	defer pre.Close()

	post, errO := os.Open(app.Config.GetTemplateFilepath("post-restore.sh"))
	if errO != nil {
		return errO
	}
	defer post.Close()

	SSHSuperUserAuth, err := app.SSHPairDB.GetPublicKeyAuth(vm.MulchSuperUserSSHKey)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tasks := []*RunTask{}
	tasks = append(tasks, &RunTask{
		ScriptName:   "pre-restore.sh",
		ScriptReader: pre,
		As:           vm.App.Config.MulchSuperUser,
	})

	for _, confTask := range vm.Config.Restore {
		stream, errG := app.Origins.GetContent(confTask.ScriptURL)
		if errG != nil {
			return fmt.Errorf("unable to get script '%s': %s", confTask.ScriptURL, errG)
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
		Caption: "restore",
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
	err = run.Go(ctx)
	if err != nil {
		return err
	}
	log.Info("restore completed")

	after := time.Now()
	log.Infof("restore: %s", after.Sub(before))
	return nil
}

// VMRename will rename the VM in Mulch and in libvirt (including disks)
// TODO: try to make some sort of transaction here
// WARNING: currently not used (old rebuild system) so… unproven code.
func VMRename(orgVMName *VMName, newVMName *VMName, app *App, log *Log) error {
	conn, err := app.Libvirt.GetConnection()
	if err != nil {
		return err
	}

	vm, err := app.VMDB.GetByName(orgVMName)
	if err != nil {
		return err
	}

	if found, _ := app.VMDB.GetByName(newVMName); found != nil {
		return fmt.Errorf("VM %s already exists", newVMName)
	}

	running, _ := VMIsRunning(orgVMName, app)
	if running {
		return errors.New("can't rename a running VM")
	}

	if vm.WIP != VMOperationNone {
		return fmt.Errorf("VM have a work in progress (%s)", string(vm.WIP))
	}

	orgLibvirtName := orgVMName.LibvirtDomainName(app)
	newLibvirtName := newVMName.LibvirtDomainName(app)

	domain, err := app.Libvirt.GetDomainByName(orgLibvirtName)
	if err != nil {
		return err
	}
	if domain == nil {
		return fmt.Errorf("VM %s: does not exists in libvirt", orgLibvirtName)
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

	newDiskName := vmGenDiskName(newVMName)

	diskName := ""
	for _, disk := range domcfg.Devices.Disks {
		if disk.Alias != nil && disk.Alias.Name == VMStorageAliasDisk {
			diskName = path.Base(disk.Source.File.File)
			dir := path.Dir(disk.Source.File.File)
			disk.Source.File.File = path.Clean(dir + "/" + newDiskName)
		}
	}

	diskTemplate := app.Config.GetTemplateFilepath("volume.xml")

	diskPool := app.Libvirt.Pools.Disks
	diskPoolXML := app.Libvirt.Pools.DisksXML

	if diskName != "" {
		log.Infof("cloning volume '%s'", diskName)
		errC := app.Libvirt.CloneVolume(diskName, diskPool, newDiskName, diskPool, diskPoolXML, diskTemplate, log)
		if errC != nil {
			return errC
		}
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

	active, err := app.VMDB.IsVMActive(orgVMName)
	if err != nil {
		return err
	}

	// rename in app DB
	err = app.VMDB.Delete(orgVMName)
	if err != nil {
		return err
	}

	// the Delete() may have set a previous VM as active. It's bad
	// because the Add() below will fail if active is true.
	if active {
		// an error is non-fatal for us (no previous active VM, for instance)
		app.VMDB.SetActiveRevision(orgVMName.Name, RevisionNone)
	}

	vm.Config.Name = newVMName.Name

	err = app.VMDB.Add(vm, newVMName, active)
	if err != nil {
		return err
	}

	return nil
}

// VMRebuild delete VM and rebuilds it from a backup (using revisions)
func VMRebuild(vmName *VMName, lock bool, authorKey string, app *App, log *Log) error {
	rebuildStart := time.Now()

	entry, err := app.VMDB.GetEntryByName(vmName)
	if err != nil {
		return err
	}
	vm := entry.VM

	if vm.WIP != VMOperationNone {
		return fmt.Errorf("VM already have a work in progress (%s)", string(vm.WIP))
	}

	if len(vm.Config.Restore) > 0 && len(vm.Config.Backup) == 0 {
		return errors.New("restore script(s) defined but no backup script found")
	}
	if len(vm.Config.Backup) > 0 && len(vm.Config.Restore) == 0 {
		return errors.New("backup script(s) defined but no restore script found")
	}

	backupAndRestore := true
	if len(vm.Config.Restore) == 0 && len(vm.Config.Backup) == 0 {
		// simple rebuild, without any data
		backupAndRestore = false
	}

	if vm.WIP != VMOperationNone {
		return fmt.Errorf("VM have a work in progress (%s)", string(vm.WIP))
	}

	running, _ := VMIsRunning(vmName, app)
	if !running {
		return errors.New("VM should be up and running")
	}

	configFile := vm.Config.FileContent

	conf, err := NewVMConfigFromTomlReader(strings.NewReader(configFile), app)
	if err != nil {
		return fmt.Errorf("decoding config: %s", err)
	}

	if backupAndRestore {
		conf.RestoreBackup = BackupBlankRestore
	} else {
		conf.RestoreBackup = ""
	}

	success := false

	// create VM rev+1
	// replace original VM author with "rebuilder"
	newVM, newVMName, err := NewVM(conf, false, VMStopOnScriptFailure, authorKey, app, log)
	if err != nil {
		log.Error(err.Error())
		return fmt.Errorf("cannot create VM: %s", err)
	}

	defer func() {
		if !success {
			err = VMDelete(newVMName, app, log)
			if err != nil {
				log.Error(err.Error())
			}
		}
	}()

	sourceIsActive := entry.Active

	downtimeStart := time.Now()

	if sourceIsActive {
		// set rev+0 as inactive ("default" behavior, add a --no-downtime flag?)
		err = app.VMDB.SetActiveRevision(vmName.Name, RevisionNone)
		if err != nil {
			return fmt.Errorf("can't disable all revisions: %s", err)
		}

		defer func() {
			if !success {
				err = app.VMDB.SetActiveRevision(vmName.Name, vmName.Revision)
				if err != nil {
					log.Error(err.Error())
				}
			}
		}()
	}

	if backupAndRestore {
		// backup rev+0
		backupName, err := VMBackup(vmName, authorKey, app, log, BackupCompressDisable, BackupNoExpiration)
		if err != nil {
			return fmt.Errorf("creating backup: %s", err)
		}

		defer func() {
			// -always- delete backup (success or not)
			err = app.BackupsDB.Delete(backupName)
			if err != nil {
				// not a "real" error
				log.Errorf("unable remove '%s' backup from DB: %s", backupName, err)
			} else {
				err = app.Libvirt.DeleteVolume(backupName, app.Libvirt.Pools.Backups)
				if err != nil {
					// not a "real" error
					log.Errorf("unable remove '%s' backup from storage: %s", backupName, err)
				}
			}
		}()

		backup := app.BackupsDB.GetByName(backupName)
		if backup == nil {
			return fmt.Errorf("can't find backup '%s' in DB", backupName)
		}

		// restore rev+1
		err = VMRestoreNoChecks(newVM, newVMName, backup, app, log)
		if err != nil {
			return fmt.Errorf("restoring backup: %s", err)
		}
	}

	if sourceIsActive {
		// activate rev+1
		err = app.VMDB.SetActiveRevision(newVMName.Name, newVMName.Revision)
		if err != nil {
			return fmt.Errorf("can't enable new revision: %s", err)
		}
		log.Infof("VM %s is now active", newVMName)
	}
	downtimeEnd := time.Now()

	// get lock status of original VM
	originalLocked := vm.Locked
	err = VMLockUnlock(vmName, false, app.VMDB)
	if err != nil {
		return fmt.Errorf("unlocking original VM: %s", err)
	}

	// - delete rev+0 VM
	err = VMDelete(vmName, app, log)
	if err != nil {
		return fmt.Errorf("delete original VM: %s", err)
	}

	// commit (too late to rollback, original VM does not exists anymore)
	success = true

	if lock || originalLocked {
		err := VMLockUnlock(newVMName, true, app.VMDB)
		if err != nil {
			log.Failuref("unable to lock '%s': %s", vmName, err)
			return nil
		}
		log.Info("VM locked")
	}

	rebuildEnd := time.Now()

	downtime := downtimeEnd.Sub(downtimeStart)
	rebuildtime := rebuildEnd.Sub(rebuildStart)

	newVM.LastRebuildDowntime = downtime
	newVM.LastRebuildDuration = rebuildtime
	app.VMDB.Update()

	if sourceIsActive {
		log.Infof("downtime: %s", downtime)
	} else {
		log.Infof("downtime: none (was not active)")
	}

	return nil
}
