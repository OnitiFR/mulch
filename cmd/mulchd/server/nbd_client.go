package server

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

// NBD protocol constants (minimal subset for pull-mode backup reading)
const (
	nbdMagic    uint64 = 0x4e42444d41474943 // "NBDMAGIC"
	nbdOptMagic uint64 = 0x49484156454F5054 // "IHAVEOPT"

	nbdFlagFixedNewstyle  uint16 = 1 << 0
	nbdFlagNoZeroes       uint16 = 1 << 1
	nbdFlagCFixedNewstyle uint32 = 1 << 0
	nbdFlagCNoZeroes      uint32 = 1 << 1

	// Transmission flags
	nbdFlagHasFlags         uint16 = 1 << 0
	nbdFlagSendDF           uint16 = 1 << 7
	nbdFlagCanMultiConn     uint16 = 1 << 8
	nbdFlagSendBlockStatus  uint16 = 1 << 12

	// Option codes
	nbdOptGo              uint32 = 7
	nbdOptStructuredReply uint32 = 8
	nbdOptSetMetaContext  uint32 = 10

	// Option reply types
	nbdRepAck         uint32 = 1
	nbdRepInfo        uint32 = 3
	nbdRepMetaContext uint32 = 4
	nbdRepErrUnsup    uint32 = (1 << 31) | 1

	// Info types
	nbdInfoExport uint16 = 0

	// Command codes
	nbdCmdRead        uint16 = 0
	nbdCmdDisc        uint16 = 2
	nbdCmdBlockStatus uint16 = 7

	// Request magic
	nbdRequestMagic uint32 = 0x25609513

	// Reply types
	nbdSimpleReplyMagic     uint32 = 0x67446698
	nbdStructuredReplyMagic uint32 = 0x668e33ef

	// Structured reply types
	nbdReplyTypeNone        uint16 = 0
	nbdReplyTypeError       uint16 = (1 << 15) | 1
	nbdReplyTypeBlockStatus uint16 = 5

	// Structured reply flags
	nbdReplyFlagDone uint16 = 1 << 0

	// Command flags
	nbdCmdFlagReqOne uint16 = 1 << 3
)

// NBDExtent describes a range of blocks with status flags.
// For qemu:dirty-bitmap context: bit 0 set = dirty, bit 0 clear = clean.
type NBDExtent struct {
	Length uint32
	Flags  uint32
}

// NBDClient is a minimal NBD client for reading from QEMU pull-mode backups
type NBDClient struct {
	conn              net.Conn
	ExportSize        uint64
	metaContextID     uint32
	handle            uint64
	structuredReplies bool
}

// NewNBDClient connects to a QEMU NBD server (Unix socket), performs the
// newstyle handshake, negotiates metadata context for the dirty bitmap,
// and enters the transmission phase.
//
// bitmapName can be empty for full copy (no bitmap negotiation).
func NewNBDClient(address string, exportName string, bitmapName string) (*NBDClient, error) {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("NBD connect to '%s': %s", address, err)
	}

	c := &NBDClient{conn: conn}

	if err := c.handshake(exportName, bitmapName); err != nil {
		conn.Close()
		return nil, err
	}

	return c, nil
}

