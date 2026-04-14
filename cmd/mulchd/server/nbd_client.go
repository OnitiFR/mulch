package server

import (
	"context"
	"fmt"

	nbd "github.com/digitalocean/go-nbd"
	"github.com/digitalocean/go-nbd/nbdmeta"
)

// NBDExtent describes a range of blocks with dirty/clean status
type NBDExtent struct {
	Length uint32
	Dirty  bool
}

// NBDClient wraps a go-nbd connection for reading from QEMU pull-mode backups
type NBDClient struct {
	conn       *nbd.Conn
	ExportSize uint64
}

// NewNBDClient connects to a QEMU NBD server via TCP, performs the newstyle
// handshake, negotiates metadata context for the dirty bitmap (if bitmapName
// is non-empty), and enters the transmission phase.
func NewNBDClient(address string, exportName string, bitmapName string) (*NBDClient, error) {
	uri, err := nbd.ParseURI("nbd://" + address)
	if err != nil {
		return nil, fmt.Errorf("NBD parse URI: %s", err)
	}

	var dialer nbd.Dialer
	conn, err := dialer.Dial(context.Background(), uri)
	if err != nil {
		return nil, fmt.Errorf("NBD dial '%s': %s", address, err)
	}

	if err := conn.Connect(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("NBD handshake: %s", err)
	}

	// negotiate structured replies + meta context for dirty bitmap
	if bitmapName != "" {
		if err := conn.StructuredReplies(); err != nil {
			conn.Close()
			return nil, fmt.Errorf("NBD structured replies: %s", err)
		}

		contextQuery := "qemu:dirty-bitmap:" + bitmapName
		err = conn.SetMetaContext(exportName, []string{contextQuery}, func(m nbd.MetaContext) error {
			return nil
		})
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("NBD set meta context: %s", err)
		}
	}

	// enter transmission phase
	var exportSize uint64
	err = conn.Go(exportName, nbd.InfoRequestAll(), func(info nbd.ExportInfo) error {
		if info.ValidExport {
			exportSize = info.Size
		}
		return nil
	})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("NBD go: %s", err)
	}

	if exportSize == 0 {
		conn.Close()
		return nil, fmt.Errorf("NBD server did not provide export size")
	}

	return &NBDClient{
		conn:       conn,
		ExportSize: exportSize,
	}, nil
}

// BlockStatus queries the block status (dirty bitmap) for a range.
// The provided slice is reused to avoid allocations; results are returned
// in the same slice (re-sliced).
func (c *NBDClient) BlockStatus(offset uint64, length uint32, extents []NBDExtent) ([]NBDExtent, error) {
	extents = extents[:0]
	err := c.conn.BlockStatus(offset, length, func(bs nbd.BlockStatus) error {
		for _, desc := range bs.Descriptors {
			flags := nbdmeta.DirtyBitmapFlags(desc.Status)
			extents = append(extents, NBDExtent{
				Length: desc.Length,
				Dirty:  flags.Dirty(),
			})
		}
		return nil
	}, 0)
	return extents, err
}

// Read reads data from the export at the given offset into buf[:length].
func (c *NBDClient) Read(offset uint64, length uint32, buf []byte) error {
	_, err := c.conn.Read(buf[:length], offset, 0)
	return err
}

// Close disconnects from the NBD server and closes the connection.
func (c *NBDClient) Close() error {
	c.conn.Disconnect()
	return c.conn.Close()
}
