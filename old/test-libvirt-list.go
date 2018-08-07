package old

import (
	"fmt"
	"log"

	libvirt "github.com/libvirt/libvirt-go"
)

func main003() {
	conn, err := libvirt.NewConnect("qemu:///system")
	//	conn, err := libvirt.NewConnect("qemu:///session")
	if err != nil {
		// domain: VIR_FROM_POLKIT (60)
		// code: VIR_ERR_AUTH_CANCELLED (79)
		// also found in the doc: VIR_ERR_AUTH_FAILED (45)
		// You may add your user to the libvirt group
		// (usermod --append --groups libvirt `whoami` - no need to reconnect your user)
		log.Fatal(err)
	}
	defer conn.Close()

	doms, err := conn.ListAllDomains(0)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%d domains:\n", len(doms))
	for _, dom := range doms {
		name, err := dom.GetName()
		if err == nil {
			fmt.Printf("  %s\n", name)
		}
		dom.Free()
	}
}
