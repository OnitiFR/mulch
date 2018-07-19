package main

// Found mulch network gateway address, or create the network

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/libvirt/libvirt-go"
	"github.com/libvirt/libvirt-go-xml"
)

func main() {
	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// pools, err := conn.ListAllStoragePools(0)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	//
	// fmt.Printf("%d pools:\n", len(pools))
	// for _, pool := range pools {
	// 	name, err := pool.GetName()
	// 	if err == nil {
	// 		fmt.Printf("  %s\n", name)
	// 	}
	// 	pool.Free() // ?
	// }
	const poolName = "mulch"
	const poolPath = "storage"

	pool, err := conn.LookupStoragePoolByName(poolName)
	if err != nil {
		virtErr := err.(libvirt.Error)
		if virtErr.Domain == libvirt.FROM_STORAGE && virtErr.Code == libvirt.ERR_NO_STORAGE_POOL {
			fmt.Println("no pool found, let's create it")
			cwd, _ := os.Getwd()

			xml, err := ioutil.ReadFile("test-storage.xml")
			if err != nil {
				log.Fatal(err)
			}

			poolcfg := &libvirtxml.StoragePool{}
			err = poolcfg.Unmarshal(string(xml))
			if err != nil {
				log.Fatal(err)
			}

			poolcfg.Name = poolName
			poolcfg.Target.Path = cwd + "/storage"

			out, err := poolcfg.Marshal()
			if err != nil {
				log.Fatal(err)
			}

			pool, err = conn.StoragePoolDefineXML(string(out), 0)
			if err != nil {
				log.Fatal(err)
			}
			pool.SetAutostart(true)
			err = pool.Create(libvirt.STORAGE_POOL_CREATE_NORMAL)
			if err != nil {
				log.Fatal(err)
			}
		}
	}

	err = pool.Refresh(0)
	if err != nil {
		log.Fatal(err)
	}

	vols, err := pool.ListAllStorageVolumes(0)
	if err != nil {
		log.Fatal(err)
	}
	for _, vol := range vols {
		fmt.Println(vol.GetName())
	}
}
