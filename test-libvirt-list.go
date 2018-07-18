package main

import (
	"fmt"
	"log"

	libvirt "github.com/libvirt/libvirt-go"
)

func main() {
	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
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
