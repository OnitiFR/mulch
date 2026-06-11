package server

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
)

// ReplicaDatabase describes a persistent ReplicaState database (receiver side)
type ReplicaDatabase struct {
	filename string
	db       map[string]*ReplicaState
	mutex    sync.Mutex
}

// NewReplicaDatabase instanciates a new ReplicaDatabase
func NewReplicaDatabase(filename string) (*ReplicaDatabase, error) {
	db := &ReplicaDatabase{
		filename: filename,
		db:       make(map[string]*ReplicaState),
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

func (db *ReplicaDatabase) save() error {
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

func (db *ReplicaDatabase) load() error {
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

// Get returns the ReplicaState for a VM ID, or nil if not found
func (db *ReplicaDatabase) Get(vmID string) *ReplicaState {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	state, exists := db.db[vmID]
	if !exists {
		return nil
	}
	return state
}

// Set stores or updates the ReplicaState for a VM
func (db *ReplicaDatabase) Set(state *ReplicaState) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	db.db[state.ID()] = state
	return db.save()
}

// Delete removes the ReplicaState for a VM ID
func (db *ReplicaDatabase) Delete(vmID string) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	delete(db.db, vmID)
	return db.save()
}

// GetAllForName returns all ReplicaStates (every revision) sharing the given name
func (db *ReplicaDatabase) GetAllForName(name string) []*ReplicaState {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	states := make([]*ReplicaState, 0)
	for _, state := range db.db {
		if state.Name == name {
			states = append(states, state)
		}
	}
	return states
}

// GetAll returns all ReplicaStates
func (db *ReplicaDatabase) GetAll() []*ReplicaState {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	states := make([]*ReplicaState, 0, len(db.db))
	for _, state := range db.db {
		states = append(states, state)
	}
	return states
}

// Count returns the number of entries in the database
func (db *ReplicaDatabase) Count() int {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	return len(db.db)
}