func (c *NBDClient) handshake(exportName string, bitmapName string) error {
	// 1. Read server magic + opt magic + flags
	var serverMagic, optMagic uint64
	var globalFlags uint16

	if err := binary.Read(c.conn, binary.BigEndian, &serverMagic); err != nil {
		return fmt.Errorf("reading server magic: %s", err)
	}
	if serverMagic != nbdMagic {
		return fmt.Errorf("bad server magic: %x", serverMagic)
	}

	if err := binary.Read(c.conn, binary.BigEndian, &optMagic); err != nil {
		return fmt.Errorf("reading opt magic: %s", err)
	}
	if optMagic != nbdOptMagic {
		return fmt.Errorf("bad opt magic: %x", optMagic)
	}

	if err := binary.Read(c.conn, binary.BigEndian, &globalFlags); err != nil {
		return fmt.Errorf("reading global flags: %s", err)
	}
	if globalFlags&nbdFlagFixedNewstyle == 0 {
		return fmt.Errorf("server does not support fixed newstyle")
	}

	// 2. Send client flags
	clientFlags := nbdFlagCFixedNewstyle
	if globalFlags&nbdFlagNoZeroes != 0 {
		clientFlags |= nbdFlagCNoZeroes
	}
	if err := binary.Write(c.conn, binary.BigEndian, clientFlags); err != nil {
		return fmt.Errorf("sending client flags: %s", err)
	}

	// 3. Negotiate structured replies + metadata context (if bitmap specified)
	if bitmapName != "" {
		if err := c.optStructuredReply(); err != nil {
			return fmt.Errorf("OPT_STRUCTURED_REPLY: %s", err)
		}
		contextQuery := "qemu:dirty-bitmap:" + bitmapName
		if err := c.optSetMetaContext(exportName, contextQuery); err != nil {
			return fmt.Errorf("SET_META_CONTEXT: %s", err)
		}
	}

	// 4. OPT_GO to enter transmission phase
	if err := c.optGo(exportName); err != nil {
		return fmt.Errorf("OPT_GO: %s", err)
	}

	return nil
}

// optStructuredReply negotiates structured replies with the server.
// Required before OPT_SET_META_CONTEXT.
func (c *NBDClient) optStructuredReply() error {
	// OPT_STRUCTURED_REPLY has no payload
	if err := binary.Write(c.conn, binary.BigEndian, nbdOptMagic); err != nil {
		return err
	}
	if err := binary.Write(c.conn, binary.BigEndian, nbdOptStructuredReply); err != nil {
		return err
	}
	if err := binary.Write(c.conn, binary.BigEndian, uint32(0)); err != nil {
		return err
	}

	replyType, _, err := c.readOptionReply(nbdOptStructuredReply)
	if err != nil {
		return err
	}
	if replyType != nbdRepAck {
		return fmt.Errorf("server rejected structured replies (reply type %x)", replyType)
	}
	c.structuredReplies = true
	return nil
}

// optSetMetaContext negotiates a metadata context with the server
func (c *NBDClient) optSetMetaContext(exportName string, contextQuery string) error {
	// OPT_SET_META_CONTEXT payload:
	//   uint32 export name length + export name bytes
	//   uint32 number of queries (1)
	//   uint32 query length + query bytes
	exportNameBytes := []byte(exportName)
	queryBytes := []byte(contextQuery)

	payloadLen := 4 + len(exportNameBytes) + 4 + 4 + len(queryBytes)

	// Send option header
	if err := binary.Write(c.conn, binary.BigEndian, nbdOptMagic); err != nil {
		return err
	}
	if err := binary.Write(c.conn, binary.BigEndian, nbdOptSetMetaContext); err != nil {
		return err
	}
	if err := binary.Write(c.conn, binary.BigEndian, uint32(payloadLen)); err != nil {
		return err
	}

	// Export name
	if err := binary.Write(c.conn, binary.BigEndian, uint32(len(exportNameBytes))); err != nil {
		return err
	}
	if _, err := c.conn.Write(exportNameBytes); err != nil {
		return err
	}

	// Number of queries
	if err := binary.Write(c.conn, binary.BigEndian, uint32(1)); err != nil {
		return err
	}

	// Query string
	if err := binary.Write(c.conn, binary.BigEndian, uint32(len(queryBytes))); err != nil {
		return err
	}
	if _, err := c.conn.Write(queryBytes); err != nil {
		return err
	}

	// Read replies until REP_ACK
	for {
		replyType, data, err := c.readOptionReply(nbdOptSetMetaContext)
		if err != nil {
			return err
		}

		if replyType == nbdRepMetaContext && len(data) >= 4 {
			c.metaContextID = binary.BigEndian.Uint32(data[:4])
			c.structuredReplies = true
		} else if replyType == nbdRepAck {
			break
		} else if replyType&(1<<31) != 0 {
			return fmt.Errorf("server rejected SET_META_CONTEXT (reply type %x)", replyType)
		}
	}

	return nil
}

