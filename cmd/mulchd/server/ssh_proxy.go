package server

import (
	"io"
	"net"
	"sync"

	"golang.org/x/crypto/ssh"
)

// SSHProxy is a proxy between two SSH connections
type SSHProxy struct {
	net.Conn
	config    *ssh.ServerConfig
	connectCB func(c ssh.ConnMetadata) (*ssh.Client, error)
	closeCB   func(c ssh.ConnMetadata) error
	log       *Log
}

func (proxy *SSHProxy) serveProxy() error {
	serverConn, chans, reqs, err := ssh.NewServerConn(proxy, proxy.config)
	if err != nil {
		return err
	}
	defer serverConn.Close()

	clientConn, err := proxy.connectCB(serverConn)
	if err != nil {
		return err
	}
	defer clientConn.Close()

	go ssh.DiscardRequests(reqs)

	var wg sync.WaitGroup

	for newChannel := range chans {
		proxy.log.Tracef("SSH newChannel.ChannelType() = %s", newChannel.ChannelType())

		upChannel, upRequests, err := clientConn.OpenChannel(newChannel.ChannelType(), newChannel.ExtraData())
		if err != nil {
			return err
		}

		downChannel, downRequests, err := newChannel.Accept()
		if err != nil {
			return err
		}

		wg.Add(2)

		// connect requests
		go func() {
			proxy.log.Trace("SSH waiting for request")

			for {
				var req *ssh.Request
				var dst ssh.Channel
				var chn string

				select {
				case req = <-upRequests:
					dst = downChannel
					chn = "from up to down (dst=down)"
				case req = <-downRequests:
					dst = upChannel
					chn = "from down to up (dst=up)"
				}

				if req == nil {
					proxy.log.Trace("SSH: req is nil, both chan closed!")
					// continue
					break
				}

				proxy.log.Tracef("SSH request: %s %t %s", req.Type, req.WantReply, chn)
				// proxy.log.Tracef("SSH payload: -%s-", req.Payload)

				b, errS := dst.SendRequest(req.Type, req.WantReply, req.Payload)
				if errS != nil {
					proxy.log.Errorf("SSH: SendRequest error: %s", errS)
				}

				if req.WantReply {
					req.Reply(b, nil)
				}
			}
			proxy.log.Trace("SSH: goroutine quits")
		}()

		proxy.log.Trace("SSH: Connecting channels")

		go func() {
			io.Copy(upChannel, downChannel)
			upChannel.Close()
			downChannel.Close()
			proxy.log.Trace("down->up finished")
			wg.Done()
		}()
		go func() {
			io.Copy(downChannel, upChannel)
			upChannel.Close()
			downChannel.Close()
			proxy.log.Trace("up->down finished")
			wg.Done()
		}()

	}
	// Wait io.Copies (we have defered Closes in this function)
	wg.Wait()

	if proxy.closeCB != nil {
		proxy.closeCB(serverConn)
	}

	return nil
}

// ListenAndServeProxy of our own SSH server
func ListenAndServeProxy(
	addr string,
	serverConfig *ssh.ServerConfig,
	log *Log,
	connectCB func(c ssh.ConnMetadata) (*ssh.Client, error),
	closeCB func(c ssh.ConnMetadata) error,
) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	go func() {
		defer listener.Close()
		for {
			connListener, err := listener.Accept()
			log.Tracef("SSH connection from %s", connListener.RemoteAddr())

			if err != nil {
				log.Error(err.Error())
				return
			}

			sshconn := &SSHProxy{
				Conn:      connListener,
				config:    serverConfig,
				connectCB: connectCB,
				closeCB:   closeCB,
				log:       log,
			}

			go func() {
				if err := sshconn.serveProxy(); err != nil {
					log.Errorf("SSH proxy serving error: %s", err)
					return
				}

				log.Trace("SSH connection closed")
			}()
		}
	}()

	return nil
}
