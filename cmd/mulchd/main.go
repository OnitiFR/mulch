package main

import (
	"flag"
	"log"

	"github.com/Xfennec/mulch/cmd/mulchd/server"
)

var configPath = flag.String("path", "./etc/", "configuration path")
var configTrace = flag.Bool("trace", false, "show trace message (debug)")

func main() {
	flag.Parse()

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
