package main

import (
	"io"
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

func CloudInitCreateFatImg(outputFile string, size int64, inputFiles []string) error {

	f, err := os.Create(outputFile)
	if err != nil {
		return err
	}

	if err := f.Truncate(size); err != nil {
		f.Close()
		os.Remove(outputFile)
		return err
	}

	// BlockDevice backed by a file
	device, err := fs.NewFileDisk(f)
	if err != nil {
		f.Close()
		os.Remove(outputFile)
		return err
	}

	// Format the block device so it contains a valid FAT filesystem
	formatConfig := &fat.SuperFloppyConfig{
		FATType: fat.FAT12,
		Label:   "cidata",
		OEMName: "cidata",
	}

	if fat.FormatSuperFloppy(device, formatConfig); err != nil {
		f.Close()
		os.Remove(outputFile)
		return err
	}

	filesys, err := fat.New(device)
	if err != nil {
		f.Close()
		os.Remove(outputFile)
		return err
	}

	rootDir, err := filesys.RootDir()
	if err != nil {
		f.Close()
		os.Remove(outputFile)
		return err
	}

	for _, file := range inputFiles {
		err = addFile(rootDir, file)
		if err != nil {
			return err
		}
	}

	f.Close()
	return nil

}
