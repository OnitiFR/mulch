package main

import (
	"io"
	"net"
	"sync/atomic"
)

// PortForward is an actual established port forwarding
type PortForward struct {
	fromConn          io.ReadWriteCloser
	toConn            io.ReadWriteCloser
	closeChanInternal chan bool
	closeChanExternal chan bool
	closed            bool
	log               *Log
}

// NewPortForward will connect to remote host and forward connection
func NewPortForward(fromConn io.ReadWriteCloser, toAddr *net.TCPAddr, closeChanExternal chan bool, log *Log, connectionCount *int32, toPrelude []byte) {
	defer func() {
		err := fromConn.Close()
		if err != nil {
			log.Errorf("local close error: %s", err)
		}
	}()

	var err error

	pf := &PortForward{
		fromConn:          fromConn,
		closeChanInternal: make(chan bool),
		closeChanExternal: closeChanExternal,
		log:               log,
	}

	pf.toConn, err = net.DialTCP("tcp", nil, toAddr)

	if err != nil {
		pf.log.Errorf("port proxy: %s", err)
		return
	}

	defer func() {
		err := pf.toConn.Close()
		if err != nil {
			pf.log.Errorf("remote close error: %s", err)
		}
	}()

	if toPrelude != nil {
		pf.toConn.Write(toPrelude)
	}

	atomic.AddInt32(connectionCount, 1)

	go pf.pipe(pf.fromConn, pf.toConn)
	go pf.pipe(pf.toConn, pf.fromConn)

	select {
	case <-pf.closeChanInternal:
	case <-pf.closeChanExternal:
	}

	atomic.AddInt32(connectionCount, -1)
}

// pipe a reader to a writer
func (pf *PortForward) pipe(src io.Reader, dst io.Writer) {
	buff := make([]byte, 64*1024) // 64kB
	for {
		n, err := src.Read(buff)
		if err != nil {
			pf.close()
			return
		}
		b := buff[:n]

		_, err = dst.Write(b)
		if err != nil {
			pf.close()
			return
		}
	}
}

// close the forwarding
func (pf *PortForward) close() {
	if pf.closed {
		return
	}
	pf.closed = true
	close(pf.closeChanInternal)
}
