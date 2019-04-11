package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/OnitiFR/mulch/common"
)

// TODO: watch the file, re-create proxies on refresh

// DomainDatabase describes a persistent DataBase of Domain structures
type DomainDatabase struct {
	filename string
	db       map[string]*common.Domain
	mutex    sync.Mutex
}

// NewDomainDatabase instanciates a new DomainDatabase
func NewDomainDatabase(filename string) (*DomainDatabase, error) {
	ddb := &DomainDatabase{
		filename: filename,
	}

	err := ddb.load()
	if err != nil {
		return nil, err
	}

	return ddb, nil
}

func (ddb *DomainDatabase) load() error {
	f, err := os.Open(ddb.filename)
	if err != nil {
		return err
	}
	defer f.Close()

	// clear any previous map
	ddb.db = make(map[string]*common.Domain)

	dec := json.NewDecoder(f)
	err = dec.Decode(&ddb.db)
	if err != nil {
		return err
	}
	return nil
}

// Reload is the mutex-protected variant of load()
func (ddb *DomainDatabase) Reload() error {
	ddb.mutex.Lock()
	defer ddb.mutex.Unlock()

	return ddb.load()
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
func (ddb *DomainDatabase) GetByName(name string) (*common.Domain, error) {
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
