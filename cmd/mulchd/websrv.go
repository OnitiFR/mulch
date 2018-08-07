package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/Xfennec/mulch"
)

func serveTest(w http.ResponseWriter, r *http.Request) {
	fmt.Println("new connecion")
	cn, ok := w.(http.CloseNotifier)
	if !ok {
		http.NotFound(w, r)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.NotFound(w, r)
		return
	}

	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))

	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	enc := json.NewEncoder(w)

	messages := make(chan string)

	go func() {
		for {
			delay := rnd.Intn(12000)
			time.Sleep(time.Duration(delay) * time.Millisecond)
			messages <- fmt.Sprintf("Hello %d", delay)
		}
	}()

	for {
		select {
		case <-cn.CloseNotify():
			fmt.Println("connection closed")
			return
		case <-time.After(10 * time.Second):
			// Keep-alive
			m := mulch.Message{
				Type:    "NOOP",
				Message: "",
			}

			err := enc.Encode(m)
			if err != nil {
				fmt.Println(err)
			}
			flusher.Flush()

		case msg := <-messages:
			m := mulch.Message{
				Type:    "INFO",
				Message: msg,
			}

			err := enc.Encode(m)
			if err != nil {
				fmt.Println(err)
			}
			flusher.Flush()
			break
		}
	}

}

func phoneController(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Invalid request method.", 405)
	}

	ip := strings.Split(r.RemoteAddr, ":")
	fmt.Printf("phoning: %s, ip=%s\n", r.PostFormValue("instance_id"), ip[0])

	w.Write([]byte("OK"))
}

func logController(w http.ResponseWriter, r *http.Request, hub *Hub) {
	if r.Method != "GET" {
		http.Error(w, "Invalid request method.", 405)
	}

	cn, ok := w.(http.CloseNotifier)
	if !ok {
		http.NotFound(w, r)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	enc := json.NewEncoder(w)

	// plug ourselves into the hub
	client := hub.Register("me")

	for {
		select {
		case <-cn.CloseNotify():
			client.Unregister()
			return
		case <-time.After(10 * time.Second):
			// Keep-alive
			// fmt.Println("keepalive")
			m := mulch.Message{
				Type:    "NOOP",
				Message: "",
			}

			err := enc.Encode(m)
			if err != nil {
				fmt.Println(err)
			}
			flusher.Flush()

		case msg := <-client.Messages:
			m := mulch.Message{
				Type:    "INFO",
				Message: string(msg),
			}

			err := enc.Encode(m)
			if err != nil {
				fmt.Println(err)
			}
			flusher.Flush()
			break
		}
	}
}
