package common

import "io"

// FakeWriteCloser allows to create a WriteCloser with a
// fake Close() from a Writer
type FakeWriteCloser struct {
	io.Writer
}

// Close is a no-op method
func (fake *FakeWriteCloser) Close() error {
	return nil
}
