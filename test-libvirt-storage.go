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

func GetOrCreateStoragePool(poolName string, poolPath string, mode string, conn *libvirt.Connect) (*libvirt.StoragePool, error) {
	pool, err := conn.LookupStoragePoolByName(poolName)
	if err != nil {
		virtErr := err.(libvirt.Error)
		if virtErr.Domain == libvirt.FROM_STORAGE && virtErr.Code == libvirt.ERR_NO_STORAGE_POOL {
			fmt.Printf("pool '%s' not found, let's create it\n", poolName)
			cwd, _ := os.Getwd()

			xml, err := ioutil.ReadFile("test-storage.xml")
			if err != nil {
				return nil, err
			}

			poolcfg := &libvirtxml.StoragePool{}
			err = poolcfg.Unmarshal(string(xml))
			if err != nil {
				return nil, fmt.Errorf("poolcfg.Unmarshal: %s", err)
			}

			poolcfg.Name = poolName
			poolcfg.Target.Path = cwd + "/" + poolPath

			if mode != "" {
				poolcfg.Target.Permissions.Mode = mode
			}

			out, err := poolcfg.Marshal()
			if err != nil {
				return nil, fmt.Errorf("poolcfg.Marshal: %s", err)
			}

			pool, err = conn.StoragePoolDefineXML(string(out), 0)
			if err != nil {
				return nil, fmt.Errorf("StoragePoolDefineXML: %s", err)
			}

			pool.SetAutostart(true)
			if err != nil {
				return nil, fmt.Errorf("pool.SetAutostart: %s", err)
			}

			// WITH_BUILD = will create target directory if net exists
			err = pool.Create(libvirt.STORAGE_POOL_CREATE_WITH_BUILD)
			if err != nil {
				return nil, fmt.Errorf("pool.Create: %s", err)
			}
		}
	}

	err = pool.Refresh(0)
	if err != nil {
		return nil, fmt.Errorf("pool.Refresh: %s", err)
	}
	return pool, nil
}

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

	// I've seen strange things once in a while, like:
	// - Code=38, Domain=0, Message='cannot open directory '…/storage/cloud-init': No such file or directory'
	// - Code=55, Domain=18, Message='Requested operation is not valid: storage pool 'mulch-cloud-init' is not active
	// Added more precise error messages to diagnose this.

	pool0, err := GetOrCreateStoragePool("mulch-cloud-init", "storage/cloud-init", "", conn)
	if err != nil {
		log.Fatal(err)
	}
	defer pool0.Free()

	pool1, err := GetOrCreateStoragePool("mulch-releases", "storage/releases", "", conn)
	if err != nil {
		log.Fatal(err)
	}
	defer pool1.Free()

	pool2, err := GetOrCreateStoragePool("mulch-disks", "storage/disks", "0711", conn)
	if err != nil {
		log.Fatal(err)
	}
	defer pool2.Free()

	// vols, err := pool.ListAllStorageVolumes(0)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// for _, vol := range vols {
	// 	fmt.Println(vol.GetName())
	// }
}
