package common

import "net"

// PortListenersConfig is the port database shared by mulchd and mulch-proxy
type PortListenersConfig struct {
	TCP TCPPortListenersConfig
	UDP UDPPortListenersConfig
}

type TCPForwarderConfig struct {
	Dest           *net.TCPAddr
	PROXYProtoPort uint16
}

// TCPPortListenerConfig describe how the TCP proxy will listen on a port
type TCPPortListenerConfig struct {
	// example: net.ResolveTCPAddr("10.104.0.1:9001")
	ListenAddr *net.TCPAddr

	// the key is the client VM IPv4, value is the forward destination
	Forwards map[string]*TCPForwarderConfig
}

// TCPPortListenersConfig is a map of TCPPortListenerConfig instances, with the port as key
type TCPPortListenersConfig map[uint16]*TCPPortListenerConfig

type UDPForwarderConfig struct {
	Dest *net.UDPAddr
}

// UDPPortListenerConfig describe how the UDP proxy will listen on a port
type UDPPortListenerConfig struct {
	// example: net.ResolveUDPAddr("10.104.0.1:9001")
	ListenAddr *net.UDPAddr

	// the key is the client VM IPv4, value is the forward destination
	Forwards map[string]*UDPForwarderConfig
}

// UDPPortListenersConfig is a map of UDPPortListenerConfig instances, with the port as key
type UDPPortListenersConfig map[uint16]*UDPPortListenerConfig
