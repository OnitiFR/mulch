package main

import (
	"fmt"
	"net"
	"sync"
)

// PortListener is a TCP listener holder
type PortListener struct {
	ConnectionCount int32
	listenAddr      *net.TCPAddr
	listener        *net.TCPListener
	port            uint16
	public          bool
	forwardMap      map[string]*net.TCPAddr
	closeChannels   map[string]chan bool
	log             *Log
	closed          bool
	version         int
	mutex           sync.Mutex
}

// NewPortListener will listen on port and forward clients
func NewPortListener(listenAddr *net.TCPAddr, forwardMap map[string]*net.TCPAddr, version int, log *Log) (*PortListener, error) {
	var err error

	listener := &PortListener{
		listenAddr:    listenAddr,
		port:          uint16(listenAddr.Port),
		closeChannels: make(map[string]chan bool),
		log:           log,
		version:       version,
		closed:        false,
	}

	_, exists := forwardMap["*"]
	if exists {
		listener.public = true
	}

	listener.listener, err = net.ListenTCP("tcp", listenAddr)
	if err != nil {
		return nil, err
	}

	err = listener.UpdateForwardMap(forwardMap)
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
			if pl.closed {
				break
			}
			pl.log.Errorf("port proxy: %s: failed to accept connection: %s", pl.listenAddr.String(), err)
		}

		sourceIP := conn.RemoteAddr().(*net.TCPAddr).IP.String()

		var forwardTo *net.TCPAddr
		var existsF bool

		if pl.public {
			forwardTo, existsF = pl.forwardMap["*"]
		} else {
			forwardTo, existsF = pl.forwardMap[sourceIP]
		}

		key := "???"
		if existsF {
			if pl.public {
				key = fmt.Sprintf("*->%s", forwardTo.IP.String())
			} else {
				key = fmt.Sprintf("%s->%s", sourceIP, forwardTo.IP.String())
			}
		}

		closeChan, existsC := pl.closeChannels[key]

		if existsF && existsC {
			pl.log.Tracef("+ TCP %s->%s:%d", sourceIP, forwardTo.IP.String(), pl.port)
			go NewPortForward(conn, forwardTo, closeChan, pl.log, &pl.ConnectionCount)
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

	pl.mutex.Lock()
	defer pl.mutex.Unlock()

	for key, c := range pl.closeChannels {
		close(c)
		delete(pl.closeChannels, key)
	}

	return nil
}

// UpdateForwardMap will update our forwarding config
func (pl *PortListener) UpdateForwardMap(forwardMap map[string]*net.TCPAddr) error {
	pl.mutex.Lock()
	defer pl.mutex.Unlock()

	allowedForwards := make(map[string]bool)
	for srcIP, forward := range forwardMap {
		key := fmt.Sprintf("%s->%s", srcIP, forward.IP.String())
		allowedForwards[key] = true
	}

	// add missing close channels
	for key := range allowedForwards {
		_, exists := pl.closeChannels[key]
		if !exists {
			pl.closeChannels[key] = make(chan bool)
			// pl.log.Tracef("add close chan %s (port %d)", key, pl.port)
		}
	}

	// close old ones
	for key, c := range pl.closeChannels {
		_, exists := allowedForwards[key]
		if !exists {
			// pl.log.Tracef("closing chan %s (port %d)", key, pl.port)
			close(c)
			delete(pl.closeChannels, key)
		}
	}

	pl.forwardMap = forwardMap
	return nil
}
