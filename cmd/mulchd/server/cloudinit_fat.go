package server

import (
	"bytes"
	"io"
	"os"

	"github.com/mitchellh/go-fs"
	"github.com/mitchellh/go-fs/fat"
)

// CIFFile means CloudInitFat File, it describes a file that will
// be added to the FAT image
type CIFFile struct {
	Filename string
	Content  []byte
}

func cloudInitFatAddFile(dir fs.Directory, file CIFFile) error {
	subEntry, err := dir.AddFile(file.Filename)
	if err != nil {
		return err
	}

	sub, err := subEntry.File()
	if err != nil {
		return err
	}

	in := bytes.NewReader(file.Content)

	if _, err := io.Copy(sub, in); err != nil {
		return err
	}

	return nil
}

// CloudInitFatCreateImage creates a FAT12 image with inputFiles as a content
func CloudInitFatCreateImage(outputFile *os.File, size int64, inputFiles []CIFFile) error {

	if err := outputFile.Truncate(size); err != nil {
		outputFile.Close()
		os.Remove(outputFile.Name())
		return err
	}

	// BlockDevice backed by a file
	device, err := fs.NewFileDisk(outputFile)
	if err != nil {
		outputFile.Close()
		os.Remove(outputFile.Name())
		return err
	}

	// Format the block device so it contains a valid FAT filesystem
	formatConfig := &fat.SuperFloppyConfig{
		FATType: fat.FAT12,
		Label:   "cidata",
		OEMName: "cidata",
	}

	if fat.FormatSuperFloppy(device, formatConfig); err != nil {
		outputFile.Close()
		os.Remove(outputFile.Name())
		return err
	}

	filesys, err := fat.New(device)
	if err != nil {
		outputFile.Close()
		os.Remove(outputFile.Name())
		return err
	}

	rootDir, err := filesys.RootDir()
	if err != nil {
		outputFile.Close()
		os.Remove(outputFile.Name())
		return err
	}

	for _, file := range inputFiles {
		err = cloudInitFatAddFile(rootDir, file)
		if err != nil {
			return err
		}
	}

	outputFile.Close()
	return nil
}
