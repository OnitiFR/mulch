package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"time"
)

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

func main() {
	// 1 - copy from reference image
	createDiskFromRelease("debian-9-openstack-amd64.qcow2", "test.qcow2")

	// 2 - resize
	start := time.Now()

	disk := "storage/disks/" + "test.qcow2"
	cmd := "qemu-img"
	args := []string{"resize", disk, "20G"}
	out, err := exec.Command(cmd, args...).CombinedOutput()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	elapsed := time.Since(start)
	fmt.Print(string(out))
	fmt.Printf("took %s\n", elapsed)

	// then define domain using disk and virtfs-ci
	// (first test : define the domain with only its disk, to test
	// probable access issues between libvirt / qemu)
}
