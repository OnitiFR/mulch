package server

import (
	"encoding/json"
	"os"
	"reflect"
	"sync"
	"time"
)

// VM states
const (
	VMStateUp   = "up"
	VMStateDown = "down"
)

// VMStateDatabase describes a persistent DataBase of VM state (up or down)
type VMStateDatabase struct {
	filename string
	db       map[string]string
	mutex    sync.Mutex
	app      *App
	restored bool
}

// NewVMStateDatabase instanciates a new VMStateDatabase
func NewVMStateDatabase(filename string, app *App) (*VMStateDatabase, error) {
	vmsdb := &VMStateDatabase{
		filename: filename,
		db:       make(map[string]string),
		app:      app,
		restored: false,
	}

	// if the file exists, load it
	if _, err := os.Stat(vmsdb.filename); err == nil {
		err = vmsdb.load()
		if err != nil {
			return nil, err
		}
	}

	// save the file to check if it's writable
	err := vmsdb.save()
	if err != nil {
		return nil, err
	}

	return vmsdb, nil
}

// This is done internaly, because it must be done with the mutex locked,
// but we can't lock it here, since save() is called by functions that
// are already locking the mutex.
func (vmsdb *VMStateDatabase) save() error {
	f, err := os.Create(vmsdb.filename)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	err = enc.Encode(&vmsdb.db)
	if err != nil {
		return err
	}

	return nil
}

func (vmsdb *VMStateDatabase) load() error {
	f, err := os.Open(vmsdb.filename)
	if err != nil {
		return err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	err = dec.Decode(&vmsdb.db)
	if err != nil {
		return err
	}
	return nil
}

// Update saves the DB with current VM states
func (vmsdb *VMStateDatabase) Update() error {
	vmsdb.mutex.Lock()
	defer vmsdb.mutex.Unlock()

	// loop over VM and get state
	newStates := make(map[string]string)
	for _, vmName := range vmsdb.app.VMDB.GetNames() {
		state, err := VMIsRunning(vmName, vmsdb.app)
		if err != nil {
			return err
		}
		sState := VMStateDown
		if state {
			sState = VMStateUp
		}
		newStates[vmName.ID()] = sState
	}

	// compare states with previous ones
	eq := reflect.DeepEqual(vmsdb.db, newStates)
	if eq {
		return nil
	}

	// something changed, let's update
	vmsdb.app.Log.Info("updating VM state database")
	vmsdb.db = newStates
	return vmsdb.save()
}

// Get returns the state of all VMs
func (vmsdb *VMStateDatabase) Get() map[string]string {
	vmsdb.mutex.Lock()
	defer vmsdb.mutex.Unlock()

	res := make(map[string]string, len(vmsdb.db))
	for k, v := range vmsdb.db {
		res[k] = v
	}

	return res
}

// Run the VM state monitoring loop
func (vmsdb *VMStateDatabase) Run() error {
	// small cooldown (app init)
	time.Sleep(1 * time.Second)

	vmsdb.restoreStates()
	vmsdb.Update()

	vmsdb.restored = true

	for {
		vmsdb.Update()
		time.Sleep(10 * time.Second)
	}
}

func (vmsdb *VMStateDatabase) restoreStates() {
	coldStates := vmsdb.db
	var wg sync.WaitGroup

	operation := vmsdb.app.Operations.Add(&Operation{
		Origin:        "[app]",
		Action:        "restore_states",
		Ressource:     "vm",
		RessourceName: "*",
	})
	defer vmsdb.app.Operations.Remove(operation)

	for id, coldState := range coldStates {
		entry, err := vmsdb.app.VMDB.getEntryByID(id)
		if err != nil {
			vmsdb.app.Log.Error(err.Error())
			continue
		}

		hotState, err := VMIsRunning(entry.Name, vmsdb.app)
		if err != nil {
			vmsdb.app.Log.Error(err.Error())
			continue
		}

		if coldState == VMStateUp && !hotState {
			vmsdb.app.Log.Infof("restore state: starting %s", entry.Name)
			wg.Add(1)
			go func() {
				err := VMStartByName(entry.Name, entry.VM.SecretUUID, vmsdb.app, vmsdb.app.Log)
				if err != nil {
					vmsdb.app.Log.Error(err.Error())
				}
				wg.Done()
			}()
		}
		if coldState == VMStateDown && hotState {
			wg.Add(1)
			vmsdb.app.Log.Infof("restore state: stopping %s", entry.Name)
			go func() {
				err := VMStopByName(entry.Name, vmsdb.app, vmsdb.app.Log)
				if err != nil {
					vmsdb.app.Log.Error(err.Error())
				}
				wg.Done()
			}()
		}
	}

	wg.Wait()
	vmsdb.app.Log.Infof("VM states restored")
}

// WaitRestore blocks until VM states are not restored, so
// non-crucial tasks can kindly wait for a "quiter" system load.
// NOTE: very crude timeout-baed implementation, should use sync.Cond
func (vmsdb *VMStateDatabase) WaitRestore() {
	for {
		if vmsdb.restored {
			return
		}
		time.Sleep(2 * time.Second)
	}
}
