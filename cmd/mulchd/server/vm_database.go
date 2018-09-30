package server

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/Xfennec/mulch/common"
)

// VMDatabase describes a persistent DataBase of VMs structures
type VMDatabase struct {
	filename       string
	domainFilename string
	db             map[string]*VM
	mutex          sync.Mutex
}

// NewVMDatabase instanciates a new VMDatabase
func NewVMDatabase(filename string, domainFilename string) (*VMDatabase, error) {
	vmdb := &VMDatabase{
		filename:       filename,
		domainFilename: domainFilename,
		db:             make(map[string]*VM),
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

	for _, vm := range vmdb.db {
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
func (vmdb *VMDatabase) Delete(name string) error {
	vmdb.mutex.Lock()
	defer vmdb.mutex.Unlock()

	if _, exists := vmdb.db[name]; exists == false {
		return fmt.Errorf("VM '%s' was not found in database", name)
	}

	delete(vmdb.db, name)

	err := vmdb.save()
	if err != nil {
		return err
	}

	return nil
}

// Add a new VM in the database
func (vmdb *VMDatabase) Add(vm *VM) error {
	vmdb.mutex.Lock()
	defer vmdb.mutex.Unlock()

	if _, exists := vmdb.db[vm.Config.Name]; exists == true {
		return fmt.Errorf("VM '%s' already exists in database", vm.Config.Name)
	}

	vmdb.db[vm.Config.Name] = vm
	err := vmdb.save()
	if err != nil {
		return err
	}
	return nil
}

// GetNames of all VMs in the database
func (vmdb *VMDatabase) GetNames() []string {
	vmdb.mutex.Lock()
	defer vmdb.mutex.Unlock()

	keys := make([]string, 0, len(vmdb.db))
	for key := range vmdb.db {
		keys = append(keys, key)
	}
	return keys
}

// GetByName lookups a VM by its name
func (vmdb *VMDatabase) GetByName(name string) (*VM, error) {
	vmdb.mutex.Lock()
	defer vmdb.mutex.Unlock()

	vm, exists := vmdb.db[name]
	if !exists {
		return nil, fmt.Errorf("VM '%s' not found in database", name)
	}
	return vm, nil
}

// GetBySecretUUID lookups a VM by its secretUUID
func (vmdb *VMDatabase) GetBySecretUUID(uuid string) (*VM, error) {
	vmdb.mutex.Lock()
	defer vmdb.mutex.Unlock()

	anon := "?"
	if len(uuid) > 4 {
		anon = uuid[:4] + "…"
	}

	for _, vm := range vmdb.db {
		if vm.SecretUUID == uuid {
			return vm, nil
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
