// We put this file in the main package (and not in 'old') because
// it's close to integration in the main code of mulchd

package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/libvirt/libvirt-go"
	"github.com/libvirt/libvirt-go-xml"
)

func createDiskFromReleaseWithLibvirt(release string, disk string, conn *libvirt.Connect) {
	const poolReleasesName = "mulch-releases"
	const poolDisksName = "mulch-disks"
	poolReleases, err := conn.LookupStoragePoolByName(poolReleasesName)
	if err != nil {
		log.Fatal(err)
	}
	defer poolReleases.Free()

	poolDisks, err := conn.LookupStoragePoolByName(poolDisksName)
	if err != nil {
		log.Fatal(err)
	}
	defer poolDisks.Free()

	// find source volume
	volSrc, err := poolReleases.LookupStorageVolByName(release)
	if err != nil {
		log.Fatal(err)
	}
	defer volSrc.Free()

	// create dest volume
	xml, err := ioutil.ReadFile("test-volume.xml")
	if err != nil {
		log.Fatal(err)
	}

	volcfg := &libvirtxml.StorageVolume{}
	err = volcfg.Unmarshal(string(xml))
	if err != nil {
		log.Fatal(err)
	}
	volcfg.Name = disk
	cwd, _ := os.Getwd()
	volcfg.Target.Path = cwd + "/storage/disks/" + disk
	// volObj.Target.Format.Type = "raw"

	xml2, err := volcfg.Marshal()
	if err != nil {
		log.Fatal(err)
	}
	volDst, err := poolDisks.StorageVolCreateXML(string(xml2), 0)
	if err != nil {
		log.Fatal(err)
	}
	defer volDst.Free()

	vt, err := NewVolumeTransfert(conn, volSrc, conn, volDst)
	if err != nil {
		log.Fatal(err)
	}

	bytesWritten, err := vt.Copy()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("transfered %d MiB (%s → %s)\n", bytesWritten/1024/1024, release, disk)
}

/*
func createDiskFromRelease(release string, disk string) {
	start := time.Now()

	srcFile, err := os.Open("storage/releases/" + release)
	if err != nil {
		log.Fatal(err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create("storage/disks/" + disk)
	if err != nil {
		log.Fatal(err)
	}
	defer dstFile.Close()

	bytesWritten, err := io.Copy(dstFile, srcFile)
	if err != nil {
		log.Fatal(err)
	}
	dstFile.Sync() // so timing below is fair

	elapsed := time.Since(start)
	fmt.Printf("copied %d MiB (%s → %s)\n", bytesWritten/1024/1024, release, disk)
	fmt.Printf("took %s\n", elapsed)
}
*/

func resizeDiskWithLibvirt(disk string, size uint64, conn *libvirt.Connect) {
	// Should have a look at virStorageVolResize() !
	const poolDisksName = "mulch-disks"

	poolDisks, err := conn.LookupStoragePoolByName(poolDisksName)
	if err != nil {
		log.Fatal(err)
	}
	defer poolDisks.Free()

	vol, err := poolDisks.LookupStorageVolByName(disk)
	if err != nil {
		log.Fatal(err)
	}
	defer vol.Free()

	err = vol.Resize(size, 0)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Reised %s to %d\n", disk, size)
}

/*
func resizeDisk(disk string, size string) {
	start := time.Now()

	diskPath := "storage/disks/" + disk
	cmd := "qemu-img"
	args := []string{"resize", diskPath, size}
	out, err := exec.Command(cmd, args...).CombinedOutput()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	elapsed := time.Since(start)
	fmt.Print(string(out))
	fmt.Printf("took %s\n", elapsed)
}
*/

