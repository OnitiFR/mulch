package main

import (
	"flag"
	"log"
)

var configPath = flag.String("path", "./etc/", "configuration path")

// ConfigTrace is a global setting, used by other files
var ConfigTrace = flag.Bool("trace", false, "show trace message (debug)")

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
		MulchSuperUser:     "mulch-cc",

		configPath: *configPath, // example: /etc/mulch
	}

	app, err := NewApp(config)
	if err != nil {
		log.Fatalf("Fatal error: %s", err)
	}
	app.Run()
}
