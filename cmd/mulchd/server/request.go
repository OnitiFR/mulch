package server

import (
	"fmt"
	"net"
	"net/http"
	"sync"

	"golang.org/x/crypto/ssh"
)

// Request describes a request and allows to build a response
type Request struct {
	Route           *Route
	SubPath         string
	HTTP            *http.Request
	Response        http.ResponseWriter
	App             *App
	Stream          *Log
	HubClient       *HubClient
	APIKey          *APIKey
	SSHPubKey       ssh.PublicKey // SSH key of the session (nil with a static API key)
	startStreamChan chan bool
	streamStarted   bool
	streamMutex     sync.Mutex
}

// StartStream indicates that headers have been sent and "body" stream can start
func (req *Request) StartStream() {
	req.streamMutex.Lock()
	defer req.streamMutex.Unlock()

	// already started
	if req.streamStarted {
		return
	}

	req.streamStarted = true
	req.startStreamChan <- true
}

// WaitStream waits for StartStream()
func (req *Request) WaitStream() {
	<-req.startStreamChan
}

// SetTarget define or change the default target for the request, for both
// sending (Stream) and receiving (HubClient)
func (req *Request) SetTarget(target string) {
	req.Stream.SetTarget(target)
	// there's no HubClient for RouteTypeCustom routes
	if req.HubClient != nil {
		req.HubClient.SetTarget(target)
	}
}

// Printf like helper for req.Response.Write
func (req *Request) Printf(format string, args ...interface{}) {
	req.Response.Write([]byte(fmt.Sprintf(format, args...)))
}

// Println like helper for req.Response.Write
func (req *Request) Println(message string) {
	req.Response.Write([]byte(fmt.Sprintf("%s\n", message)))
}

// IsAPIKeyAllowed will return true if the APIKey is allowed to do this request
func (req *Request) IsAPIKeyAllowed() bool {

	method := req.HTTP.Method
	path := req.HTTP.URL.Path

	return req.APIKey.IsAllowed(method, path, req.HTTP)
}

// RemoteIP returns the IP address of the client
func (req *Request) RemoteIP() string {
	ip, _, _ := net.SplitHostPort(req.HTTP.RemoteAddr)
	return ip
}

// Author describes who sent the request (API key, SSH key, IP), for alerts
func (req *Request) Author() string {
	auth := "static key"
	if req.SSHPubKey != nil {
		auth = ssh.FingerprintSHA256(req.SSHPubKey)
	}
	return fmt.Sprintf("'%s' (%s, from %s)", req.APIKey.Comment, auth, req.RemoteIP())
}
