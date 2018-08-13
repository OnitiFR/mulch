package main

import (
	"io"
	"os"

	"github.com/libvirt/libvirt-go"
)

// VolumeUpload contains source and destination for the upload operation
type VolumeUpload struct {
	streamSrc *os.File
	streamDst *libvirt.Stream
}

// NewVolumeUpload creates a VolumeUpload instance, allowing to upload
// a file to a libvirt storage pool
func NewVolumeUpload(srcFile string, connDst *libvirt.Connect, volDst *libvirt.StorageVol) (instance *VolumeUpload, err error) {
	streamSrc, err := os.Open(srcFile)
	if err != nil {
		return nil, err
	}

	streamDst, err := connDst.NewStream(0)
	if err != nil {
		return nil, err
	}

	err = volDst.Upload(streamDst, 0, 0, 0)
	if err != nil {
		return nil, err
	}

	instance = &VolumeUpload{
		streamSrc: streamSrc,
		streamDst: streamDst,
	}

	return instance, nil
}

func (v VolumeUpload) Write(p []byte) (n int, e error) {
	return v.streamDst.Send(p)
}

// Copy do the actual upload
func (v *VolumeUpload) Copy() (written int64, err error) {
	defer v.streamDst.Free()

	written, err = io.Copy(v, v.streamSrc)

	if err != nil {
		v.streamSrc.Close()
		v.streamDst.Abort()
		return written, err
	}

	if e := v.streamSrc.Close(); e != nil {
		return written, e
	}
	if e := v.streamDst.Finish(); e != nil {
		return written, e
	}

	return written, err
}
