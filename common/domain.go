package common

import "net/http/httputil"

// Domain defines a route for the reverse-proxy request handler
type Domain struct {
	Name            string
	VMName          string
	RedirectTo      string
	RedirectCode    int
	DestinationHost string
	DestinationPort int
	RedirectToHTTPS bool
	RateProfile     string

	// Pinned protects the domain on a proxy-chain parent: it belongs to a
	// promoted VM (failover, see mulchd's 'replica promote') and another
	// child's registration can neither overwrite nor reclaim it. Only its
	// owner (TargetURL) can update or release it.
	Pinned bool

	// used internaly by Mulch reverse proxy server
	ReverseProxy *httputil.ReverseProxy `json:"-"`
	TargetURL    string
	Chained      bool
}
