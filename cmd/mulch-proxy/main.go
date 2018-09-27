package main

import (
	"flag"
	"log"
)

var configPath = flag.String("path", "./etc/", "configuration path")
var configTrace = flag.Bool("trace", false, "show trace message (debug)")

func main() {
	flag.Parse()

	config, err := NewAppConfigFromTomlFile(*configPath)
	if err != nil {
		log.Fatalf("mulchd.conf (%s)': %s", *configPath, err)
	}

	app, err := NewApp(config, *configTrace)
	if err != nil {
		log.Fatalf("Fatal error: %s", err)
	}
	app.Run()
}
