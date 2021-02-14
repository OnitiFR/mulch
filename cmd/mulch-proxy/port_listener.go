package main

import (
	"net"
)

// PortListener is a TCP listener holder
type PortListener struct {
	listenAddr *net.TCPAddr
	listener   *net.TCPListener
	forwardMap map[string]*net.TCPAddr
	log        *Log
	closed     bool
	version    int
}

// NewPortListener will listen on port and forward clients
func NewPortListener(listenAddr *net.TCPAddr, forwardMap map[string]*net.TCPAddr, version int, log *Log) (*PortListener, error) {
	var err error

	listener := &PortListener{
		listenAddr: listenAddr,
		forwardMap: forwardMap,
		log:        log,
		version:    version,
		closed:     false,
	}

	listener.listener, err = net.ListenTCP("tcp", listenAddr)
	if err != nil {
		return nil, err
	}

	go listener.Listen()

	return listener, nil
}

// Listen will wait for client connections
func (pl *PortListener) Listen() {
	for {
		conn, err := pl.listener.AcceptTCP()
		if err != nil {
			if pl.closed == true {
				break
			}
			pl.log.Errorf("port proxy: %s: failed to accept connection: %s", pl.listenAddr.String(), err)
		}

		sourceIP := conn.RemoteAddr().(*net.TCPAddr).IP.String()
		forwardTo, exists := pl.forwardMap[sourceIP]
		if exists {
			// TODO: have a list of all established connection (or at least
			// a wat to clone a specific [or all] connection)
			go NewPortForward(conn, forwardTo, pl.log)
		} else {
			conn.Close()
		}
	}

}

// Close the listener
func (pl *PortListener) Close() error {
	pl.closed = true
	err := pl.listener.Close()
	if err != nil {
		return err
	}
	// TODO: close and wait all established connections
	return nil
}

// UpdateForwardMap will update our forwarding config
func (pl *PortListener) UpdateForwardMap(forwardMap map[string]*net.TCPAddr) error {
	// TODO: look if we need to close some outdated connections
	pl.forwardMap = forwardMap
	return nil
}
