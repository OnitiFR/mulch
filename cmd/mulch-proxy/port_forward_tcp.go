package main

import (
	"io"
	"net"
	"sync"
	"sync/atomic"
)

// TCPPortForward is an actual established port forwarding
type TCPPortForward struct {
	fromConn          io.ReadWriteCloser
	toConn            io.ReadWriteCloser
	closeChanInternal chan bool
	closeChanExternal chan bool
	closeOnce         sync.Once
	log               *Log
}

// NewTCPPortForward will connect to remote host and forward connection
func NewTCPPortForward(fromConn io.ReadWriteCloser, toAddr *net.TCPAddr, closeChanExternal chan bool, log *Log, connectionCount *int32, toPrelude []byte) {
	defer func() {
		err := fromConn.Close()
		if err != nil {
			log.Errorf("local close error: %s", err)
		}
	}()

	var err error

	pf := &TCPPortForward{
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
func (pf *TCPPortForward) pipe(src io.Reader, dst io.Writer) {
	buff := make([]byte, 64*1024) // 64kB
	for {
		n, err := src.Read(buff)
		if n > 0 {
			_, werr := dst.Write(buff[:n])
			if werr != nil {
				pf.close()
				return
			}
		}
		if err != nil {
			pf.close()
			return
		}
	}
}

// close the forwarding (called by both pipes, possibly at the same time)
func (pf *TCPPortForward) close() {
	pf.closeOnce.Do(func() {
		close(pf.closeChanInternal)
	})
}
