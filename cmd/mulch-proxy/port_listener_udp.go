package main

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/OnitiFR/mulch/common"
)

// UDP session timeouts (same as Linux conntrack defaults)
const (
	// the VM never replied to the client
	UDPSessionTimeoutUnreplied = 30 * time.Second
	// the VM replied at least once
	UDPSessionTimeoutEstablished = 120 * time.Second
)

// UDPMaxSessionsPerListener is the maximum number of simultaneous UDP sessions
// for a listener, the least recently active session is evicted when reached
const UDPMaxSessionsPerListener = 1024

// UDPPortListener is a UDP listener holder
// Since UDP is connectionless, a "session" is created for each client
// address, with its own socket to the destination, and closed after some
// inactivity.
type UDPPortListener struct {
	ConnectionCount int32
	listenAddr      *net.UDPAddr
	conn            *net.UDPConn
	port            uint16
	public          bool
	forwardMap      map[string]*common.UDPForwarder
	sessions        map[string]*udpSession // key is the client address (ip:port)
	log             *Log
	closed          bool
	version         int
	mutex           sync.Mutex
}

// udpSession is a client of a UDPPortListener
type udpSession struct {
	clientAddr   *net.UDPAddr
	dest         *net.UDPAddr
	upstream     *net.UDPConn
	lastActivity atomic.Int64 // UnixNano
	replied      atomic.Bool
}

// NewUDPPortListener will listen on port and forward clients
func NewUDPPortListener(listenAddr *net.UDPAddr, forwardMap map[string]*common.UDPForwarder, version int, log *Log) (*UDPPortListener, error) {
	var err error

	listener := &UDPPortListener{
		listenAddr: listenAddr,
		port:       uint16(listenAddr.Port),
		sessions:   make(map[string]*udpSession),
		log:        log,
		version:    version,
		closed:     false,
	}

	_, exists := forwardMap["*"]
	if exists {
		listener.public = true
	}

	listener.conn, err = net.ListenUDP("udp", listenAddr)
	if err != nil {
		return nil, err
	}

	listener.UpdateForwardMap(forwardMap)

	go listener.Listen()

	return listener, nil
}

// Listen will wait for client datagrams
func (pl *UDPPortListener) Listen() {
	buff := make([]byte, 64*1024) // max UDP datagram size
	for {
		n, clientAddr, err := pl.conn.ReadFromUDP(buff)
		if err != nil {
			if pl.isClosed() {
				break
			}
			pl.log.Errorf("port proxy: %s: failed to read UDP datagram: %s", pl.listenAddr.String(), err)
			continue
		}

		session := pl.getSession(clientAddr)
		if session == nil {
			continue
		}

		_, err = session.upstream.Write(buff[:n])
		if err != nil {
			pl.log.Tracef("port proxy: UDP %s->%s: %s", clientAddr.String(), session.dest.String(), err)
		}
	}
}

// getSession returns the session of the client, creating it if needed,
// and updates its activity (nil is returned if the client is not allowed)
func (pl *UDPPortListener) getSession(clientAddr *net.UDPAddr) *udpSession {
	pl.mutex.Lock()
	defer pl.mutex.Unlock()

	if pl.closed {
		return nil
	}

	key := clientAddr.String()
	session, exists := pl.sessions[key]
	if exists {
		if session.remaining() > 0 {
			session.touch()
			return session
		}
		// expired, but relayReplies did not clean it yet
		delete(pl.sessions, key)
		session.upstream.Close()
	}

	forwarder := pl.findForwarder(clientAddr.IP.String())
	if forwarder == nil {
		return nil
	}

	if len(pl.sessions) >= UDPMaxSessionsPerListener {
		pl.evictOldestSession()
	}

	upstream, err := net.DialUDP("udp", nil, forwarder.Dest)
	if err != nil {
		pl.log.Errorf("port proxy: %s", err)
		return nil
	}

	session = &udpSession{
		clientAddr: clientAddr,
		dest:       forwarder.Dest,
		upstream:   upstream,
	}
	session.touch()
	pl.sessions[key] = session

	pl.log.Tracef("+ UDP %s->%s:%d", clientAddr.IP.String(), forwarder.Dest.IP.String(), pl.port)
	atomic.AddInt32(&pl.ConnectionCount, 1)
	go pl.relayReplies(session)

	return session
}

