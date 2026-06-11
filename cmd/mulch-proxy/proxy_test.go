package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/OnitiFR/mulch/common"
)

// newTestProxyFront builds an httptest server that proxies to backendURL using
// the errorHandlingRoundTripper, optionally simulating the concurrent
// rate-limiter (which detaches the transport from client cancellation during
// the connection phase, see handleRequest / RoundTrip).
func newTestProxyFront(t *testing.T, backendURL string, withConcurrentLimit bool) *httptest.Server {
	t.Helper()

	target, err := url.Parse(backendURL)
	if err != nil {
		t.Fatalf("parse backend url: %s", err)
	}

	proxy := &ProxyServer{Log: NewLog(false)}
	rp := httputil.NewSingleHostReverseProxy(target)
	rp.Transport = &errorHandlingRoundTripper{
		ProxyServer: proxy,
		Domain:      &common.Domain{Name: "test"},
		Log:         proxy.Log,
	}

	front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if withConcurrentLimit {
			r = r.WithContext(context.WithValue(r.Context(), contextKeyNoCancelTransport, true))
		}
		rp.ServeHTTP(w, r)
	}))
	t.Cleanup(front.Close)
	return front
}

// TestDisconnectPropagatesDuringStreaming is the core regression test: when the
// concurrent rate-limiter is active, a client disconnection during a streaming
// (SSE-like) response must still reach the backend. Before the fix the backend
// stayed blocked because the transport context was detached for the whole
// request lifetime.
func TestDisconnectPropagatesDuringStreaming(t *testing.T) {
	for _, withLimit := range []bool{false, true} {
		name := "rateLimiterOff"
		if withLimit {
			name = "rateLimiterOn"
		}
		t.Run(name, func(t *testing.T) {
			backendCanceled := make(chan struct{})
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				io.WriteString(w, "data: hello\n\n")
				w.(http.Flusher).Flush()

				select {
				case <-r.Context().Done():
					close(backendCanceled)
				case <-time.After(5 * time.Second):
					t.Error("backend never saw the client disconnection")
				}
			}))
			defer backend.Close()

			front := newTestProxyFront(t, backend.URL, withLimit)

			ctx, cancel := context.WithCancel(context.Background())
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, front.URL, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("client request: %s", err)
			}

			// read the first streamed chunk so the backend has surely responded
			// (this is what arms the re-cancellation path)
			buf := make([]byte, 4)
			if _, err := resp.Body.Read(buf); err != nil {
				t.Fatalf("reading first chunk: %s", err)
			}

			// now simulate a client disconnection
			cancel()
			resp.Body.Close()

			select {
			case <-backendCanceled:
				// success: disconnection propagated to the backend
			case <-time.After(3 * time.Second):
				t.Fatal("disconnection did not propagate to the backend in time")
			}
		})
	}
}

// TestConcurrentSlotHeldDuringConnectionPhase checks that the anti-abuse intent
// is preserved: with the concurrent limiter active, a client that cancels
// *before* the backend has responded must NOT abort the backend request (the
// slot stays held until the backend responds).
func TestConcurrentSlotHeldDuringConnectionPhase(t *testing.T) {
	backendStarted := make(chan struct{})
	ctxDoneBeforeResponse := make(chan bool, 1)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(backendStarted)
		// simulate a slow backend "connection phase"
		select {
		case <-time.After(400 * time.Millisecond):
			ctxDoneBeforeResponse <- r.Context().Err() != nil
		case <-r.Context().Done():
			ctxDoneBeforeResponse <- true
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	front := newTestProxyFront(t, backend.URL, true)

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, front.URL, nil)

	go func() {
		<-backendStarted
		// client disconnects while the backend is still "connecting"
		cancel()
	}()

	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		resp.Body.Close()
	}

	select {
	case done := <-ctxDoneBeforeResponse:
		if done {
			t.Fatal("backend request was cancelled during the connection phase despite the concurrent limiter")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("backend did not report in time")
	}
}

// TestUpgradeStillWorksWithConcurrentLimit guards against the WebSocket / 101
// regression: the response body of an upgraded connection must remain an
// io.ReadWriteCloser (httputil.ReverseProxy type-asserts it), so the fix must
// not wrap it.
func TestUpgradeStillWorksWithConcurrentLimit(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, brw, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Errorf("backend hijack: %s", err)
			return
		}
		defer conn.Close()

		io.WriteString(brw, "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: mulchtest\r\n\r\n")
		brw.Flush()

		line, err := brw.ReadString('\n')
		if err != nil {
			return
		}
		io.WriteString(brw, "echo:"+line)
		brw.Flush()
	}))
	defer backend.Close()

	front := newTestProxyFront(t, backend.URL, true)

	frontAddr := strings.TrimPrefix(front.URL, "http://")
	c, err := net.Dial("tcp", frontAddr)
	if err != nil {
		t.Fatalf("dial front: %s", err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))

	fmt.Fprintf(c, "GET / HTTP/1.1\r\nHost: x\r\nConnection: Upgrade\r\nUpgrade: mulchtest\r\n\r\n")

	br := bufio.NewReader(c)
	statusLine, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("reading status line: %s", err)
	}
	if !strings.Contains(statusLine, "101") {
		t.Fatalf("expected 101 Switching Protocols, got %q", strings.TrimSpace(statusLine))
	}
	// consume headers up to the blank line
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("reading upgrade headers: %s", err)
		}
		if line == "\r\n" {
			break
		}
	}

	// the connection is now a raw bidirectional tunnel through the proxy
	fmt.Fprintf(c, "ping\n")
	echo, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("reading echo: %s", err)
	}
	if echo != "echo:ping\n" {
		t.Fatalf("expected %q, got %q", "echo:ping\n", echo)
	}
}
