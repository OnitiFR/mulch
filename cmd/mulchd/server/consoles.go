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
	app       *App
	readers   map[string]*ConsoleReader
	mutex     sync.Mutex
	lastCycle uint64
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

// update internal database, looking for new VMs and expired readers
func (cm *ConsoleManager) update() {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	cycle := cm.app.VMStateDB.Cycles()
	if cycle > cm.lastCycle {
		cm.lastCycle = cycle

		states := cm.app.VMStateDB.Get()
		for vmName, state := range states {
			if state == VMStateUp {
				if _, ok := cm.readers[vmName]; !ok {
					err := cm.addReader(vmName)
					if err != nil {
						cm.app.Log.Errorf("can't add console reader for VM %s: %s", vmName, err)
					}
				}
			}
		}
	}

	for vmName, cr := range cm.readers {
		if cr.terminated {
			cm.removeReader(vmName)
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

	/* seen strange cases : Dec 07 2022 (23:33:44), Mar 06 2023 (11:57:50)

	mulchd: INFO(vm): creating VM disk 'vm-r57.qcow2'
	mulchd: INFO(): HUP signal sent to mulch-proxy
	mulchd: panic: runtime error: invalid memory address or nil pointer dereference
	mulchd: [signal SIGSEGV: segmentation violation code=0x1 addr=0x0 pc=0x7518c7]
	mulchd: goroutine 87 [running]:
	mulchd: libvirt.org/go/libvirt.(*Domain).OpenConsole.func2(0xc00015dcb8?, 0x42c527?, 0x8?, 0x0, 0xc000e5e801?)
	mulchd:         go/pkg/mod/libvirt.org/go/libvirt@v1.8006.0/domain.go:4628 +0x27
	mulchd: libvirt.org/go/libvirt.(*Domain).OpenConsole(0xc0030c00a8?, {0x0?, 0x0}, 0xc000101000?, 0xc000658090?)
	mulchd:         go/pkg/mod/libvirt.org/go/libvirt@v1.8006.0/domain.go:4628 +0xc6
	mulchd: github.com/OnitiFR/mulch/cmd/mulchd/server.(*ConsoleReader).Start(0xc000658090)
	mulchd:         go/src/github.com/OnitiFR/mulch/cmd/mulchd/server/consoles.go:239 +0xc6
	mulchd: github.com/OnitiFR/mulch/cmd/mulchd/server.(*ConsoleManager).addReader(0xc0000ac000, {0xc001a623e4, 0xa})
	mulchd:         go/src/github.com/OnitiFR/mulch/cmd/mulchd/server/consoles.go:104 +0x27d
	mulchd: github.com/OnitiFR/mulch/cmd/mulchd/server.(*ConsoleManager).update(0xc0000ac000)
	mulchd:         go/src/github.com/OnitiFR/mulch/cmd/mulchd/server/consoles.go:155 +0x187

	mulchd: [signal SIGSEGV: segmentation violation code=0x1 addr=0x0 pc=0x755c27]
	mulchd: goroutine 77 [running]:
	mulchd: libvirt.org/go/libvirt.(*Domain).OpenConsole.func2(0xc0004b5cb8?, 0x42d6e7?, 0x8?, 0x0, 0xc00171ab01?)
	mulchd:         /home/mulch/go/pkg/mod/libvirt.org/go/libvirt@v1.8010.0/domain.go:4562 +0x27
	mulchd: libvirt.org/go/libvirt.(*Domain).OpenConsole(0xc0050a0ee8?, {0x0?, 0x0}, 0xc00058e400?, 0xc0038fc2a0?)
	mulchd:         /home/mulch/go/pkg/mod/libvirt.org/go/libvirt@v1.8010.0/domain.go:4562 +0xc6
	mulchd: github.com/OnitiFR/mulch/cmd/mulchd/server.(*ConsoleReader).Start(0xc0038fc2a0)
	mulchd:         /home/mulch/go/pkg/mod/github.com/!oniti!f!r/mulch@v0.0.0-20230302103019-2e88b7ce0bb9/cmd/mulchd/server/consoles.go:260 +0xcf
	mulchd: github.com/OnitiFR/mulch/cmd/mulchd/server.(*ConsoleManager).addReader(0xc000525d40, {0xc0050a0ee8, 0x10})
	mulchd:         /home/mulch/go/pkg/mod/github.com/!oniti!f!r/mulch@v0.0.0-20230302103019-2e88b7ce0bb9/cmd/mulchd/server/consoles.go:104 +0x27d
	mulchd: github.com/OnitiFR/mulch/cmd/mulchd/server.(*ConsoleManager).update(0xc000525d40)
	mulchd:         /home/mulch/go/pkg/mod/github.com/!oniti!f!r/mulch@v0.0.0-20230302103019-2e88b7ce0bb9/cmd/mulchd/server/consoles.go:155 +0x187
	mulchd: github.com/OnitiFR/mulch/cmd/mulchd/server.(*ConsoleManager).ScheduleManager(0xc000525d40)
	mulchd:         /home/mulch/go/pkg/mod/github.com/!oniti!f!r/mulch@v0.0.0-20230302103019-2e88b7ce0bb9/cmd/mulchd/server/consoles.go:62 +0x47
	mulchd: created by github.com/OnitiFR/mulch/cmd/mulchd/server.NewConsoleManager
	mulchd:         /home/mulch/go/pkg/mod/github.com/!oniti!f!r/mulch@v0.0.0-20230302103019-2e88b7ce0bb9/cmd/mulchd/server/consoles.go:51 +0xb6

	another one *at the end* of a seed rebuild (?!) (more or less during database save)
	issue specific to seeds?
	*/

	// turns out that this is not the correct fix, we had another crash
	if stream == nil {
		return fmt.Errorf("strange empty stream with no error (%s)", cr.vmNameID)
	}

	err = domain.OpenConsole("", stream, 0)
	if err != nil {
		stream.Abort()
		return fmt.Errorf("can't open console: %s", err)
	}

	// cr.app.Log.Infof("console %s opened", domainName)

	go func() {
		defer stream.Free()
		buf := make([]byte, ConsoleReaderSize)
		for {
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
