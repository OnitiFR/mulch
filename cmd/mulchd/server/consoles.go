package server

import (
	"sync"
	"time"
)

// ConsoleManager will read a VM console
type ConsoleReader struct {
	// rolling buffer?
}

// ConsoleManager manages console readers
type ConsoleManager struct {
	vmStateDB *VMStateDatabase
	readers   map[string]*ConsoleReader
	log       *Log
	mutex     sync.Mutex
}

// NewConsoleManager creates a new ConsoleManager
func NewConsoleManager(app *App) *ConsoleManager {
	cm := &ConsoleManager{
		vmStateDB: app.VMStateDB,
		readers:   make(map[string]*ConsoleReader),
		log:       app.Log,
	}

	go cm.ScheduleManager()

	return cm
}

func (cm *ConsoleManager) ScheduleManager() {
	// we wait VM state restore, since vmStateDB is not updated before
	cm.vmStateDB.WaitRestore()

	for {
		cm.update()
		time.Sleep(5 * time.Second)
	}
}

// addReader adds a new console reader, without mutex lock
func (cm *ConsoleManager) addReader(vmNameID string) error {
	if _, ok := cm.readers[vmNameID]; ok {
		cm.log.Tracef("console reader for VM %s already exists", vmNameID)
		return nil
	}

	cm.log.Tracef("creating a new console reader for VM %s", vmNameID)
	cm.readers[vmNameID] = &ConsoleReader{}
	return nil
}

// AddReader adds a new console reader
func (cm *ConsoleManager) AddReader(vmNameID string) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	return cm.addReader(vmNameID)
}

// removeReader removes a console reader, without mutex lock
func (cm *ConsoleManager) removeReader(vmNameID string) error {
	if _, ok := cm.readers[vmNameID]; !ok {
		cm.log.Tracef("console reader for VM %s no more exists", vmNameID)
		return nil
	}

	cm.log.Tracef("removing console reader for VM %s", vmNameID)
	delete(cm.readers, vmNameID)
	return nil
}

// RemoveReader removes a console reader
func (cm *ConsoleManager) RemoveReader(vmNameID string) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	return cm.removeReader(vmNameID)
}

// update internal database, looking for new VMs and expired ones
func (cm *ConsoleManager) update() {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	states := cm.vmStateDB.Get()
	for vmName, state := range states {
		if state == VMStateUp {
			if _, ok := cm.readers[vmName]; !ok {
				cm.addReader(vmName)
			}
		} else {
			if _, ok := cm.readers[vmName]; ok {
				cm.removeReader(vmName)
			}
		}
	}
}

// restart readers when they have exited?

/*

// console read, for vm.go, after domain.Create() in NewVM() / VMStartByName()
// the stream seems to survive vm reboot (not start/stop)
// store this in a rolling buffer, add mulch vm console cmd? (if so: lock our DB!)

	connect, err := app.Libvirt.GetConnection()
	if err != nil {
		return err
	}

	stream, err := connect.NewStream(0)
	if err != nil {
		return err
	}
	defer stream.Free()
	// defer stream.Finish()
	defer stream.Abort()

	err = domain.OpenConsole("", stream, 0)
	if err != nil {
		log.Warningf("can't open console: %s", err)
	} else {
		log.Infof("console opened")

		go func() {
			for {
				buf := make([]byte, 128)
				n, err := stream.Recv(buf)
				if err != nil {
					log.Warningf("console: %s (n=%d) - exit", err, n)
					return
				} else {
					log.Infof("console: %s", string(buf[:n]))
				}
			}
		}()
		time.Sleep(10 * time.Minute)
	}

*/
