package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

var addr = flag.String("addr", ":8585", "http service address")

type Message struct {
	// SUCCESS, ERROR, INFO, TRACE
	Type    string `json:"type"`
	Message string `json:"message"`
}

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
			m := Message{
				Type:    "NOOP",
				Message: "",
			}

			err := enc.Encode(m)
			if err != nil {
				fmt.Println(err)
			}
			flusher.Flush()

		case msg := <-messages:
			m := Message{
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

func servePhone(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Invalid request method.", 405)
	}

	ip := strings.Split(r.RemoteAddr, ":")
	fmt.Printf("phoning: %s, ip=%s\n", r.PostFormValue("instance_id"), ip[0])

	w.Write([]byte("OK"))
}

func main() {
	flag.Parse()

	http.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		serveTest(w, r)
	})

	http.HandleFunc("/phone", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		servePhone(w, r)
	})

	fmt.Println("HTTP server listening:", *addr)
	err := http.ListenAndServe(*addr, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
