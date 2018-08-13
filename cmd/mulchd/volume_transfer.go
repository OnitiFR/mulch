package main

import (
	"io"

	"github.com/libvirt/libvirt-go"
)

// VolumeTransfert contains source and destination for the transfert operation
type VolumeTransfert struct {
	streamSrc *libvirt.Stream
	streamDst *libvirt.Stream
}

// NewVolumeTransfert creates a VolumeTransfert instance, allowing to
// transfert a content from a libvirt storage pool to another
func NewVolumeTransfert(connSrc *libvirt.Connect, volSrc *libvirt.StorageVol, connDst *libvirt.Connect, volDst *libvirt.StorageVol) (instance *VolumeTransfert, err error) {
	streamSrc, err := connSrc.NewStream(0)
	if err != nil {
		return nil, err
	}

	streamDst, err := connDst.NewStream(0)
	if err != nil {
		return nil, err
	}

	err = volSrc.Download(streamSrc, 0, 0, 0)
	if err != nil {
		return nil, err
	}

	err = volDst.Upload(streamDst, 0, 0, 0)
	if err != nil {
		return nil, err
	}

	instance = &VolumeTransfert{
		streamSrc: streamSrc,
		streamDst: streamDst,
	}

	return instance, nil
}

func (v VolumeTransfert) Read(p []byte) (n int, e error) {
	return v.streamSrc.Recv(p)
}

func (v VolumeTransfert) Write(p []byte) (n int, e error) {
	return v.streamDst.Send(p)
}

// Copy do the actual transfert
func (v *VolumeTransfert) Copy() (written int64, err error) {
	defer v.streamSrc.Free()
	defer v.streamDst.Free()

	written, err = io.Copy(v, v)

	if err != nil {
		v.streamSrc.Abort()
		v.streamDst.Abort()
		return written, err
	}

	if e := v.streamSrc.Finish(); e != nil {
		return written, e
	}
	if e := v.streamDst.Finish(); e != nil {
		return written, e
	}

	return written, err
}
