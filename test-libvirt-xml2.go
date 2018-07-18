package main

import (
	"fmt"
	"log"

	libvirt "github.com/libvirt/libvirt-go"
	"github.com/libvirt/libvirt-go-xml"
)

func main() {
	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	dom, err := conn.LookupDomainByName("test")
	if err != nil {
		log.Fatal(err)
	}

	xmldoc, err := dom.GetXMLDesc(0)
	if err != nil {
		log.Fatal(err)
	}

	domcfg := &libvirtxml.Domain{}
	err = domcfg.Unmarshal(xmldoc)
	if err != nil {
		log.Fatal(err)
	}

	// we delete the 'config-2' filesystem
	tmp := domcfg.Devices.Filesystems[:0]
	for _, fs := range domcfg.Devices.Filesystems {
		if fs.Target.Dir != "config-2" {
			tmp = append(tmp, fs)
		}
	}
	domcfg.Devices.Filesystems = tmp
	domcfg.Description = "test desc Julien"

	out, err := domcfg.Marshal()
	if err != nil {
		log.Fatal(err)
	}

	// will update the domain. Cool.
	dom2, err := conn.DomainDefineXML(string(out))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(dom2)
}
