package main

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/OnitiFR/mulch/common"
)

// TCPPortListener is a TCP listener holder
type TCPPortListener struct {
	ConnectionCount int32
	listenAddr      *net.TCPAddr
	listener        *net.TCPListener
	port            uint16
	public          bool
	forwardMap      map[string]*common.TCPForwarderConfig
	closeChannels   map[string]chan bool
	log             *Log
	closed          bool
	version         int
	mutex           sync.Mutex
}

// NewTCPPortListener will listen on port and forward clients
func NewTCPPortListener(listenAddr *net.TCPAddr, forwardMap map[string]*common.TCPForwarderConfig, version int, log *Log) (*TCPPortListener, error) {
	var err error

	listener := &TCPPortListener{
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
func (pl *TCPPortListener) Listen() {
	for {
		conn, err := pl.listener.AcceptTCP()
		if err != nil {
			pl.mutex.Lock()
			closed := pl.closed
			pl.mutex.Unlock()
			if closed {
				break
			}
			pl.log.Errorf("port proxy: %s: failed to accept connection: %s", pl.listenAddr.String(), err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		sourceIP := conn.RemoteAddr().(*net.TCPAddr).IP.String()

		var forwarder *common.TCPForwarderConfig
		var forwardTo net.TCPAddr
		var existsF bool

		pl.mutex.Lock()
		if pl.public {
			forwarder, existsF = pl.forwardMap["*"]
			if existsF {
				forwardTo = *forwarder.Dest
				if forwarder.PROXYProtoPort != 0 {
					forwardTo.Port = int(forwarder.PROXYProtoPort)
				}
			}
		} else {
			forwarder, existsF = pl.forwardMap[sourceIP]
			if existsF {
				forwardTo = *forwarder.Dest
			}
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
		pl.mutex.Unlock()

		if existsF && existsC {
			pl.log.Tracef("+ TCP %s->%s:%d", sourceIP, forwardTo.IP.String(), pl.port)

			var toPrelude []byte
			if forwarder.PROXYProtoPort != 0 {
				// inject PROXY protocol header (v1)
				p := fmt.Sprintf("PROXY TCP4 %s %s %d %d\r\n", sourceIP, "127.0.0.1", conn.RemoteAddr().(*net.TCPAddr).Port, forwarder.Dest.Port)
				toPrelude = []byte(p)
			}

			go NewTCPPortForward(conn, &forwardTo, closeChan, pl.log, &pl.ConnectionCount, toPrelude)
		} else {
			conn.Close()
		}
	}
}

// Close the listener
func (pl *TCPPortListener) Close() error {
	pl.mutex.Lock()
	defer pl.mutex.Unlock()

	pl.closed = true
	err := pl.listener.Close()
	if err != nil {
		return err
	}

	for key, c := range pl.closeChannels {
		close(c)
		delete(pl.closeChannels, key)
	}

	return nil
}

// UpdateForwardMap will update our forwarding config
func (pl *TCPPortListener) UpdateForwardMap(forwardMap map[string]*common.TCPForwarderConfig) error {
	pl.mutex.Lock()
	defer pl.mutex.Unlock()

	allowedForwards := make(map[string]bool)
	for srcIP, forward := range forwardMap {
		key := fmt.Sprintf("%s->%s", srcIP, forward.Dest.IP.String())
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
