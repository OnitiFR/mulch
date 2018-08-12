package main

import (
	"flag"
	"log"
)

var configPath = flag.String("path", "./etc/", "configuration path")

func main() {
	flag.Parse()

	config := &AppConfig{
		Listen:      ":8585",
		LibVirtURI:  "qemu:///system",
		StoragePath: "./var",

		configPath: *configPath,
	}

	app, err := NewApp(config)
	if err != nil {
		log.Fatalln(err)
	}
	app.Run()
}
