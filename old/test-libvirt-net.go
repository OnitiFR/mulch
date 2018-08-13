package old

// Found mulch network gateway address, or create the network

import (
	"log"

	"github.com/libvirt/libvirt-go"
)

func main004() {
	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

}
