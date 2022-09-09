package server

import (
	"fmt"
	"sync"
	"time"
)

const (
	ConsoleRingBufferSize = 1024 * 1024
	ConsoleReaderSize     = 1024
)

// ConsoleManager will read a VM console
type ConsoleReader struct {
	buffer   *OverflowBuffer
	vmNameID string
	app      *App
}

// ConsoleManager manages console readers
type ConsoleManager struct {
	app     *App
	readers map[string]*ConsoleReader
	mutex   sync.Mutex
}

// NewConsoleManager creates a new ConsoleManager
func NewConsoleManager(app *App) *ConsoleManager {
	cm := &ConsoleManager{
		app:     app,
		readers: make(map[string]*ConsoleReader),
	}

	go cm.ScheduleManager()

	return cm
}

func (cm *ConsoleManager) ScheduleManager() {
	// we wait VM state restore, since vmStateDB is not updated before
	cm.app.VMStateDB.WaitRestore()

	for {
		cm.update()
		time.Sleep(5 * time.Second)
	}
}

// addReader adds a new console reader, without mutex lock
func (cm *ConsoleManager) addReader(vmNameID string) error {
	if _, ok := cm.readers[vmNameID]; ok {
		cm.app.Log.Tracef("console reader for VM %s already exists", vmNameID)
		return nil
	}

	cm.app.Log.Tracef("creating a new console reader for VM %s", vmNameID)

	cr := &ConsoleReader{
		buffer:   NewOverflowBuffer(ConsoleRingBufferSize),
		vmNameID: vmNameID,
		app:      cm.app,
	}

	err := cr.Start()
	if err != nil {
		return err
	}

	cm.readers[vmNameID] = cr

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
		cm.app.Log.Tracef("console reader for VM %s no more exists", vmNameID)
		return nil
	}

	cm.app.Log.Tracef("removing console reader for VM %s", vmNameID)
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

	states := cm.app.VMStateDB.Get()
	for vmName, state := range states {
		if state == VMStateUp {
			if _, ok := cm.readers[vmName]; !ok {
				err := cm.addReader(vmName)
				if err != nil {
					cm.app.Log.Errorf("can't add console reader for VM %s: %s", vmName, err)
				}
			}
		} else {
			if _, ok := cm.readers[vmName]; ok {
				err := cm.removeReader(vmName)
				if err != nil {
					cm.app.Log.Errorf("can't remove console reader for VM %s: %s", vmName, err)
				}
			}
		}
	}
}

// Start reading data from a console and store it in a ring buffer
func (cr *ConsoleReader) Start() error {
	connect, err := cr.app.Libvirt.GetConnection()
	if err != nil {
		return err
	}

	name, err := ParseVMName(cr.vmNameID)
	if err != nil {
		return err
	}

	domainName := name.LibvirtDomainName(cr.app)
	domain, err := cr.app.Libvirt.GetDomainByName(domainName)

	if err != nil {
		return err
	}

	stream, err := connect.NewStream(0)
	if err != nil {
		return err
	}

	err = domain.OpenConsole("", stream, 0)
	if err != nil {
		stream.Abort() // Free?
		return fmt.Errorf("can't open console: %s", err)
	}

	cr.app.Log.Infof("console %s opened", domainName)

	go func() {
		defer stream.Free() // Abort?
		for {
			buf := make([]byte, 128)
			n, err := stream.Recv(buf)
			if err != nil {
				cr.app.Log.Warningf("console: %s (n=%d) - exit", err, n)
				return
			} else {
				cr.app.Log.Infof("console: %s", string(buf[:n]))
			}
		}
	}()

	return nil
}

// conflict between early AddReader() and update()
// restart readers when they have exited?
// - should not happend and/or see conflict with vm stop / start commands
// add mulch vm console cmd? (if so: lock our DB!)