// findForwarder returns the forwarder for this source IP (or nil)
// (mutex must be locked)
func (pl *UDPPortListener) findForwarder(sourceIP string) *common.UDPForwarder {
	if pl.public {
		sourceIP = "*"
	}
	return pl.forwardMap[sourceIP]
}

// evictOldestSession closes the least recently active session
// (mutex must be locked)
func (pl *UDPPortListener) evictOldestSession() {
	var oldestKey string
	var oldest *udpSession
	for key, session := range pl.sessions {
		if oldest == nil || session.lastActivity.Load() < oldest.lastActivity.Load() {
			oldestKey = key
			oldest = session
		}
	}

	if oldest != nil {
		pl.log.Tracef("port proxy: %s: too many UDP sessions, evicting %s", pl.listenAddr.String(), oldestKey)
		delete(pl.sessions, oldestKey)
		oldest.upstream.Close()
	}
}

// relayReplies will send destination replies back to the client, until
// the session is closed or expired
func (pl *UDPPortListener) relayReplies(session *udpSession) {
	defer func() {
		pl.mutex.Lock()
		key := session.clientAddr.String()
		if pl.sessions[key] == session {
			delete(pl.sessions, key)
		}
		pl.mutex.Unlock()

		session.upstream.Close()
		atomic.AddInt32(&pl.ConnectionCount, -1)
		pl.log.Tracef("- UDP %s->%s:%d", session.clientAddr.IP.String(), session.dest.IP.String(), pl.port)
	}()

	buff := make([]byte, 64*1024)
	for {
		remaining := session.remaining()
		if remaining <= 0 {
			return
		}
		session.upstream.SetReadDeadline(time.Now().Add(remaining))

		n, err := session.upstream.Read(buff)
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				// client activity may have extended the session
				continue
			}
			if errors.Is(err, syscall.ECONNREFUSED) {
				// ICMP port unreachable: nothing is listening (yet?) in the VM
				continue
			}
			return
		}

		session.replied.Store(true)
		session.touch()

		_, err = pl.conn.WriteToUDP(buff[:n], session.clientAddr)
		if err != nil {
			pl.log.Tracef("port proxy: UDP %s<-%s: %s", session.clientAddr.String(), session.dest.String(), err)
		}
	}
}

// Close the listener
func (pl *UDPPortListener) Close() error {
	pl.mutex.Lock()
	defer pl.mutex.Unlock()

	pl.closed = true
	err := pl.conn.Close()
	if err != nil {
		return err
	}

	for key, session := range pl.sessions {
		session.upstream.Close()
		delete(pl.sessions, key)
	}

	return nil
}

// UpdateForwardMap will update our forwarding config, closing sessions
// that are no longer allowed
func (pl *UDPPortListener) UpdateForwardMap(forwardMap map[string]*common.UDPForwarder) {
	pl.mutex.Lock()
	defer pl.mutex.Unlock()

	pl.forwardMap = forwardMap

	for key, session := range pl.sessions {
		forwarder := pl.findForwarder(session.clientAddr.IP.String())
		if forwarder == nil || forwarder.Dest.String() != session.dest.String() {
			delete(pl.sessions, key)
			session.upstream.Close()
		}
	}
}

func (pl *UDPPortListener) isClosed() bool {
	pl.mutex.Lock()
	defer pl.mutex.Unlock()
	return pl.closed
}

// touch updates session last activity
func (s *udpSession) touch() {
	s.lastActivity.Store(time.Now().UnixNano())
}

// remaining returns the time left before the session expires
func (s *udpSession) remaining() time.Duration {
	timeout := UDPSessionTimeoutUnreplied
	if s.replied.Load() {
		timeout = UDPSessionTimeoutEstablished
	}
	idle := time.Since(time.Unix(0, s.lastActivity.Load()))
	return timeout - idle
}
