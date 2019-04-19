package server

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

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

func sshProxyCopyChan(dst ssh.Channel, src ssh.Channel, way string, wgChannels *sync.WaitGroup, log *Log) {
	_, err := io.Copy(dst, src)
	if err != nil {
		log.Tracef("SSH: %s Copy error: %s", way, err)
	}

	// -- Cheap "relaxing" of channels
	// I don't see better option, since EOF/Close messages from
	// remotes are hidden by x/crypto/ssh API.
	src.CloseWrite()
	dst.CloseWrite()
	log.Tracef("SSH: %s EOF sent", way)

	// This is it: we wait for the other copy and last requests…
	// If connection latency is greater, the "exit-status" request
	// is missed, and the ssh/scp/… client will return its own error code.
	time.Sleep(500 * time.Millisecond)

	src.Close()
	dst.Close()
	log.Tracef("SSH: %s finished", way)

	wgChannels.Done()
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

	// discard global requests, we won't manage it anyway
	go ssh.DiscardRequests(reqs)

	var wgChannels sync.WaitGroup

	for newChannel := range chans {

		proxy.log.Tracef("SSH: newChannel.ChannelType() = %s", newChannel.ChannelType())

		// up = internal VM
		upChannel, upRequests, err := clientConn.OpenChannel(newChannel.ChannelType(), newChannel.ExtraData())
		if err != nil {
			if newChannel.ChannelType() == "session" {
				// no session is fatal
				return fmt.Errorf("OpenChannel: %s", err)
			}

			proxy.log.Errorf("OpenChannel: %s", err)
			newChannel.Reject(2, err.Error()) // 2 = SSH_OPEN_CONNECT_FAILED
			continue
		}

		// down = external client to mulch ssh proxy
		downChannel, downRequests, err := newChannel.Accept()
		if err != nil {
			return fmt.Errorf("Accept: %s", err)
		}

		// requests + two Copy
		wgChannels.Add(3)

		// connect requests
		go func() {
			proxy.log.Trace("SSH: waiting for request")

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
					proxy.log.Trace("SSH: req is nil, both chan closed")
					wgChannels.Done()
					break
				}

				proxy.log.Tracef("SSH: request: %s %t %s", req.Type, req.WantReply, chn)
				// proxy.log.Tracef("SSH payload: -%s-", req.Payload)

				b, errS := dst.SendRequest(req.Type, req.WantReply, req.Payload)
				if errS != nil {
					proxy.log.Errorf("SSH: SendRequest error: %s", errS)
				}

				if req.WantReply {
					req.Reply(b, nil)
				}
			}
		}()

		proxy.log.Trace("SSH: Connecting channels")

		go sshProxyCopyChan(upChannel, downChannel, "down->up", &wgChannels, proxy.log)
		go sshProxyCopyChan(downChannel, upChannel, "up->down", &wgChannels, proxy.log)
	}

	// Wait io.Copies (we have defered Closes in this function)
	wgChannels.Wait()

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
					log.Errorf("SSH: proxy serving error: %s", err)
					return
				}

				log.Trace("SSH: connection closed")
			}()
		}
	}()

	return nil
}
