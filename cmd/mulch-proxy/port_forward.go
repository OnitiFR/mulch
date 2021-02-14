package main

import (
	"io"
	"net"
)

// TODO: clean error logging (needed at all? trace?)

// PortForward is an actual established port forwarding
type PortForward struct {
	fromConn  io.ReadWriteCloser
	toConn    io.ReadWriteCloser
	closeChan chan bool
	closed    bool
	log       *Log
}

// NewPortForward will connect to remote host and forward connection
func NewPortForward(fromConn io.ReadWriteCloser, toAddr *net.TCPAddr, log *Log) {
	defer func() {
		err := fromConn.Close()
		if err != nil {
			log.Errorf("local close error: %s", err)
		}
	}()

	var err error

	pf := &PortForward{
		fromConn:  fromConn,
		closeChan: make(chan bool),
		log:       log,
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

	go pf.pipe(pf.fromConn, pf.toConn)
	go pf.pipe(pf.toConn, pf.fromConn)

	<-pf.closeChan
	pf.log.Info("closed")

}

// pipe a reader to a writer
func (pf *PortForward) pipe(src io.Reader, dst io.Writer) {
	buff := make([]byte, 64*1024) // 64kB
	for {
		n, err := src.Read(buff)
		if err != nil {
			pf.close(err)
			return
		}
		b := buff[:n]

		n, err = dst.Write(b)
		if err != nil {
			pf.close(err)
			return
		}
	}
}

// close the forwarding
func (pf *PortForward) close(err error) {
	// pf.log.Errorf("port forward error: %s", err)
	if pf.closed {
		return
	}
	pf.closed = true
	close(pf.closeChan)
}