// optGo sends OPT_GO and reads the export info
func (c *NBDClient) optGo(exportName string) error {
	nameBytes := []byte(exportName)

	// OPT_GO payload: uint32 name length + name + uint16 num info requests (0)
	payloadLen := 4 + len(nameBytes) + 2

	if err := binary.Write(c.conn, binary.BigEndian, nbdOptMagic); err != nil {
		return err
	}
	if err := binary.Write(c.conn, binary.BigEndian, nbdOptGo); err != nil {
		return err
	}
	if err := binary.Write(c.conn, binary.BigEndian, uint32(payloadLen)); err != nil {
		return err
	}
	if err := binary.Write(c.conn, binary.BigEndian, uint32(len(nameBytes))); err != nil {
		return err
	}
	if _, err := c.conn.Write(nameBytes); err != nil {
		return err
	}
	if err := binary.Write(c.conn, binary.BigEndian, uint16(0)); err != nil {
		return err
	}

	// Read replies until REP_ACK
	for {
		replyType, data, err := c.readOptionReply(nbdOptGo)
		if err != nil {
			return err
		}

		if replyType == nbdRepInfo && len(data) >= 12 {
			infoType := binary.BigEndian.Uint16(data[:2])
			if infoType == nbdInfoExport {
				c.ExportSize = binary.BigEndian.Uint64(data[2:10])
				// transmission flags at data[10:12]
			}
		} else if replyType == nbdRepAck {
			break
		} else if replyType&(1<<31) != 0 {
			return fmt.Errorf("server rejected OPT_GO (reply type %x)", replyType)
		}
	}

	if c.ExportSize == 0 {
		return fmt.Errorf("server did not provide export size")
	}

	return nil
}

// readOptionReply reads a single option reply from the server
func (c *NBDClient) readOptionReply(expectedOption uint32) (uint32, []byte, error) {
	var replyMagic uint64
	var optionCode uint32
	var replyType uint32
	var dataLen uint32

	if err := binary.Read(c.conn, binary.BigEndian, &replyMagic); err != nil {
		return 0, nil, fmt.Errorf("reading reply magic: %s", err)
	}
	if replyMagic != 0x3e889045565a9 {
		return 0, nil, fmt.Errorf("bad option reply magic: %x", replyMagic)
	}

	if err := binary.Read(c.conn, binary.BigEndian, &optionCode); err != nil {
		return 0, nil, err
	}
	if optionCode != expectedOption {
		return 0, nil, fmt.Errorf("unexpected option code %d (expected %d)", optionCode, expectedOption)
	}

	if err := binary.Read(c.conn, binary.BigEndian, &replyType); err != nil {
		return 0, nil, err
	}
	if err := binary.Read(c.conn, binary.BigEndian, &dataLen); err != nil {
		return 0, nil, err
	}

	var data []byte
	if dataLen > 0 {
		data = make([]byte, dataLen)
		if _, err := io.ReadFull(c.conn, data); err != nil {
			return 0, nil, fmt.Errorf("reading reply data: %s", err)
		}
	}

	return replyType, data, nil
}

func (c *NBDClient) nextHandle() uint64 {
	c.handle++
	return c.handle
}

// sendCommand sends an NBD transmission request
func (c *NBDClient) sendCommand(cmdType uint16, cmdFlags uint16, offset uint64, length uint32) (uint64, error) {
	handle := c.nextHandle()

	if err := binary.Write(c.conn, binary.BigEndian, nbdRequestMagic); err != nil {
		return 0, err
	}
	if err := binary.Write(c.conn, binary.BigEndian, cmdFlags); err != nil {
		return 0, err
	}
	if err := binary.Write(c.conn, binary.BigEndian, cmdType); err != nil {
		return 0, err
	}
	if err := binary.Write(c.conn, binary.BigEndian, handle); err != nil {
		return 0, err
	}
	if err := binary.Write(c.conn, binary.BigEndian, offset); err != nil {
		return 0, err
	}
	if err := binary.Write(c.conn, binary.BigEndian, length); err != nil {
		return 0, err
	}

	return handle, nil
}

