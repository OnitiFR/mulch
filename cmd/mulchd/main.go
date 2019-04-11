package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/OnitiFR/mulch/cmd/mulchd/server"
)

var configPath = flag.String("path", "./etc/", "configuration path")
var configTrace = flag.Bool("trace", false, "show trace message (debug)")
var configVersion = flag.Bool("version", false, "show version")

func main() {
	flag.Parse()

	if *configVersion == true {
		fmt.Println(server.Version)
		os.Exit(0)
	}

	config, err := server.NewAppConfigFromTomlFile(*configPath)
	if err != nil {
		log.Fatalf("mulchd.conf (%s)': %s", *configPath, err)
	}

	app, err := server.NewApp(config, *configTrace)
	if err != nil {
		log.Fatalf("Fatal error: %s", err)
	}
	AddRoutes(app)
	app.Run()
}
