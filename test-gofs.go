package main

import (
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/mitchellh/go-fs"
	"github.com/mitchellh/go-fs/fat"
)

func addFile(dir fs.Directory, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	subEntry, err := dir.AddFile(filepath.Base(src))
	if err != nil {
		return err
	}

	file, err := subEntry.File()
	if err != nil {
		return err
	}

	if _, err := io.Copy(file, in); err != nil {
		return err
	}

	return nil
}

func main() {
	f, err := os.Create("storage/cloud-init/test.img")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	if err := f.Truncate(256 * 1024); err != nil {
		panic(err)
	}

	// BlockDevice backed by a file
	device, err := fs.NewFileDisk(f)
	if err != nil {
		panic(err)
	}

	// Format the block device so it contains a valid FAT filesystem
	formatConfig := &fat.SuperFloppyConfig{
		FATType: fat.FAT12,
		Label:   "cidata",
		OEMName: "cidata",
	}
	if fat.FormatSuperFloppy(device, formatConfig); err != nil {
		log.Fatal("Error creating floppy: %s", err)
	}

	filesys, err := fat.New(device)
	if err != nil {
		panic(err)
	}

	rootDir, err := filesys.RootDir()
	if err != nil {
		panic(err)
	}

	err = addFile(rootDir, "ci-sample/meta-data")
	if err != nil {
		panic(err)
	}
	err = addFile(rootDir, "ci-sample/user-data")
	if err != nil {
		panic(err)
	}

}
