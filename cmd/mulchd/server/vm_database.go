package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"

	"github.com/OnitiFR/mulch/common"
)

// RevisionNone means… none (see SetActiveRevision for instance)
const RevisionNone = -1

type updateCallback func()

// VMDatabaseEntry is an entry in the DB: a name and a VM
// Only one entry can be active per name
type VMDatabaseEntry struct {
	Name   *VMName
	VM     *VM
	Active bool
}

// VMDatabase describes a persistent DataBase of VMs structures
// ---
// It includes a greenhouse, where all new VM (= currently building)
// are stored. This transient database is not stored on disk.
// (this DB is used by GetBySecretUUID, for instance)
type VMDatabase struct {
	filename       string
	domainFilename string
	portFilename   string
	db             map[string]*VMDatabaseEntry
	greenhouseDB   map[string]*VMDatabaseEntry
	mutex          sync.Mutex
	onUpdate       updateCallback
	app            *App
}

// NewVMDatabase instanciates a new VMDatabase
func NewVMDatabase(filename string, domainFilename string, portFilename string, onUpdate updateCallback, app *App) (*VMDatabase, error) {
	vmdb := &VMDatabase{
		filename:       filename,
		domainFilename: domainFilename,
		portFilename:   portFilename,
		db:             make(map[string]*VMDatabaseEntry),
		greenhouseDB:   make(map[string]*VMDatabaseEntry),
		onUpdate:       onUpdate,
		app:            app,
	}

	// if the file exists, load it
	if _, err := os.Stat(vmdb.filename); err == nil {
		err = vmdb.load()
		if err != nil {
			return nil, err
		}
	}

	// save the file to check if it's writable
	err := vmdb.save()
	if err != nil {
		return nil, err
	}

	return vmdb, nil
}

