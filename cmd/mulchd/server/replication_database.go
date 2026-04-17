package server

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
)

// ReplicationDatabase describes a persistent ReplicationState database
type ReplicationDatabase struct {
	filename string
	db       map[string]*ReplicationState
	mutex    sync.Mutex
}

// NewReplicationDatabase instanciates a new ReplicationDatabase
func NewReplicationDatabase(filename string) (*ReplicationDatabase, error) {
	db := &ReplicationDatabase{
		filename: filename,
		db:       make(map[string]*ReplicationState),
	}

	// if the file exists, load it
	if _, err := os.Stat(db.filename); err == nil {
		err = db.load()
		if err != nil {
			return nil, err
		}
	}

	// save the file to check if it's writable
	err := db.save()
	if err != nil {
		return nil, err
	}

	return db, nil
}

func (db *ReplicationDatabase) save() error {
	f, err := os.OpenFile(db.filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	err = enc.Encode(&db.db)
	if err != nil {
		return err
	}
	return nil
}

func (db *ReplicationDatabase) load() error {
	f, err := os.Open(db.filename)
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
		return fmt.Errorf("%s: only the owner should be able to read/write this file (mode 0600)", db.filename)
	}

	dec := json.NewDecoder(f)
	err = dec.Decode(&db.db)
	if err != nil {
		return err
	}
	return nil
}

// Get returns the ReplicationState for a VM, or nil if not found
func (db *ReplicationDatabase) Get(vmName string) *ReplicationState {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	state, exists := db.db[vmName]
	if !exists {
		return nil
	}
	return state
}

// Set stores or updates the ReplicationState for a VM
func (db *ReplicationDatabase) Set(state *ReplicationState) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	db.db[state.ID()] = state
	return db.save()
}

// Delete removes the ReplicationState for a VM
func (db *ReplicationDatabase) Delete(vmName string) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	delete(db.db, vmName)
	return db.save()
}

// GetAll returns all ReplicationStates
func (db *ReplicationDatabase) GetAll() []*ReplicationState {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	states := make([]*ReplicationState, 0, len(db.db))
	for _, state := range db.db {
		states = append(states, state)
	}
	return states
}

// Count returns the number of entries in the database
func (db *ReplicationDatabase) Count() int {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	return len(db.db)
}
