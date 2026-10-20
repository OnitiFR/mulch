package common

import "net"

// PortListeners is the port database shared by mulchd and mulch-proxy
type PortListeners struct {
	TCP TCPPortListeners
	UDP UDPPortListeners
}

type TCPForwarder struct {
	Dest           *net.TCPAddr
	PROXYProtoPort uint16
}

// TCPPortListener describe how the TCP proxy will listen on a port
type TCPPortListener struct {
	// example: net.ResolveTCPAddr("10.104.0.1:9001")
	ListenAddr *net.TCPAddr

	// the key is the client VM IPv4, value is the forward destination
	Forwards map[string]*TCPForwarder
}

// TCPPortListeners is a map of TCPPortListener instances, with the port as key
type TCPPortListeners map[uint16]*TCPPortListener

type UDPForwarder struct {
	Dest *net.UDPAddr
}

// UDPPortListener describe how the UDP proxy will listen on a port
type UDPPortListener struct {
	// example: net.ResolveUDPAddr("10.104.0.1:9001")
	ListenAddr *net.UDPAddr

	// the key is the client VM IPv4, value is the forward destination
	Forwards map[string]*UDPForwarder
}

// UDPPortListeners is a map of UDPPortListener instances, with the port as key
type UDPPortListeners map[uint16]*UDPPortListener
