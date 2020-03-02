package main

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Request for our RequestList
type Request struct {
	Request *http.Request
	Start   time.Time
}

// RequestList store requests in thread-safe map
type RequestList struct {
	trace    bool
	requests map[uint64]Request
	mutex    sync.Mutex
}

// NewRequestList instances a new RequestList
// trace=bool will currently completely disable the list
func NewRequestList(trace bool) *RequestList {
	return &RequestList{
		trace:    trace,
		requests: make(map[uint64]Request),
	}
}

// AddRequest to the RequestList
func (rl *RequestList) AddRequest(id uint64, req *http.Request) {
	if !rl.trace {
		return
	}

	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	rl.requests[id] = Request{
		Request: req,
		Start:   time.Now(),
	}
}

// DeleteRequest from the RequestList
func (rl *RequestList) DeleteRequest(id uint64) {
	if !rl.trace {
		return
	}

	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	delete(rl.requests, id)
}

// Dump RequestList content to a Log
func (rl *RequestList) Dump(w io.Writer) {
	if !rl.trace {
		return
	}

	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now()
	fmt.Fprintf(w, "-- Requests (%d):\n", len(rl.requests))

	for id, request := range rl.requests {
		age := now.Sub(request.Start)
		req := request.Request
		fmt.Fprintf(w, "req %d: %s %s %s %s (%s)\n", id, req.RemoteAddr, req.Host, req.Method, req.RequestURI, age)
	}
}
