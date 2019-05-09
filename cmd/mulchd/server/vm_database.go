package server

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/OnitiFR/mulch/common"
)

type updateCallback func()

// VMDatabaseEntry is an entry in the DB: a name and a VM
// Only one entry can be active per name
type VMDatabaseEntry struct {
	Name   *VMName
	VM     *VM
	Active bool
}

// VMDatabase describes a persistent DataBase of VMs structures
type VMDatabase struct {
	filename       string
	domainFilename string
	db             map[string]*VMDatabaseEntry
	mutex          sync.Mutex
	onUpdate       updateCallback
}

// NewVMDatabase instanciates a new VMDatabase
func NewVMDatabase(filename string, domainFilename string, onUpdate updateCallback) (*VMDatabase, error) {
	vmdb := &VMDatabase{
		filename:       filename,
		domainFilename: domainFilename,
		db:             make(map[string]*VMDatabaseEntry),
		onUpdate:       onUpdate,
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

// build domain database, updated with each vm.LastIP
func (vmdb *VMDatabase) genDomainsDB() error {
	domains := make(map[string]*common.Domain)

	for _, entry := range vmdb.db {
		if entry.Active == false {
			continue
		}
		vm := entry.VM
		for _, domain := range vm.Config.Domains {
			if domain.RedirectTo == "" {
				domain.DestinationHost = vm.LastIP
			}

			otherDomain, exist := domains[domain.Name]
			if exist == true {
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
	vmdb.mutex.Lock()
	defer vmdb.mutex.Unlock()

	if _, exists := vmdb.db[name.ID()]; exists == false {
		return fmt.Errorf("VM '%s' was not found in database", name.ID())
	}

	delete(vmdb.db, name.ID())

	err := vmdb.save()
	if err != nil {
		return err
	}

	return nil
}

// Add a new VM in the database
func (vmdb *VMDatabase) Add(vm *VM, name *VMName, active bool) error {
	vmdb.mutex.Lock()
	defer vmdb.mutex.Unlock()

	if _, exists := vmdb.db[name.ID()]; exists == true {
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

// GetByName lookups a VM by its name
func (vmdb *VMDatabase) GetByName(name *VMName) (*VM, error) {
	vmdb.mutex.Lock()
	defer vmdb.mutex.Unlock()

	vm, exists := vmdb.db[name.ID()]
	if !exists {
		return nil, fmt.Errorf("VM %s not found in database", name)
	}
	return vm.VM, nil
}

// GetBySecretUUID lookups a VM by its secretUUID
func (vmdb *VMDatabase) GetBySecretUUID(uuid string) (*VM, error) {
	vmdb.mutex.Lock()
	defer vmdb.mutex.Unlock()

	anon := "?"
	if len(uuid) > 4 {
		anon = uuid[:4] + "…"
	}

	for _, entry := range vmdb.db {
		if entry.VM.SecretUUID == uuid {
			return entry.VM, nil
		}
	}
	return nil, fmt.Errorf("UUID '%s' was not found in database", anon)
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
