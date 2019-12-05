package server

import (
	"fmt"
	"net/http"
	"sync"
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
	startStreamChan chan bool
	streamStarted   bool
	streamMutex     sync.Mutex
}

// StartStream indicates that headers have been sent and "body" stream can start
func (req *Request) StartStream() {
	req.streamMutex.Lock()
	defer req.streamMutex.Unlock()

	// already started
	if req.streamStarted == true {
		return
	}

	req.streamStarted = true
	req.startStreamChan <- true
}

// WaitStream waits for StartStream()
func (req *Request) WaitStream() {
	_ = <-req.startStreamChan
}

// SetTarget define or change the default target for the request, for both
// sending (Stream) and receiving (HubClient)
func (req *Request) SetTarget(target string) {
	req.Stream.SetTarget(target)
	req.HubClient.SetTarget(target)
}

// Printf like helper for req.Response.Write
func (req *Request) Printf(format string, args ...interface{}) {
	req.Response.Write([]byte(fmt.Sprintf(format, args...)))
}

// Println like helper for req.Response.Write
func (req *Request) Println(message string) {
	req.Response.Write([]byte(fmt.Sprintf("%s\n", message)))
}
