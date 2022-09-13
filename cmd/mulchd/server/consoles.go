package server

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// FUTURE: implement a real blocking read in ConsoleReader.Read() (caution, it's tricky ;)
// FUTURE: only allow one client at a time to read a console

const (
	ConsoleRingBufferSize = 256 * 1024 // history size
	ConsoleReaderSize     = 1024
	CooldownMaxWait       = 2 * time.Second
)

// ConsoleManager will read and store output of VM's console
type ConsoleReader struct {
	buffer     *OverflowBuffer
	vmNameID   string
	app        *App
	startCycle uint64
	terminated bool
}

// ConsolePersitentReader allows a consumer to read the console output
// even if the underlying ConsoleReader changes
type ConsolePersitentReader struct {
	manager     *ConsoleManager
	vmNameID    string
	ctx         context.Context
	emptyCycles uint64
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

// ScheduleManager will continuously update the readers list
func (cm *ConsoleManager) ScheduleManager() {
	// we wait VM state restore, since vmStateDB is not updated before
	cm.app.VMStateDB.WaitRestore()

	for {
		cm.update()
		time.Sleep(5 * time.Second)
	}
}

// NewPersitentReader returns a ConsolePersitentReader for a VM
func (cm *ConsoleManager) NewPersitentReader(name *VMName, ctx context.Context) (*ConsolePersitentReader, error) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	vmNameID := name.ID()

	if _, ok := cm.readers[vmNameID]; ok {
		return &ConsolePersitentReader{
			manager:  cm,
			vmNameID: vmNameID,
			ctx:      ctx,
		}, nil
	}

	return nil, fmt.Errorf("unable to find a console reader for VM %s", vmNameID)
}

// addReader adds a new console reader, without mutex lock
func (cm *ConsoleManager) addReader(vmNameID string) error {
	if r, ok := cm.readers[vmNameID]; ok {
		if !r.terminated {
			cm.app.Log.Tracef("console reader for VM %s already exists", vmNameID)
			return nil
		} else {
			cm.app.Log.Tracef("replacing terminated console reader for VM %s", vmNameID)
		}
	}

	cm.app.Log.Tracef("creating a new console reader for VM %s", vmNameID)

	cr := &ConsoleReader{
		buffer:     NewOverflowBuffer(ConsoleRingBufferSize),
		vmNameID:   vmNameID,
		app:        cm.app,
		startCycle: cm.app.VMStateDB.Cycles(),
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
				currentCycle := cm.app.VMStateDB.Cycles()
				if currentCycle > cm.readers[vmName].startCycle {
					err := cm.removeReader(vmName)
					if err != nil {
						cm.app.Log.Errorf("can't remove console reader for VM %s: %s", vmName, err)
					}
				} else {
					cm.app.Log.Tracef("VMStateDB was not updated yet, we keep %s reader for now", vmName)
				}
			}
		}
	}
}

// Read implements io.Reader interface
func (pr *ConsolePersitentReader) Read(p []byte) (n int, err error) {
	pr.manager.mutex.Lock()
	defer pr.manager.mutex.Unlock()

	if pr.ctx.Err() != nil {
		return 0, pr.ctx.Err()
	}

	dataAvailable := true

	r, ok := pr.manager.readers[pr.vmNameID]
	if !ok {
		// this VM may come back soon ;) (ex: stop-wait-start cycle)
		dataAvailable = false
	} else {
		if r.buffer.IsEmpty() {
			dataAvailable = false
		}
	}

	if !dataAvailable {
		// For this situation, a cooldown is prefered to a sync.Cond or sync.Mutex
		// because there's tricky edge cases, particularly when the
		// underlying ConsoleReader changes. It may lead to slighly higher CPU usage
		// when a "vm console" is pending, but it's way more reliable.

		wait := time.Duration(pr.emptyCycles*10) * time.Millisecond
		if wait > CooldownMaxWait {
			wait = CooldownMaxWait
		}

		pr.emptyCycles++
		// pr.manager.app.Log.Tracef("no data available for VM %s, waiting %s", pr.vmNameID, wait)
		time.Sleep(wait)

		return 0, nil
	}

	pr.emptyCycles = 0
	return r.buffer.Read(p)
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

	// cr.app.Log.Infof("console %s opened", domainName)

	go func() {
		defer stream.Free() // Abort?
		for {
			buf := make([]byte, ConsoleReaderSize)
			n, err := stream.Recv(buf)
			if err != nil {
				cr.terminated = true
				// cr.app.Log.Warningf("console: %s (n=%d) - exit", err, n)
				return
			} else {
				cr.buffer.Write(buf[:n])
			}
		}
	}()

	return nil
}
