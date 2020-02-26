package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

var configPath = flag.String("path", "./etc/", "configuration path")
var configTrace = flag.Bool("trace", false, "show trace message (debug)")
var configVersion = flag.Bool("version", false, "show version")

func main() {
	flag.Parse()

	if *configVersion == true {
		fmt.Println(Version)
		os.Exit(0)
	}

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
