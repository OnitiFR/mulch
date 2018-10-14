package server

import (
	"io"
	"log"
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
}

func (p *SSHProxy) serveProxy() error {
	serverConn, chans, reqs, err := ssh.NewServerConn(p, p.config)
	if err != nil {
		log.Println("failed to handshake")
		return (err)
	}
	defer serverConn.Close()

	clientConn, err := p.connectCB(serverConn)
	if err != nil {
		log.Printf("connectCB: %s", err.Error())
		return (err)
	}
	defer clientConn.Close()

	go ssh.DiscardRequests(reqs)

	var wg sync.WaitGroup

	for newChannel := range chans {

		log.Printf("newChannel.ChannelType() = %s\n", newChannel.ChannelType())

		upChannel, upRequests, err := clientConn.OpenChannel(newChannel.ChannelType(), newChannel.ExtraData())
		if err != nil {
			log.Printf("Could not accept client channel: %s", err.Error())
			return err
		}

		downChannel, downRequests, err := newChannel.Accept()
		if err != nil {
			log.Printf("Could not accept server channel: %s", err.Error())
			return err
		}

		wg.Add(2)

		// connect requests
		go func() {
			log.Printf("Waiting for request")

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
					log.Println("req is nil, both chan closed!")
					// continue
					break
				}

				log.Printf("Request: %s %t %s", req.Type, req.WantReply, chn)
				// log.Printf("Payload: -%s-", req.Payload)

				b, errS := dst.SendRequest(req.Type, req.WantReply, req.Payload)
				if errS != nil {
					log.Printf("SendRequest error: %s", errS)
				}

				if req.WantReply {
					req.Reply(b, nil)
				}
			}
			log.Println("goroutine quits")
		}()

		// connect channels
		log.Printf("Connecting channels.")

		go func() {
			io.Copy(upChannel, downChannel)
			upChannel.Close()
			downChannel.Close()
			log.Println("down->up finished")
			wg.Done()
		}()
		go func() {
			io.Copy(downChannel, upChannel)
			upChannel.Close()
			downChannel.Close()
			log.Println("up->down finished")
			wg.Done()
		}()

	}
	// Wait io.Copies (we have defered Closes in this function)
	wg.Wait()

	if p.closeCB != nil {
		p.closeCB(serverConn)
	}

	return nil
}

// ListenAndServeProxy of our own SSH server
func ListenAndServeProxy(addr string, serverConfig *ssh.ServerConfig,
	connectCB func(c ssh.ConnMetadata) (*ssh.Client, error),
	closeCB func(c ssh.ConnMetadata) error,
) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("net.Listen failed: %v\n", err)
		return err
	}

	defer listener.Close()

	for {
		connListener, err := listener.Accept()
		if err != nil {
			log.Printf("listen.Accept failed: %v\n", err)
			return err
		}

		sshconn := &SSHProxy{
			Conn:      connListener,
			config:    serverConfig,
			connectCB: connectCB,
			closeCB:   closeCB,
		}

		go func() {
			if err := sshconn.serveProxy(); err != nil {
				log.Printf("Error occured while serving: %s\n", err)
				return
			}

			log.Println("Connection closed.")
		}()
	}
}
