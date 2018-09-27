package main

import "net/http/httputil"

// Domain defines a route for the reverse-proxy request handler
type Domain struct {
	Name            string
	RedirectTo      string
	DestinationHost string
	DestinationPort int
	RedirectToHTTPS bool
	reverseProxy    *httputil.ReverseProxy
	targetURL       string
}
