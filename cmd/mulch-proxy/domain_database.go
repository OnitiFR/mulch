package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
)

// DomainDatabase describes a persistent DataBase of Domain structures
type DomainDatabase struct {
	filename string
	db       map[string]*Domain
	mutex    sync.Mutex
}

// NewDomainDatabase instanciates a new DomainDatabase
func NewDomainDatabase(filename string) (*DomainDatabase, error) {
	ddb := &DomainDatabase{
		filename: filename,
		db:       make(map[string]*Domain),
	}

	// if the file exists, load it
	if _, err := os.Stat(ddb.filename); err == nil {
		err = ddb.load()
		if err != nil {
			return nil, err
		}
	}

	// save the file to check if it's writable
	err := ddb.save()
	if err != nil {
		return nil, err
	}

	return ddb, nil
}

// This is done internaly, because it must be done with the mutex locked,
// but we can't lock it here, since save() is called by functions that
// are already locking the mutex.
func (ddb *DomainDatabase) save() error {
	f, err := os.OpenFile(ddb.filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	err = enc.Encode(&ddb.db)
	if err != nil {
		return err
	}
	return nil
}

func (ddb *DomainDatabase) load() error {
	f, err := os.Open(ddb.filename)
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
		return fmt.Errorf("%s: only the owner should be able to read/write this file (mode 0600)", ddb.filename)
	}

	dec := json.NewDecoder(f)
	err = dec.Decode(&ddb.db)
	if err != nil {
		return err
	}
	return nil
}

// Delete the Domain from the database using its name
func (ddb *DomainDatabase) Delete(name string) error {
	ddb.mutex.Lock()
	defer ddb.mutex.Unlock()

	if _, exists := ddb.db[name]; exists == false {
		return fmt.Errorf("Domain '%s' was not found in database", name)
	}

	delete(ddb.db, name)

	err := ddb.save()
	if err != nil {
		return err
	}

	return nil
}

// Add a new Domain in the database
func (ddb *DomainDatabase) Add(domain *Domain) error {
	ddb.mutex.Lock()
	defer ddb.mutex.Unlock()

	if _, exists := ddb.db[domain.Name]; exists == true {
		return fmt.Errorf("Domain '%s' already exists in database", domain.Name)
	}

	ddb.db[domain.Name] = domain
	err := ddb.save()
	if err != nil {
		return err
	}
	return nil
}

// GetDomains return all domain names in the database
func (ddb *DomainDatabase) GetDomains() []string {
	ddb.mutex.Lock()
	defer ddb.mutex.Unlock()

	keys := make([]string, 0, len(ddb.db))
	for key := range ddb.db {
		keys = append(keys, key)
	}
	return keys
}

// GetByName lookups a Domain by its domain
func (ddb *DomainDatabase) GetByName(name string) (*Domain, error) {
	ddb.mutex.Lock()
	defer ddb.mutex.Unlock()

	domain, exists := ddb.db[name]
	if !exists {
		return nil, fmt.Errorf("Domain '%s' not found in database", name)
	}
	return domain, nil
}

// Count returns the number of Domains in the database
func (ddb *DomainDatabase) Count() int {
	ddb.mutex.Lock()
	defer ddb.mutex.Unlock()

	return len(ddb.db)
}
