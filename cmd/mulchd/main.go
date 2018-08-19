package main

import (
	"flag"
	"log"
)

var configPath = flag.String("path", "./etc/", "configuration path")

func main() {
	flag.Parse()

	config := &AppConfig{
		Listen:             ":8585",
		LibVirtURI:         "qemu:///system",
		StoragePath:        "./var/storage", // example: /srv/mulch
		DataPath:           "./var/data",    // example: /var/lib/mulch
		VMPrefix:           "mulch-",
		MulchSSHPrivateKey: "/home/xfennec/.ssh/id_rsa_mulch",
		MulchSSHPublicKey:  "/home/xfennec/.ssh/id_rsa_mulch.pub",

		configPath: *configPath, // example: /etc/mulch
	}

	app, err := NewApp(config)
	if err != nil {
		log.Fatalln(err)
	}
	app.Run()
}