// Read reads data from the export at the given offset into buf[:length]
func (c *NBDClient) Read(offset uint64, length uint32, buf []byte) error {
	_, err := c.sendCommand(nbdCmdRead, 0, offset, length)
	if err != nil {
		return fmt.Errorf("sending READ command: %s", err)
	}

	if c.structuredReplies {
		return c.readStructuredReadReply(buf[:length], offset)
	}
	return c.readSimpleReadReply(buf[:length])
}

func (c *NBDClient) readSimpleReadReply(buf []byte) error {
	var magic uint32
	var errCode uint32
	var handle uint64

	if err := binary.Read(c.conn, binary.BigEndian, &magic); err != nil {
		return err
	}
	if magic != nbdSimpleReplyMagic {
		return fmt.Errorf("bad simple reply magic: %x", magic)
	}
	if err := binary.Read(c.conn, binary.BigEndian, &errCode); err != nil {
		return err
	}
	if err := binary.Read(c.conn, binary.BigEndian, &handle); err != nil {
		return err
	}
	if errCode != 0 {
		return fmt.Errorf("NBD read error: %d", errCode)
	}

	_, err := io.ReadFull(c.conn, buf)
	return err
}

func (c *NBDClient) readStructuredReadReply(buf []byte, requestOffset uint64) error {
	totalRead := 0

	for totalRead < len(buf) {
		var magic uint32
		var flags uint16
		var replyType uint16
		var handle uint64
		var dataLen uint32

		if err := binary.Read(c.conn, binary.BigEndian, &magic); err != nil {
			return err
		}
		if magic != nbdStructuredReplyMagic {
			// might be a simple reply
			if magic == nbdSimpleReplyMagic {
				var errCode uint32
				if err := binary.Read(c.conn, binary.BigEndian, &errCode); err != nil {
					return fmt.Errorf("reading simple reply error code: %s", err)
				}
				if err := binary.Read(c.conn, binary.BigEndian, &handle); err != nil {
					return fmt.Errorf("reading simple reply handle: %s", err)
				}
				if errCode != 0 {
					return fmt.Errorf("NBD read error: %d", errCode)
				}
				_, err := io.ReadFull(c.conn, buf[totalRead:])
				return err
			}
			return fmt.Errorf("bad reply magic: %x", magic)
		}

		if err := binary.Read(c.conn, binary.BigEndian, &flags); err != nil {
			return err
		}
		if err := binary.Read(c.conn, binary.BigEndian, &replyType); err != nil {
			return err
		}
		if err := binary.Read(c.conn, binary.BigEndian, &handle); err != nil {
			return err
		}
		if err := binary.Read(c.conn, binary.BigEndian, &dataLen); err != nil {
			return err
		}

		if replyType&(1<<15) != 0 {
			// error reply — skip data
			discard := make([]byte, dataLen)
			io.ReadFull(c.conn, discard)
			return fmt.Errorf("NBD structured error reply type %x", replyType)
		}

		if dataLen > 0 {
			// offset-data chunk: first 8 bytes are the offset, rest is data
			if dataLen > 8 {
				var chunkOffset uint64
				if err := binary.Read(c.conn, binary.BigEndian, &chunkOffset); err != nil {
					return fmt.Errorf("reading chunk offset: %s", err)
				}
				chunkLen := int(dataLen - 8)
				position := int(chunkOffset - requestOffset)
				if position < 0 || position+chunkLen > len(buf) {
					discard := make([]byte, chunkLen)
					io.ReadFull(c.conn, discard)
					return fmt.Errorf("chunk offset %d out of range for request at %d length %d", chunkOffset, requestOffset, len(buf))
				}
				if _, err := io.ReadFull(c.conn, buf[position:position+chunkLen]); err != nil {
					return err
				}
				totalRead += chunkLen
			} else {
				discard := make([]byte, dataLen)
				io.ReadFull(c.conn, discard)
			}
		}

		if flags&nbdReplyFlagDone != 0 {
			break
		}
	}

	return nil
}

