package old

import (
	"fmt"
	"io/ioutil"
	"log"

	"github.com/libvirt/libvirt-go-xml"
)

func main006() {
	// conn, err := libvirt.NewConnect("qemu:///system")
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// defer conn.Close()
	//
	// dom, err := conn.LookupDomainByName("win10")
	// if err != nil {
	// 	log.Fatal(err)
	// }
	//
	// xmldoc, err := dom.GetXMLDesc(0)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	//
	// domcfg := &libvirtxml.Domain{}
	// err = domcfg.Unmarshal(xmldoc)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	//
	// fmt.Printf("Virt type %s\n", domcfg.Type)

	xml, err := ioutil.ReadFile("test-virsh.xml")
	if err != nil {
		log.Fatal(err)
	}

	domcfg2 := &libvirtxml.Domain{}
	err = domcfg2.Unmarshal(string(xml))
	if err != nil {
		log.Fatal(err)
	}
	// fmt.Println(domcfg2.Memory, domcfg2.CurrentMemory, domcfg2.Devices.Interfaces)

	for _, intf := range domcfg2.Devices.Interfaces {
		//fmt.Println(intf.MAC)
		fmt.Println(intf.Source.Bridge.Bridge) // change this to mulch net Bridge
		fmt.Println(intf.MAC.Address)          // randomize that
	}

	domcfg2.Name = "test2"
	out, err := domcfg2.Marshal()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(out)
}
