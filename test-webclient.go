package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
)

var host = flag.String("host", "http://localhost:8585", "Server host:port")

type Message struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

func main() {
	flag.Parse()

	req, err := http.NewRequest("GET", *host+"/test", nil)
	if err != nil {
		log.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		log.Fatalf("Status code is not OK: %v (%s)", resp.StatusCode, resp.Status)
	}

	dec := json.NewDecoder(resp.Body)
	for {
		var m Message
		err := dec.Decode(&m)
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatal(err)
		}
		fmt.Printf("%s: %s\n", m.Level, m.Message)
	}

}
