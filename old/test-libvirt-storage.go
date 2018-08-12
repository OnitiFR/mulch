package old

// Found mulch network gateway address, or create the network

import (
	"log"

	libvirt "github.com/libvirt/libvirt-go"
)

func main005() {
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

	// vols, err := pool.ListAllStorageVolumes(0)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// for _, vol := range vols {
	// 	fmt.Println(vol.GetName())
	// }
}
