package server

import (
	"encoding/json"
	"os"
)

// VMDatabaseMigrate allows old format VM database migration to new (v2) format
type VMDatabaseMigrate struct {
	dbv1 map[string]*VM
	dbv2 map[string]*VMDatabaseEntry
}

// NewVMDatabaseMigrate create an new VMDatabaseMigrate instance
func NewVMDatabaseMigrate() *VMDatabaseMigrate {
	instance := &VMDatabaseMigrate{
		dbv1: make(map[string]*VM),
		dbv2: make(map[string]*VMDatabaseEntry),
	}
	return instance
}

func (vmdb *VMDatabaseMigrate) loadv1(filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	err = dec.Decode(&vmdb.dbv1)
	if err != nil {
		return err
	}
	return nil
}

// do the actual migration (revision 0, active)
func (vmdb *VMDatabaseMigrate) migrate() {
	for name, vm := range vmdb.dbv1 {
		entry := &VMDatabaseEntry{
			Name:   NewVMName(name, 0),
			Active: true,
			VM:     vm,
		}
		vmdb.dbv2[entry.Name.ID()] = entry
	}
}

func (vmdb *VMDatabaseMigrate) savev2(filename string) error {
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	err = enc.Encode(&vmdb.dbv2)
	if err != nil {
		return err
	}

	return nil
}
