package main

import (
	"flag"
	"log"
	"path"
)

var configPath = flag.String("path", "./etc/", "configuration path")

// ConfigTrace is a global setting, used by other files
var ConfigTrace = flag.Bool("trace", false, "show trace message (debug)")

func main() {
	flag.Parse()
	configFile := path.Clean(*configPath + "/mulchd.toml")

	config, err := NewAppConfigFromTomlFile(configFile)
	if err != nil {
		log.Fatalf("config '%s': %s", configFile, err)
	}

	app, err := NewApp(config)
	if err != nil {
		log.Fatalf("Fatal error: %s", err)
	}
	app.Run()
}
