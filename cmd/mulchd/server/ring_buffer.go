package server

import "github.com/smallnest/ringbuffer"

// OverflowBuffer is a ring buffer that will overflow when full
type OverflowBuffer struct {
	rb *ringbuffer.RingBuffer
}

// NewOverflowBuffer creates a new OverflowBuffer
func NewOverflowBuffer(size int) *OverflowBuffer {
	return &OverflowBuffer{
		rb: ringbuffer.New(size),
	}
}

// Write writes data to the buffer (non blocking [overwrites])
func (ob *OverflowBuffer) Write(data []byte) (n int, err error) {
	if ob.rb.Free() < len(data) {
		// TODO: we should really *drop* data :( (but it would require a custom ringbuffer)
		trash := make([]byte, len(data)-ob.rb.Free())
		ob.rb.Read(trash)
	}

	return ob.rb.Write(data)
}

// Read reads data from the buffer
func (ob *OverflowBuffer) Read(data []byte) (n int, err error) {
	return ob.rb.Read(data)
}

// IsEmpty returns true if the buffer is empty
func (ob *OverflowBuffer) IsEmpty() bool {
	return ob.rb.IsEmpty()
}