// BlockStatus queries the block status (dirty bitmap) for a range.
// Returns a slice of extents describing which regions are dirty (data) or clean (hole).
func (c *NBDClient) BlockStatus(offset uint64, length uint32) ([]NBDExtent, error) {
	_, err := c.sendCommand(nbdCmdBlockStatus, nbdCmdFlagReqOne, offset, length)
	if err != nil {
		return nil, fmt.Errorf("sending BLOCK_STATUS command: %s", err)
	}

	return c.readBlockStatusReply()
}

func (c *NBDClient) readBlockStatusReply() ([]NBDExtent, error) {
	var extents []NBDExtent

	for {
		var magic uint32
		if err := binary.Read(c.conn, binary.BigEndian, &magic); err != nil {
			return nil, err
		}

		if magic == nbdSimpleReplyMagic {
			// simple reply = error only, no data
			var errCode uint32
			var handle uint64
			if err := binary.Read(c.conn, binary.BigEndian, &errCode); err != nil {
				return nil, fmt.Errorf("reading block status error code: %s", err)
			}
			if err := binary.Read(c.conn, binary.BigEndian, &handle); err != nil {
				return nil, fmt.Errorf("reading block status handle: %s", err)
			}
			if errCode != 0 {
				return nil, fmt.Errorf("NBD block status error: %d", errCode)
			}
			break
		}

		if magic != nbdStructuredReplyMagic {
			return nil, fmt.Errorf("bad reply magic: %x", magic)
		}

		var flags uint16
		var replyType uint16
		var handle uint64
		var dataLen uint32

		if err := binary.Read(c.conn, binary.BigEndian, &flags); err != nil {
			return nil, fmt.Errorf("reading block status flags: %s", err)
		}
		if err := binary.Read(c.conn, binary.BigEndian, &replyType); err != nil {
			return nil, fmt.Errorf("reading block status reply type: %s", err)
		}
		if err := binary.Read(c.conn, binary.BigEndian, &handle); err != nil {
			return nil, fmt.Errorf("reading block status handle: %s", err)
		}
		if err := binary.Read(c.conn, binary.BigEndian, &dataLen); err != nil {
			return nil, fmt.Errorf("reading block status data length: %s", err)
		}

		if replyType == nbdReplyTypeBlockStatus && dataLen >= 4 {
			// first 4 bytes: context ID
			var contextID uint32
			if err := binary.Read(c.conn, binary.BigEndian, &contextID); err != nil {
				return nil, fmt.Errorf("reading block status context ID: %s", err)
			}
			remaining := dataLen - 4

			// each extent is 8 bytes: uint32 length + uint32 flags
			for remaining >= 8 {
				var ext NBDExtent
				if err := binary.Read(c.conn, binary.BigEndian, &ext.Length); err != nil {
					return nil, fmt.Errorf("reading extent length: %s", err)
				}
				if err := binary.Read(c.conn, binary.BigEndian, &ext.Flags); err != nil {
					return nil, fmt.Errorf("reading extent flags: %s", err)
				}
				extents = append(extents, ext)
				remaining -= 8
			}
			if remaining > 0 {
				discard := make([]byte, remaining)
				io.ReadFull(c.conn, discard)
			}
		} else {
			// skip unknown reply
			if dataLen > 0 {
				discard := make([]byte, dataLen)
				io.ReadFull(c.conn, discard)
			}
		}

		if flags&nbdReplyFlagDone != 0 {
			break
		}
	}

	return extents, nil
}

// Close sends NBD_CMD_DISC and closes the connection
func (c *NBDClient) Close() error {
	c.sendCommand(nbdCmdDisc, 0, 0, 0)
	return c.conn.Close()
}
