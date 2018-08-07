package main

import libvirt "github.com/libvirt/libvirt-go"

type LibvirtConnection struct {
	conn *libvirt.Connect
}

// TODO: deal with keep-alive, disconnections, etc

func NewLibvirtConnection(uri string) (*LibvirtConnection, error) {
	conn, err := libvirt.NewConnect(uri)
	if err != nil {
		return nil, err
	}

	return &LibvirtConnection{
		conn: conn,
	}, nil
}

func (lc *LibvirtConnection) Close() {
	lc.conn.Close()
}
