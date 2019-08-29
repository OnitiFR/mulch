package server

import (
	"fmt"
	"math/rand"
	"time"
)

// Basic operation list, only for 'status' command currently. That's why
// it's mostly managed by controllers and not server package.

// A possible enhancement is to get "real" ressources pointers (*VM, *Seed, …)
// to allow various things like protections (ex: "you can't stop a VM
// during its rebuild")

// Operation on the server
type Operation struct {
	Origin        string // API Key, "[seeder]", "[autorebuild]", …
	Action        string // delete, remove, rebuild, …
	Ressource     string // backup, seed, vm, …
	RessourceName string // VM name, seed name, …
	StartTime     time.Time
}

// OperationList is a list of currently running operations
type OperationList struct {
	operations map[string]*Operation
	rand       *rand.Rand
}

// NewOperationList instanciates a new OperationList
func NewOperationList(rand *rand.Rand) *OperationList {
	return &OperationList{
		operations: make(map[string]*Operation),
		rand:       rand,
	}
}

// Add an operation to the list
func (db *OperationList) Add(op *Operation) string {
	id := fmt.Sprintf("operation-%d", rand.Int31())
	op.StartTime = time.Now()
	db.operations[id] = op
	return id
}

// Remove an operation from the list
func (db *OperationList) Remove(id string) {
	delete(db.operations, id)
}
