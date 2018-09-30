package common

import "net/http/httputil"

// Domain defines a route for the reverse-proxy request handler
type Domain struct {
	Name            string
	VMName          string
	RedirectTo      string
	DestinationHost string
	DestinationPort int
	RedirectToHTTPS bool

	// used internaly by Mulch reverse proxy server
	ReverseProxy *httputil.ReverseProxy
	TargetURL    string
}
