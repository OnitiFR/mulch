package main

import (
	"fmt"
	"io/ioutil"
	"log"

	"github.com/libvirt/libvirt-go"
)

func main() {
	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	xml, err := ioutil.ReadFile("/home/xfennec/Clicproxy/KVM/test.xml")
	if err != nil {
		log.Fatal(err)
	}

	dom, err := conn.DomainDefineXML(string(xml))
	if err != nil {
		log.Fatal(err)
	}

	name, _ := dom.GetName()
	id, _ := dom.GetID()
	uuid, _ := dom.GetUUID()
	fmt.Println(name, id, uuid)

	err = dom.Create()
	if err != nil {
		log.Fatal(err)
	}
}
