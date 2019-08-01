package volumes

import (
	"io"
	"os"

	"gopkg.in/libvirt/libvirt-go.v5"
)

// VolumeDownload contains source and destination for the download operation
type VolumeDownload struct {
	streamSrc *libvirt.Stream
	streamDst io.WriteCloser
}

// NewVolumeDownloadToWriter creates a VolumeDownload instance, allowing to download
// a file from a libvirt storage pool to an io.WriteCloser
func NewVolumeDownloadToWriter(volSrc *libvirt.StorageVol, connSrc *libvirt.Connect, streamDst io.WriteCloser) (instance *VolumeDownload, err error) {
	streamSrc, err := connSrc.NewStream(0)
	if err != nil {
		return nil, err
	}

	err = volSrc.Download(streamSrc, 0, 0, 0)
	if err != nil {
		return nil, err
	}

	instance = &VolumeDownload{
		streamSrc: streamSrc,
		streamDst: streamDst,
	}

	return instance, nil
}

// NewVolumeDownload is a variant that writes to a file
func NewVolumeDownload(volSrc *libvirt.StorageVol, connSrc *libvirt.Connect, dstFile string) (instance *VolumeDownload, err error) {
	streamDst, err := os.Create(dstFile)
	if err != nil {
		return nil, err
	}

	return NewVolumeDownloadToWriter(volSrc, connSrc, streamDst)
}

func (v VolumeDownload) Read(p []byte) (n int, e error) {
	return v.streamSrc.Recv(p)
}

// Copy do the actual download
func (v *VolumeDownload) Copy() (written int64, err error) {
	defer v.streamSrc.Free()

	written, err = io.Copy(v.streamDst, v)

	if err != nil {
		v.streamSrc.Abort()
		v.streamDst.Close()
		return written, err
	}

	if e := v.streamSrc.Finish(); e != nil {
		return written, e
	}
	if e := v.streamDst.Close(); e != nil {
		return written, e
	}

	return written, err
}
