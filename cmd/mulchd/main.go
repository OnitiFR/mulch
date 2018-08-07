package main

import (
	"flag"
	"log"
)

var addr = flag.String("addr", ":8585", "http service address")

func main() {
	flag.Parse()

	app, err := NewApp()
	if err != nil {
		log.Fatalln(err)
	}
	app.Run()
}
