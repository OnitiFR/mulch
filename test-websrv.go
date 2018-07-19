package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
)

var addr = flag.String("addr", ":8080", "http service address")

func serveTest(w http.ResponseWriter, r *http.Request) {
	fmt.Println(r.Method)
	w.Write([]byte("Test."))
}

func main() {
	flag.Parse()

	http.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		serveTest(w, r)
	})

	fmt.Println("HTTP server listening:", *addr)
	err := http.ListenAndServe(*addr, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
