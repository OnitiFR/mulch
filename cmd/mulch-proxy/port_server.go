package main

import "sync/atomic"

// PortServer will manage TCP/UDP forward proxy port listeners
type PortServer struct {
	PortDB         *PortDatabase
	Listeners      map[uint16]*PortListener
	UDPListeners   map[uint16]*UDPPortListener
	Log            *Log
	CurrentVersion int
}

// NewPortServer will create and return a new server
func NewPortServer(filename string, log *Log) (*PortServer, error) {
	db, err := NewPortDatabase(filename)
	if err != nil {
		return nil, err
	}

	server := &PortServer{
		PortDB:       db,
		Log:          log,
		Listeners:    make(map[uint16]*PortListener),
		UDPListeners: make(map[uint16]*UDPPortListener),
	}

	log.Infof("found %d TCP port(s) and %d UDP port(s) in database %s", len(server.PortDB.db.TCP), len(server.PortDB.db.UDP), filename)
	err = server.refreshListeners()
	if err != nil {
		return nil, err
	}

	return server, nil
}

// ReloadPorts will load the file,
func (server *PortServer) ReloadPorts() {
	err := server.PortDB.Load()
	if err != nil {
		server.Log.Errorf("ReloadPorts, load: %s", err)
	}

	err = server.refreshListeners()
	if err != nil {
		server.Log.Errorf("ReloadPorts, refresh: %s", err)
	}
}

// GetTotalConnections return the total connection count for all listeners
func (server *PortServer) GetTotalConnections() int {
	total := 0
	for _, l := range server.Listeners {
		total += int(atomic.LoadInt32(&l.ConnectionCount))
	}
	for _, l := range server.UDPListeners {
		total += int(atomic.LoadInt32(&l.ConnectionCount))
	}
	return total
}

// sync listeners and update forward maps
func (server *PortServer) refreshListeners() error {
	var err error
	server.PortDB.mutex.Lock()
	defer server.PortDB.mutex.Unlock()

	server.CurrentVersion++

	for portNum, portConf := range server.PortDB.db.TCP {
		listener, exists := server.Listeners[portNum]
		if !exists {
			server.Log.Infof("port proxy: opening port %s", portConf.ListenAddr.String())
			listener, err = NewPortListener(portConf.ListenAddr, portConf.Forwards, server.CurrentVersion, server.Log)
			if err != nil {
				server.Log.Errorf("unable to listen on port %d: %s", portNum, err)
				continue
			}
			server.Listeners[portNum] = listener
		} else {
			listener.version = server.CurrentVersion
			err := listener.UpdateForwardMap(portConf.Forwards)
			if err != nil {
				server.Log.Errorf("unable to update port %d: %s", portNum, err)
			}
		}
	}

	// delete unused listeners
	for portNum, listener := range server.Listeners {
		if listener.version != server.CurrentVersion {
			server.Log.Infof("port proxy: closing port %d", portNum)
			err := listener.Close()
			if err != nil {
				server.Log.Errorf("unable to close port %d: %s", portNum, err)
			} else {
				delete(server.Listeners, portNum)
			}
		}
	}

	for portNum, portConf := range server.PortDB.db.UDP {
		listener, exists := server.UDPListeners[portNum]
		if !exists {
			server.Log.Infof("port proxy: opening UDP port %s", portConf.ListenAddr.String())
			listener, err = NewUDPPortListener(portConf.ListenAddr, portConf.Forwards, server.CurrentVersion, server.Log)
			if err != nil {
				server.Log.Errorf("unable to listen on UDP port %d: %s", portNum, err)
				continue
			}
			server.UDPListeners[portNum] = listener
		} else {
			listener.version = server.CurrentVersion
			listener.UpdateForwardMap(portConf.Forwards)
		}
	}

	for portNum, listener := range server.UDPListeners {
		if listener.version != server.CurrentVersion {
			server.Log.Infof("port proxy: closing UDP port %d", portNum)
			err := listener.Close()
			if err != nil {
				server.Log.Errorf("unable to close UDP port %d: %s", portNum, err)
			} else {
				delete(server.UDPListeners, portNum)
			}
		}
	}

	server.Log.Infof("refresh: %d TCP port(s) - %d UDP port(s) - %d connection(s)", len(server.Listeners), len(server.UDPListeners), server.GetTotalConnections())

	return nil
}
