package main

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/OnitiFR/mulch/common"
)

// PortDatabase describes a persistent database for port forwarding
type PortDatabase struct {
	filename string
	db       common.TCPPortListeners

	mutex sync.Mutex
}

// NewPortDatabase will create a new database instance
func NewPortDatabase(filename string) (*PortDatabase, error) {
	pdb := &PortDatabase{
		filename: filename,
	}

	err := pdb.Load()
	if err != nil {
		return nil, err
	}

	return pdb, nil
}

// Load the port database
// if the file does not exists, it's not an error (database will then be empty)
func (pdb *PortDatabase) Load() error {
	pdb.mutex.Lock()
	defer pdb.mutex.Unlock()

	// clear any previous map
	pdb.db = make(common.TCPPortListeners)

	if !common.PathExist(pdb.filename) {
		return nil
	}

	f, err := os.Open(pdb.filename)
	if err != nil {
		return err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	err = dec.Decode(&pdb.db)
	if err != nil {
		return err
	}
	return nil
}
