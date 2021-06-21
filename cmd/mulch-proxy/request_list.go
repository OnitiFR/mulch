package main

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Request for our RequestList
type Request struct {
	Request    *http.Request
	RemoteAddr string // serveReverseProxy() may change the original value
	Start      time.Time
}

// RequestList store requests in thread-safe map
type RequestList struct {
	enable   bool
	requests map[uint64]Request
	mutex    sync.Mutex
}

// NewRequestList instances a new RequestList
// debug=false will currently completely disable the list
func NewRequestList(enable bool) *RequestList {
	return &RequestList{
		enable:   enable,
		requests: make(map[uint64]Request),
	}
}

// AddRequest to the RequestList
func (rl *RequestList) AddRequest(id uint64, req *http.Request, remoteAddr string) {
	if !rl.enable {
		return
	}

	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	rl.requests[id] = Request{
		Request:    req,
		RemoteAddr: remoteAddr,
		Start:      time.Now(),
	}
}

// DeleteRequest from the RequestList
func (rl *RequestList) DeleteRequest(id uint64) {
	if !rl.enable {
		return
	}

	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	delete(rl.requests, id)
}

// Dump RequestList content to a Log
func (rl *RequestList) Dump(w io.Writer) {
	fmt.Fprintf(w, "-- Request Counter: %d\n", atomic.LoadUint64(&requestCounter))

	if !rl.enable {
		fmt.Fprintf(w, "-- No RequestList (debug not unabled)\n")
		return
	}

	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now()
	fmt.Fprintf(w, "-- Requests (%d):\n", len(rl.requests))

	for id, request := range rl.requests {
		age := now.Sub(request.Start)
		req := request.Request
		fmt.Fprintf(w, "req %d: %s %s %s %s (%s)\n", id, request.RemoteAddr, req.Host, req.Method, req.RequestURI, age)
	}
}
