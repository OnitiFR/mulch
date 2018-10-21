package common

// WriteCounterCallback is a user callback, called by WriteCounter during
// writing process
type WriteCounterCallback func(current uint64, total uint64)

// WriteCounter counts the number of bytes written to it. It implements to the io.Writer
// interface and we can pass this into io.TeeReader() which will report progress on each
// write cycle.
type WriteCounter struct {
	Total        uint64
	CB           WriteCounterCallback
	Step         uint64
	previousStep uint64
	current      uint64
}

func (wc *WriteCounter) Write(p []byte) (int, error) {
	n := len(p)
	wc.current += uint64(n)

	if wc.current > wc.previousStep+wc.Step {
		wc.CB(wc.current, wc.Total)
		wc.previousStep = wc.current
	}

	return n, nil
}