// build domain database, updated with each vm.LastIP (and name, as it's not
// available at config file reading time)
func (vmdb *VMDatabase) genDomainsDB() error {
	domains := make(map[string]*common.Domain)

	for _, entry := range vmdb.db {
		if !entry.Active {
			continue
		}
		vm := entry.VM
		for _, domain := range vm.Config.Domains {
			domain.VMName = entry.Name.ID()
			if domain.RedirectTo == "" {
				domain.DestinationHost = vm.LastIP
			}

			otherDomain, exist := domains[domain.Name]
			if exist {
				return fmt.Errorf("domain '%s' is duplicated in '%s' and '%s' VMs", domain.Name, otherDomain.VMName, domain.VMName)
			}

			domains[domain.Name] = domain
		}
	}

	f, err := os.OpenFile(vmdb.domainFilename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	err = enc.Encode(&domains)
	if err != nil {
		return err
	}

	return nil
}

// build port database for the TCP proxy
func (vmdb *VMDatabase) genPortsDB() error {
	var vmEntries []*VMDatabaseEntry
	exportedPortMap := make(map[string]*VM)

	for _, entry := range vmdb.greenhouseDB {
		vmEntries = append(vmEntries, entry)
	}
	for _, entry := range vmdb.db {
		// we currently allow an inactive VM to connect to other ports
		// (since we allow it during its construction)
		vmEntries = append(vmEntries, entry)
	}

	// build a map of all exported ports
	for _, entry := range vmEntries {
		if !entry.Active {
			continue
		}
		vm := entry.VM
		for _, p := range vm.Config.Ports {
			if p.Direction == VMPortDirectionExport && p.PublicPort == 0 {
				exportedPortMap[p.String()] = vm
			}
		}
	}

	listeners := make(common.TCPPortListeners)
	gateway := vmdb.app.Libvirt.NetworkXML.IPs[0].Address

	for _, entry := range vmEntries {
		vm := entry.VM
		for _, p := range vm.Config.Ports {
			if p.Protocol != VMPortProtocolTCP {
				return fmt.Errorf("unsupported protocol '%s'", p.String())
			}

			if p.Direction != VMPortDirectionImport {
				continue
			}

			// who exports this port?
			exportedPort := *p
			exportedPort.Direction = VMPortDirectionExport
			exportingVM, found := exportedPortMap[exportedPort.String()]
			if !found {
				vmdb.app.Log.Tracef("unable to found port '%s' for VM '%s'", exportedPort.String(), vm.Config.Name)
				continue
			}

			// add to the correct listener
			listenPort := VMPortBaseForward + uint16(p.Index)
			listener, exists := listeners[listenPort]
			if !exists {
				listenStr := fmt.Sprintf("%s:%d", gateway, listenPort)
				listenAddr, err := net.ResolveTCPAddr("tcp", listenStr)
				if err != nil {
					return err
				}
				listener = &common.TCPPortListener{
					ListenAddr: listenAddr,
					Forwards:   make(map[string]*common.TCPForwarder),
				}
				listeners[listenPort] = listener
			}

			forwardStr := fmt.Sprintf("%s:%d", exportingVM.AssignedIPv4, p.Port)
			forwardAddr, err := net.ResolveTCPAddr("tcp", forwardStr)
			if err != nil {
				return err
			}
			listener.Forwards[vm.AssignedIPv4] = &common.TCPForwarder{
				Dest: forwardAddr,
			}
		}
	}

	// add listeners for public ports
	for _, entry := range vmEntries {
		if !entry.Active {
			continue
		}
		vm := entry.VM
		for _, p := range vm.Config.Ports {
			if p.PublicPort != 0 {
				listenStr := fmt.Sprintf(":%d", p.PublicPort)
				listenAddr, err := net.ResolveTCPAddr("tcp", listenStr)
				if err != nil {
					return err
				}
				listener := &common.TCPPortListener{
					ListenAddr: listenAddr,
					Forwards:   make(map[string]*common.TCPForwarder),
				}

				_, exists := listeners[p.PublicPort]
				if exists {
					return fmt.Errorf("public port %d is duplicated", p.PublicPort)
				}
				listeners[p.PublicPort] = listener

				forwardStr := fmt.Sprintf("%s:%d", vm.AssignedIPv4, p.Port)
				forwardAddr, err := net.ResolveTCPAddr("tcp", forwardStr)
				if err != nil {
					return err
				}
				listener.Forwards["*"] = &common.TCPForwarder{
					Dest:           forwardAddr,
					PROXYProtoPort: p.ProxyPort,
				}
			}
		}
	}

	f, err := os.OpenFile(vmdb.portFilename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	err = enc.Encode(&listeners)
	if err != nil {
		return err
	}

	return nil
}

// This is done internaly, because it must be done with the mutex locked,
// but we can't lock it here, since save() is called by functions that
// are already locking the mutex.
func (vmdb *VMDatabase) save() error {
	f, err := os.OpenFile(vmdb.filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	err = enc.Encode(&vmdb.db)
	if err != nil {
		return err
	}

	err = vmdb.genDomainsDB()
	if err != nil {
		return err
	}

	err = vmdb.genPortsDB()
	if err != nil {
		return err
	}

	if vmdb.onUpdate != nil {
		vmdb.onUpdate()
	}

	return nil
}

func (vmdb *VMDatabase) load() error {
	f, err := os.Open(vmdb.filename)
	if err != nil {
		return err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return err
	}

	requiredMode, err := strconv.ParseInt("0600", 8, 32)
	if err != nil {
		return err
	}

	if stat.Mode() != os.FileMode(requiredMode) {
		return fmt.Errorf("%s: only the owner should be able to read/write this file (mode 0600)", vmdb.filename)
	}

	dec := json.NewDecoder(f)
	err = dec.Decode(&vmdb.db)
	if err != nil {
		return err
	}
	return nil
}

// Update saves the DB if data was modified using *VM pointers
func (vmdb *VMDatabase) Update() error {
	vmdb.mutex.Lock()
	defer vmdb.mutex.Unlock()

	return vmdb.save()
}

// Delete the VM from the database using its name
func (vmdb *VMDatabase) Delete(name *VMName) error {
	entryToDelete, err := vmdb.GetEntryByName(name)
	if err != nil {
		return fmt.Errorf("VM '%s' was not found in database", name.ID())
	}

	if entryToDelete.Active {
		// set "highest" instance active (if any)
		maxRevision := -1
		vmNames := vmdb.GetNames()

		for _, vmName := range vmNames {
			entry, err := vmdb.GetEntryByName(vmName)
			if err != nil {
				return err
			}
			if entry.Name.Name == entryToDelete.Name.Name &&
				entry.Name.Revision != entryToDelete.Name.Revision &&
				entry.Name.Revision > maxRevision {
				maxRevision = entry.Name.Revision
			}
		}

		if maxRevision >= 0 {
			err := vmdb.SetActiveRevision(entryToDelete.Name.Name, maxRevision)
			if err != nil {
				return err
			}
		}
	}

	vmdb.mutex.Lock()
	defer vmdb.mutex.Unlock()

	delete(vmdb.db, name.ID())

	err = vmdb.save()
	if err != nil {
		return err
	}

	return nil
}

// Add a new VM in the database
func (vmdb *VMDatabase) Add(vm *VM, name *VMName, active bool) error {

	if active {
		err := CheckDomainsConflicts(vmdb, vm.Config.Domains, name.Name, vmdb.app.Config)
		if err != nil {
			return err
		}
		err = CheckPortsConflicts(vmdb, vm.Config.Ports, name.Name, nil)
		if err != nil {
			return err
		}
	}

	vmdb.mutex.Lock()
	defer vmdb.mutex.Unlock()

	if _, exists := vmdb.db[name.ID()]; exists {
		return fmt.Errorf("VM %s already exists in database", name)
	}

	if active {
		// set any other instance as inactive
		for _, entry := range vmdb.db {
			if entry.Name.Name == name.Name {
				entry.Active = false
			}
		}
	}

	entry := &VMDatabaseEntry{
		Name:   name,
		VM:     vm,
		Active: active,
	}

	vmdb.db[name.ID()] = entry
	err := vmdb.save()
	if err != nil {
		// we may have deactivate a previous VM, though :/
		delete(vmdb.db, name.ID())
		return err
	}
	return nil
}

// DeleteFromGreenhouse the VM from the greenhouse database using its name
func (vmdb *VMDatabase) DeleteFromGreenhouse(name *VMName) error {
	vmdb.mutex.Lock()
	defer vmdb.mutex.Unlock()

	_, exists := vmdb.greenhouseDB[name.ID()]
	if !exists {
		return fmt.Errorf("VM '%s' was not found in greenhouse database", name.ID())
	}

	delete(vmdb.greenhouseDB, name.ID())

	return nil
}

// AddToGreenhouse a new VM in the greenhouse database
func (vmdb *VMDatabase) AddToGreenhouse(vm *VM, name *VMName) error {
	vmdb.mutex.Lock()
	defer vmdb.mutex.Unlock()

	if _, exists := vmdb.greenhouseDB[name.ID()]; exists {
		return fmt.Errorf("VM %s already exists in greenhouse database", name)
	}

	entry := &VMDatabaseEntry{
		Name:   name,
		VM:     vm,
		Active: false,
	}

	vmdb.greenhouseDB[name.ID()] = entry
	err := vmdb.save()
	if err != nil {
		return err
	}
	return nil
}

// GetNames of all VMs in the database
func (vmdb *VMDatabase) GetNames() []*VMName {
	vmdb.mutex.Lock()
	defer vmdb.mutex.Unlock()

	names := make([]*VMName, 0, len(vmdb.db))
	for _, entry := range vmdb.db {
		names = append(names, entry.Name)
	}
	return names
}

// GetGreenhouseNames return all VMs in the greenhouse database
func (vmdb *VMDatabase) GetGreenhouseNames() []*VMName {
	vmdb.mutex.Lock()
	defer vmdb.mutex.Unlock()

	names := make([]*VMName, 0, len(vmdb.greenhouseDB))
	for _, entry := range vmdb.greenhouseDB {
		names = append(names, entry.Name)
	}
	return names
}

// GetEntryByName lookups a VMDatabaseEntry entry by its name
func (vmdb *VMDatabase) GetEntryByName(name *VMName) (*VMDatabaseEntry, error) {
	vmdb.mutex.Lock()
	defer vmdb.mutex.Unlock()

	entry, exists := vmdb.db[name.ID()]
	if !exists {
		return nil, fmt.Errorf("VM %s not found in database", name)
	}
	return entry, nil
}

// GetByName lookups a VM by its name
func (vmdb *VMDatabase) GetByName(name *VMName) (*VM, error) {
	entry, err := vmdb.GetEntryByName(name)
	if err != nil {
		return nil, err
	}
	return entry.VM, nil
}

// GetByNameID lookups a VM by its name-id (low-level, should not use)
func (vmdb *VMDatabase) GetByNameID(id string) (*VM, error) {
	vmdb.mutex.Lock()
	defer vmdb.mutex.Unlock()

	entry, exists := vmdb.db[id]
	if !exists {
		return nil, fmt.Errorf("VM id %s not found in database", id)
	}
	return entry.VM, nil
}

// GetActiveEntryByName return the active VM entry with the specified name
func (vmdb *VMDatabase) GetActiveEntryByName(name string) (*VMDatabaseEntry, error) {
	vmdb.mutex.Lock()
	defer vmdb.mutex.Unlock()

	maxRevision := -1
	for _, entry := range vmdb.db {
		if entry.Name.Name == name && entry.Name.Revision > maxRevision && entry.Active {
			maxRevision = entry.Name.Revision
		}
	}

	if maxRevision == -1 {
		return nil, fmt.Errorf("active VM %s not found in database", name)
	}

	vmName := NewVMName(name, maxRevision)
	entry := vmdb.db[vmName.ID()]
	return entry, nil
}

// GetActiveByName return the active VM with the specified name
func (vmdb *VMDatabase) GetActiveByName(name string) (*VM, error) {
	entry, err := vmdb.GetActiveEntryByName(name)
	if err != nil {
		return nil, err
	}
	return entry.VM, nil
}

// GetGreenhouseEntryByName lookups a VMDatabaseEntry in greenhouseDB entry by its name
func (vmdb *VMDatabase) GetGreenhouseEntryByName(name *VMName) (*VMDatabaseEntry, error) {
	vmdb.mutex.Lock()
	defer vmdb.mutex.Unlock()

	entry, exists := vmdb.greenhouseDB[name.ID()]
	if !exists {
		return nil, fmt.Errorf("VM %s not found in greenhouse database", name)
	}
	return entry, nil
}

// SearchGreenhouseEntries lists all VMs in the greenhouse matching the specified name
func (vmdb *VMDatabase) SearchGreenhouseEntries(name string) []*VMDatabaseEntry {
	vmdb.mutex.Lock()
	defer vmdb.mutex.Unlock()

	entries := make([]*VMDatabaseEntry, 0)
	for _, entry := range vmdb.greenhouseDB {
		if entry.Name.Name == name {
			entries = append(entries, entry)
		}
	}
	return entries
}

// GetEntryBySecretUUID lookups a VMName by its secretUUID
// Note: this function also search in greenhouseDB
func (vmdb *VMDatabase) GetEntryBySecretUUID(uuid string) (*VMDatabaseEntry, error) {
	vmdb.mutex.Lock()
	defer vmdb.mutex.Unlock()

	anon := "?"
	if len(uuid) > 4 {
		anon = uuid[:4] + "…"
	}

	for _, entry := range vmdb.db {
		if entry.VM.SecretUUID == uuid {
			return entry, nil
		}
	}

	for _, entry := range vmdb.greenhouseDB {
		if entry.VM.SecretUUID == uuid {
			return entry, nil
		}
	}

	return nil, fmt.Errorf("UUID '%s' was not found in database", anon)
}

// GetBySecretUUID lookups a VM by its secretUUID
// Note: this function also search in greenhouseDB
func (vmdb *VMDatabase) GetBySecretUUID(uuid string) (*VM, error) {
	entry, err := vmdb.GetEntryBySecretUUID(uuid)
	if err != nil {
		return nil, err
	}
	return entry.VM, nil
}

// Count returns the number of VMs in the database
func (vmdb *VMDatabase) Count() int {
	vmdb.mutex.Lock()
	defer vmdb.mutex.Unlock()

	return len(vmdb.db)
}

// GetNextRevisionForName returns the next revision for a VM name
func (vmdb *VMDatabase) GetNextRevisionForName(name string) int {
	vmdb.mutex.Lock()
	defer vmdb.mutex.Unlock()

	maxRevision := -1
	for _, entry := range vmdb.db {
		if entry.Name.Name == name && entry.Name.Revision > maxRevision {
			maxRevision = entry.Name.Revision
		}
	}

	return maxRevision + 1 // will return 0 if no previous revision was found
}

// IsVMActive returns true if VM is active
func (vmdb *VMDatabase) IsVMActive(name *VMName) (bool, error) {
	vmdb.mutex.Lock()
	defer vmdb.mutex.Unlock()

	entry, exists := vmdb.db[name.ID()]
	if !exists {
		return false, fmt.Errorf("VM %s not found in database", name)
	}
	return entry.Active, nil
}

// for special server internal purposes (ex: vm state database)
func (vmdb *VMDatabase) getEntryByID(id string) (*VMDatabaseEntry, error) {
	vmdb.mutex.Lock()
	defer vmdb.mutex.Unlock()

	entry, exists := vmdb.db[id]
	if !exists {
		return nil, fmt.Errorf("VM id '%s' not found in database", id)
	}
	return entry, nil
}

// GetEntryByVM lookups a VM entry by it's VM pointer
func (vmdb *VMDatabase) GetEntryByVM(vm *VM) (*VMDatabaseEntry, error) {
	vmdb.mutex.Lock()
	defer vmdb.mutex.Unlock()

	for _, entry := range vmdb.db {
		if entry.VM == vm {
			return entry, nil
		}
	}
	return nil, errors.New("can't find VM by address")
}

// GetCountForName returns the amount of instances with the specified name
// (so 0 means none)
func (vmdb *VMDatabase) GetCountForName(name string) int {
	vmdb.mutex.Lock()
	defer vmdb.mutex.Unlock()

	count := 0
	for _, entry := range vmdb.db {
		if entry.Name.Name == name {
			count++
		}
	}
	return count
}

// SetActiveRevision change the active instance (RevisionNone is allowed)
func (vmdb *VMDatabase) SetActiveRevision(name string, revision int) error {
	// sanity checks (out of lock!)
	if revision == RevisionNone {
		if vmdb.GetCountForName(name) == 0 {
			return fmt.Errorf("no VM '%s' found", name)
		}
	} else {
		vmName := NewVMName(name, revision)
		vm, err := vmdb.GetByName(vmName)
		if err != nil {
			return err
		}
		err = CheckDomainsConflicts(vmdb, vm.Config.Domains, name, vmdb.app.Config)
		if err != nil {
			return err
		}
		err = CheckPortsConflicts(vmdb, vm.Config.Ports, name, nil)
		if err != nil {
			return err
		}
	}

	vmdb.mutex.Lock()
	defer vmdb.mutex.Unlock()

	for _, entry := range vmdb.db {
		if entry.Name.Name == name {
			if entry.Name.Revision == revision {
				entry.Active = true
			} else {
				entry.Active = false
			}
		}
	}

	err := vmdb.save()
	if err != nil {
		return err
	}
	return nil
}
