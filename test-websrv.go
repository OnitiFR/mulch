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
	Level   string `json:"level"`
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

	// Send the initial headers saying we're gonna stream the response.
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	enc := json.NewEncoder(w)

	messages := make(chan string)

	go func() {
		for {
			delay := rnd.Intn(1200)
			time.Sleep(time.Duration(delay) * time.Millisecond)
			messages <- fmt.Sprintf("Hello %d", delay)
		}
	}()

	for {
		select {
		case <-cn.CloseNotify():
			fmt.Println("connection closed")
			return
		case <-time.After(1 * time.Second):
			m := Message{
				Level:   "NONE",
				Message: "",
			}

			// Send some data.
			err := enc.Encode(m)
			if err != nil {
				fmt.Println(err)
			}
			flusher.Flush()

		case msg := <-messages:
			// fmt.Println(".")
			m := Message{
				Level:   "INFO",
				Message: msg,
			}

			// Send some data.
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
