package main

// Found mulch network gateway address, or create the network

import (
	"fmt"
	"io/ioutil"
	"log"

	"github.com/libvirt/libvirt-go"
	"github.com/libvirt/libvirt-go-xml"
)

func main() {
	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	net, err := conn.LookupNetworkByName("mulch")
	if err != nil {
		virtErr := err.(libvirt.Error)
		if virtErr.Domain == libvirt.FROM_NETWORK && virtErr.Code == libvirt.ERR_NO_NETWORK {
			fmt.Println("not found, it's OK, let's create it")

			xml, err := ioutil.ReadFile("test-net.xml")
			if err != nil {
				log.Fatal(err)
			}

			/*domnet := &libvirtxml.Network{}
			err = domnet.Unmarshal(string(xml))
			if err != nil {
				log.Fatal(err)
			}*/
			net, err = conn.NetworkDefineXML(string(xml))
			if err != nil {
				log.Fatal(err)
			}

			err = net.SetAutostart(true)
			if err != nil {
				log.Fatal(err)
			}

			err = net.Create()
			if err != nil {
				log.Fatal(err)
			}

			// os.Exit(0)
		} else {
			log.Fatal(err)
		}
	}

	xmldoc, err := net.GetXMLDesc(0)
	if err != nil {
		log.Fatal(err)
	}

	netcfg := &libvirtxml.Network{}
	err = netcfg.Unmarshal(xmldoc)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(netcfg.IPs[0].Address)
	fmt.Println(netcfg.Bridge.Name)

}