// UploadCloudInitImageToLibvirt uploads CloudInit FAT image to libvirt storage
func UploadCloudInitImageToLibvirt(localImagePath string, asName string, conn *libvirt.Connect) error {
	const poolCiName = "mulch-cloud-init"

	poolCi, err := conn.LookupStoragePoolByName(poolCiName)
	if err != nil {
		return err
	}
	defer poolCi.Free()

	// create dest volume
	xml, err := ioutil.ReadFile("test-volume.xml")
	if err != nil {
		return err
	}

	volcfg := &libvirtxml.StorageVolume{}
	err = volcfg.Unmarshal(string(xml))
	if err != nil {
		return err
	}
	volcfg.Name = asName
	cwd, _ := os.Getwd()
	volcfg.Target.Path = cwd + "/storage/cloud-init/" + asName
	// volObj.Target.Format.Type = "raw"

	xml2, err := volcfg.Marshal()
	if err != nil {
		return err
	}
	volDst, err := poolCi.StorageVolCreateXML(string(xml2), 0)
	if err != nil {
		return err
	}
	defer volDst.Free()

	vu, err := NewVolumeUpload(localImagePath, conn, volDst)
	if err != nil {
		return err
	}

	bytesWritten, err := vu.Copy()
	if err != nil {
		return err
	}

	fmt.Printf("transfered %d MiB (%s → %s)\n", bytesWritten/1024/1024, localImagePath, asName)
	return nil
}

func createAndUploadCloudInitImage(imageName string, conn *libvirt.Connect) {
	const tmpFile = "tmp.img"
	const poolCiName = "mulch-cloud-init"

	poolCi, err := conn.LookupStoragePoolByName(poolCiName)
	if err != nil {
		log.Fatal(err)
	}
	defer poolCi.Free()

	// err = CloudInitFatCreateImage(tmpFile, 256*1024, []string{"ci-sample/meta-data", "ci-sample/user-data"})
	// if err != nil {
	// 	log.Fatalf("CloudInitCreateFatImg: %s", err)
	// }
	defer os.Remove(tmpFile)

	err = UploadCloudInitImageToLibvirt(tmpFile, imageName, conn)
	if err != nil {
		log.Fatal(err)
	}
}

func main001() {
	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	exists, err := conn.LookupDomainByName("test0")
	if err == nil { // && exists not nil (probably)
		fmt.Println(exists)
		log.Fatal("domain seems already defined")
	}

	// 1 - copy from reference image
	// createDiskFromRelease("debian-9-openstack-amd64.qcow2", "test.qcow2")
	createDiskFromReleaseWithLibvirt("debian-9-openstack-amd64.qcow2", "test.qcow2", conn)

	// 2 - resize
	// resizeDisk("test.qcow2", "20G")
	resizeDiskWithLibvirt("test.qcow2", 20*1024*1024*1024, conn)

	// 3 - create cloud-init disk
	createAndUploadCloudInitImage("test.img", conn)

	// 4 - define domain
	// should dynamically define:
	// - name
	// - CPU count, RAM amount
	// - CPU topology
	// - main qcow2 disk path
	// - cloud init disk path
	// - bridge interface name
	// - interface MAC address
	// - injected SSH key?
	xml, err := ioutil.ReadFile("test-ci-disk.xml")
	if err != nil {
		log.Fatal(err)
	}

	domcfg := &libvirtxml.Domain{}
	err = domcfg.Unmarshal(string(xml))
	if err != nil {
		log.Fatal(err)
	}
	// fmt.Println(domcfg2.Memory, domcfg2.CurrentMemory, domcfg2.Devices.Interfaces)

	domcfg.Name = "test0"

	cwd, _ := os.Getwd()

	for _, disk := range domcfg.Devices.Disks {
		if disk.Alias != nil && disk.Alias.Name == "ua-mulch-disk" {
			disk.Source.File.File = cwd + "/storage/disks/test.qcow2"
		}
		if disk.Alias != nil && disk.Alias.Name == "ua-mulch-cloudinit" {
			disk.Source.File.File = cwd + "/storage/cloud-init/test.img"
		}
	}

	for _, intf := range domcfg.Devices.Interfaces {
		//fmt.Println(intf.MAC)
		fmt.Println(intf.Source.Bridge.Bridge) // change this to mulch net Bridge
		fmt.Println(intf.MAC.Address)          // randomize that
	}

	xml2, err := domcfg.Marshal()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("creating domain")
	dom, err := conn.DomainDefineXML(string(xml2))
	if err != nil {
		log.Fatal(err)
	}

	name, _ := dom.GetName()
	id, _ := dom.GetID()
	uuid, _ := dom.GetUUIDString()
	fmt.Println(name, id, uuid)

	fmt.Println("starting domain")
	err = dom.Create()
	if err != nil {
		log.Fatal(err)
	}
}
