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
	connectCB func(c ssh.ConnMetadata) (*sshServerClient, error)
	closeCB   func(c ssh.ConnMetadata) error
	log       *Log
}

// ClientHandleChannelOpen is called when the client (= the VM) asks
// for a new channel (ex: forwarded-tcpip)
func (proxy *SSHProxy) ClientHandleChannelOpen(chanType string, client *sshServerClient, destConn ssh.Conn) {
	channels := client.sshClient.HandleChannelOpen(chanType)
	if channels == nil {
		proxy.log.Warningf("HandleChannelOpen failed for '%s' channels", chanType)
		return
	}
	go proxy.runChannels(channels, destConn, client)
}

// ForwardRequestsToClient forwards server ("outside") global requests to the client ("VM")
func (proxy *SSHProxy) ForwardRequestsToClient(in <-chan *ssh.Request, client *ssh.Client) {
	for req := range in {
		proxy.log.Tracef("ForwardRequests: %s %t", req.Type, req.WantReply)
		respStatus, respPayload, err := client.SendRequest(req.Type, req.WantReply, req.Payload)
		if err != nil {
			proxy.log.Tracef("ForwardRequests failed: %s", err)
			if req.WantReply {
				req.Reply(false, nil)
			}
		} else {
			if req.WantReply {
				req.Reply(respStatus, respPayload)
			}
		}
	}
}

func sshProxyCopyChan(dst ssh.Channel, src ssh.Channel, way string, wgChannels *sync.WaitGroup, wgClosed *sync.WaitGroup, log *Log) {
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

	// -- Wait for the other copy and exit-status request writing.
	// Notes:
	// 	- if the request is not written, the ssh/scp/… client will return its own error code
	//  - interestingly, higher latencies seems to mitigate the issue
	c := make(chan struct{})
	go func() {
		defer close(c)
		wgClosed.Wait()
	}()
	select {
	case <-c:
	case <-time.After(1 * time.Second):
		// this is not supposed to happen, but let's be paranoid
		log.Warningf("SSH: %s timeout waiting for other chan (possible goroutine leak)", way)
	}

	// -- Close channels for real
	src.Close()
	dst.Close()
	log.Tracef("SSH: %s finished", way)

	wgChannels.Done()
}

// Send sparses keepalives to detect dead connections, a failed SendRequest
// will set channels to nil, closing the connections (and we use a timeout as well)
// In case of failure, we close both connections (up & down)
func (proxy *SSHProxy) scheduleSSHKeepAlives(sshConn ssh.Conn, name string) {
	t := time.NewTicker(1 * time.Minute)
	defer t.Stop()
	for range t.C {
		proxy.log.Tracef("send SSH keepalive (%s, %s)", name, sshConn.RemoteAddr())
		err := SSHSendKeepAlive(sshConn, 10*time.Second)
		if err != nil {
			proxy.log.Tracef("ssh (%s) keepalive error: %s, closing", name, err)
			sshConn.Close()
			proxy.Close() // hardcore§§!!
			return
		}
	}
}

// runChannels is the the core of the ssh-proxy, where we manage channels and requests
// the usual destination is the VM, but see ClientHandleChannelOpen() for special cases
func (proxy *SSHProxy) runChannels(chans <-chan ssh.NewChannel, destConn ssh.Conn, clientInfo *sshServerClient) error {
	var wgChannels sync.WaitGroup

	for newChannel := range chans {

		proxy.log.Tracef("SSH: newChannel.ChannelType() = %s", newChannel.ChannelType())

		// up = proxy to internal VM (for usual channels)
		upChannel, upRequests, err := destConn.OpenChannel(newChannel.ChannelType(), newChannel.ExtraData())
		if err != nil {
			if newChannel.ChannelType() == "session" {
				// no session is fatal
				return fmt.Errorf("OpenChannel: %s", err)
			}

			proxy.log.Errorf("OpenChannel: %s", err)
			newChannel.Reject(2, err.Error()) // 2 = SSH_OPEN_CONNECT_FAILED
			continue
		}

		// down = external client to mulch ssh proxy (for usual channels)
		downChannel, downRequests, err := newChannel.Accept()
		if err != nil {
			return fmt.Errorf("failed Accept: %s", err)
		}

		if newChannel.ChannelType() == "auth-agent@openssh.com" || newChannel.ChannelType() == "agent-connect" {
			proxy.newSshAgentProxy(upChannel, downChannel, clientInfo)
			continue
		}

		// requests + two Copy
		wgChannels.Add(3)

		var wgClosed sync.WaitGroup
		wgClosed.Add(1)

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
					wgClosed.Done()
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

		go sshProxyCopyChan(upChannel, downChannel, "down->up", &wgChannels, &wgClosed, proxy.log)
		go sshProxyCopyChan(downChannel, upChannel, "up->down", &wgChannels, &wgClosed, proxy.log)
	}

	// Wait io.Copies (we have defered Closes in this function)
	wgChannels.Wait()
	return nil
}

func (proxy *SSHProxy) serveProxy() error {
	serverConn, chans, reqs, err := ssh.NewServerConn(proxy, proxy.config)
	if err != nil {
		return err
	}
	defer serverConn.Close()

	client, err := proxy.connectCB(serverConn)
	if err != nil {
		return err
	}
	clientConn := client.sshClient

	defer clientConn.Close()

	// send keepalives on both sides
	// TODO: defer-kill theses (currently, we have "send keepalive failed
	// disconnected by user" errors a few minutes after disconnection)
	go proxy.scheduleSSHKeepAlives(serverConn, "outside")
	go proxy.scheduleSSHKeepAlives(clientConn, "inside")

	// global requests (from outside to the VM)
	go proxy.ForwardRequestsToClient(reqs, clientConn)

	// special channels (requested by the VM to the outside):

	//  -- "ssh -R" tunnels from the outside
	proxy.ClientHandleChannelOpen("forwarded-tcpip", client, serverConn)

	// -- X11 forwarding
	proxy.ClientHandleChannelOpen("x11", client, serverConn)

	// -- agent forwarding (old and new names)
	proxy.ClientHandleChannelOpen("auth-agent@openssh.com", client, serverConn)
	proxy.ClientHandleChannelOpen("agent-connect", client, serverConn)

	err = proxy.runChannels(chans, clientConn, client)
	if err != nil {
		return err
	}

	if proxy.closeCB != nil {
		proxy.closeCB(serverConn)
	}
	return nil
}

// run a SSH agent server as filter between the client and the real agent
func (proxy *SSHProxy) newSshAgentProxy(realAgent ssh.Channel, client ssh.Channel, clientInfo *sshServerClient) {
	agent := NewSSHProxyAgent(realAgent, client, clientInfo.vm, clientInfo.apiKey, proxy.log)
	go func() {
		agent.Serve()
		proxy.log.Tracef("SSH internal agent closed")

		realAgent.Close()
		client.Close()
	}()
}

// ListenAndServeProxy of our own SSH server
func ListenAndServeProxy(
	addr string,
	serverConfig *ssh.ServerConfig,
	sshClients *sshServerClients,
	log *Log,
	connectCB func(c ssh.ConnMetadata) (*sshServerClient, error),
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
					log.Tracef("SSH: proxy serving error: %s", err)
					return
				}

				log.Tracef("SSH: connection closed (%s)", connListener.RemoteAddr())
			}()
		}
	}()

	return nil
}
