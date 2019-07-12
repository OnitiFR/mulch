package server

import (
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"syscall"
	"time"
)

// ProxyReloader is able to reload Mulch Reverse Proxy using a system signal
type ProxyReloader struct {
	app *App
	c   chan bool
}

// NewProxyReloader creates a new ProxyReloader instance
func NewProxyReloader(app *App) *ProxyReloader {
	c := make(chan bool, 1)
	c <- true
	return &ProxyReloader{
		c:   c,
		app: app,
	}
}

// Request a Reverse Proxy reload, if not already requested.
// The request is delayed in order to "mutualize" multiple requests in a short
// amount of time.
func (pr *ProxyReloader) Request() {
	go func() {
		select {
		case <-pr.c:
		default:
			pr.app.Log.Trace("ProxyReloader request already scheduled")
			return
		}

		time.Sleep(1 * time.Second)
		pr.sendProxyReloadSignal()

		pr.c <- true
	}()
}

func (pr *ProxyReloader) sendProxyReloadSignal() {
	app := pr.app

	lastPidFilename := path.Clean(app.Config.DataPath + "/mulch-proxy-last.pid")
	data, err := ioutil.ReadFile(lastPidFilename)
	if err != nil {
		app.Log.Errorf("reloading mulch-proxy config: %s", err)
		return
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		app.Log.Errorf("reloading mulch-proxy config: pid '%s': %s", data, err)
		return
	}

	p, err := os.FindProcess(pid)
	if err != nil {
		app.Log.Errorf("reloading mulch-proxy config: process: %s", err)
		return
	}

	err = p.Signal(syscall.SIGHUP)
	if err != nil {
		app.Log.Errorf("reloading mulch-proxy config: signal: %s", err)
		return
	}
	app.Log.Info("HUP signal sent to mulch-proxy")
}
