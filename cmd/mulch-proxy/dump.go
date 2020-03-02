package main

import (
	"fmt"
	"io"
	"runtime"
)

// from https://golang.org/src/runtime/pprof/pprof.go
func writeGoroutineStacks(w io.Writer) error {
	fmt.Fprintf(w, "-- Goroutines:\n")

	// We don't know how big the buffer needs to be to collect
	// all the goroutines. Start with 1 MB and try a few times, doubling each time.
	// Give up and use a truncated trace if 64 MB is not enough.
	buf := make([]byte, 1<<20)
	for i := 0; ; i++ {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		if len(buf) >= 64<<20 {
			// Filled 64 MB - stop there.
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	_, err := w.Write(buf)
	return err
}

// InstallSIGQUIT will react to SIGQUIT signal and dump all goroutines
// without exiting.
// kill -QUIT $(pidof mulch-proxy)
func InstallSIGQUIT() {

}
