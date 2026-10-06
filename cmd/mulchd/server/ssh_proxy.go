package server

import (
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHProxy is a proxy between two SSH connections
type SSHProxy struct {
	net.Conn
	config *ssh.ServerConfig
	app    *App
}

// ClientHandleChannelOpen is called when the client (= the VM) asks
// for a new channel (ex: forwarded-tcpip)
func (proxy *SSHProxy) ClientHandleChannelOpen(chanType string, client *sshServerClient, destConn ssh.Conn) {
	channels := client.sshClient.HandleChannelOpen(chanType)
	if channels == nil {
		proxy.app.Log.Warningf("HandleChannelOpen failed for '%s' channels", chanType)
		return
	}
	go proxy.runChannels(channels, destConn, client)
}

// ForwardRequestsToClient forwards server ("outside") global requests to the client ("VM")
func (proxy *SSHProxy) ForwardRequestsToClient(in <-chan *ssh.Request, client *ssh.Client) {
	for req := range in {
		proxy.app.Log.Tracef("ForwardRequests: %s %t", req.Type, req.WantReply)
		respStatus, respPayload, err := client.SendRequest(req.Type, req.WantReply, req.Payload)
		if err != nil {
			proxy.app.Log.Tracef("ForwardRequests failed: %s", err)
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

// sshProxyCopyStreams copies data and extended data (stderr) from src to dst,
// then forwards src EOF to dst (and only to dst: the other way may still be
// active, ex: "ssh vm cmd < /dev/null" must still get cmd output)
func sshProxyCopyStreams(dst ssh.Channel, src ssh.Channel, way string, log *Log) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// unread extended data would also eat the channel window
		_, err := io.Copy(dst.Stderr(), src.Stderr())
		if err != nil {
			log.Tracef("SSH: %s stderr Copy error: %s", way, err)
		}
	}()

	_, err := io.Copy(dst, src)
	if err != nil {
		log.Tracef("SSH: %s Copy error: %s", way, err)
	}

	// CloseWrite prevents any further (extended) write, so wait for stderr
	wg.Wait()
	dst.CloseWrite()
	log.Tracef("SSH: %s EOF sent", way)
}

// sshProxyForwardRequest forwards a channel request (exit-status, pty-req, …)
func sshProxyForwardRequest(req *ssh.Request, dst ssh.Channel, way string, log *Log) {
	log.Tracef("SSH: request: %s %t %s", req.Type, req.WantReply, way)

	b, err := dst.SendRequest(req.Type, req.WantReply, req.Payload)
	if err != nil {
		log.Errorf("SSH: SendRequest error: %s", err)
	}

	if req.WantReply {
		req.Reply(b, nil)
	}
}

// runChannelPair connects two channels until both are closed
//
// x/crypto/ssh hides EOF/Close messages from remotes, but:
//   - Read() returns io.EOF on both EOF and Close (after buffered data is drained)
//   - the requests chan is closed on Close only
//
// So EOF is forwarded way by way when a copy ends, and a Close from one side
// is forwarded to the other side once all its data has been delivered.
func runChannelPair(upChannel ssh.Channel, upRequests <-chan *ssh.Request, downChannel ssh.Channel, downRequests <-chan *ssh.Request, log *Log) {
	upDrained := make(chan struct{})
	downDrained := make(chan struct{})

	go func() {
		sshProxyCopyStreams(downChannel, upChannel, "up->down", log)
		close(upDrained)
	}()
	go func() {
		sshProxyCopyStreams(upChannel, downChannel, "down->up", log)
		close(downDrained)
	}()

	// requests are forwarded in order, so exit-status is sent before Close
	for upRequests != nil || downRequests != nil {
		select {
		case req, ok := <-upRequests:
			if !ok {
				log.Trace("SSH: up channel closed")
				upRequests = nil
				go func() {
					<-upDrained
					downChannel.Close()
				}()
				continue
			}
			sshProxyForwardRequest(req, downChannel, "from up to down (dst=down)", log)
		case req, ok := <-downRequests:
			if !ok {
				log.Trace("SSH: down channel closed")
				downRequests = nil
				go func() {
					<-downDrained
					upChannel.Close()
				}()
				continue
			}
			sshProxyForwardRequest(req, upChannel, "from down to up (dst=up)", log)
		}
	}

	<-upDrained
	<-downDrained
	log.Trace("SSH: channel pair finished")
}

// Send sparses keepalives to detect dead connections, a failed SendRequest
// will set channels to nil, closing the connections (and we use a timeout as well)
// In case of failure, we close both connections (up & down)
func (proxy *SSHProxy) scheduleSSHKeepAlives(sshConn ssh.Conn, name string) {
	t := time.NewTicker(1 * time.Minute)
	defer t.Stop()
	for range t.C {
		proxy.app.Log.Tracef("send SSH keepalive (%s, %s)", name, sshConn.RemoteAddr())
		err := SSHSendKeepAlive(sshConn, 10*time.Second)
		if err != nil {
			proxy.app.Log.Tracef("ssh (%s) keepalive error: %s, closing", name, err)
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

		proxy.app.Log.Tracef("SSH: newChannel.ChannelType() = %s", newChannel.ChannelType())

		// up = proxy to internal VM (for usual channels)
		upChannel, upRequests, err := destConn.OpenChannel(newChannel.ChannelType(), newChannel.ExtraData())
		if err != nil {
			if newChannel.ChannelType() == "session" {
				// no session is fatal
				return fmt.Errorf("OpenChannel: %s", err)
			}

			proxy.app.Log.Errorf("OpenChannel: %s", err)
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

		proxy.app.Log.Trace("SSH: Connecting channels")

		wgChannels.Add(1)
		go func() {
			runChannelPair(upChannel, upRequests, downChannel, downRequests, proxy.app.Log)
			wgChannels.Done()
		}()
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

	vmName := serverConn.Permissions.Extensions["mulch-vmName"]
	user := serverConn.Permissions.Extensions["mulch-user"]
	apiKeyComment := serverConn.Permissions.Extensions["mulch-apiKeyComment"]

	var vm *VM
	var errV error

	if strings.Contains(vmName, "-") {
		parts := strings.Split(vmName, "-")
		if len(parts) != 2 {
			return fmt.Errorf("wrong VM-revision name '%s'", vmName)
		}

		name := parts[0]
		revStr := parts[1]

		// we accept vm-123 (old) and vm-r123 (new) formats
		if revStr[0] == 'r' {
			revStr = revStr[1:]
		}

		revision, errA := strconv.Atoi(revStr)
		if errA != nil {
			return errA
		}
		vmName := NewVMName(name, revision)
		vm, errV = proxy.app.VMDB.GetByName(vmName)
		if errV != nil {
			// also search in the greenhouse (building VMs)
			entry, errG := proxy.app.VMDB.GetGreenhouseEntryByName(vmName)
			if errG != nil {
				return errV // return original error
			}
			vm = entry.VM
		}
	} else {
		vm, errV = proxy.app.VMDB.GetActiveByName(vmName)
		if errV != nil {
			return errV
		}
	}

	destAuth, errP := proxy.app.SSHPairDB.GetPublicKeyAuth(vm.MulchSuperUserSSHKey)
	if errP != nil {
		return errP
	}

	// for pubkeys, will be nil (and it's ok)
	apiKey := proxy.app.APIKeysDB.GetByComment(apiKeyComment)

	var client sshServerClient
	client.vm = vm
	client.sshUser = user
	client.apiKeyComment = apiKeyComment
	client.apiKey = apiKey
	client.startTime = time.Now()
	client.remoteAddr = proxy.RemoteAddr()

	clientConfig := &ssh.ClientConfig{}
	clientConfig.User = user
	clientConfig.Auth = []ssh.AuthMethod{
		destAuth,
	}
	clientConfig.HostKeyCallback = ssh.InsecureIgnoreHostKey()

	// connect to the destination VM
	proxy.app.Log.Tracef("SSH Proxy: dial %s@%s", user, vm.LastIP)
	clientConn, errD := ssh.Dial("tcp", vm.LastIP+":22", clientConfig)
	if errD != nil {
		return errD
	}

	client.sshClient = clientConn

	proxy.app.sshClients.add(proxy.RemoteAddr(), &client)
	proxy.app.Log.Tracef("SSH proxy: connection accepted from %s forwarded to %s", proxy.RemoteAddr(), client.sshClient.RemoteAddr())

	// --

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
	proxy.ClientHandleChannelOpen("forwarded-tcpip", &client, serverConn)

	// -- X11 forwarding
	proxy.ClientHandleChannelOpen("x11", &client, serverConn)

	// -- agent forwarding (old and new names)
	proxy.ClientHandleChannelOpen("auth-agent@openssh.com", &client, serverConn)
	proxy.ClientHandleChannelOpen("agent-connect", &client, serverConn)

	err = proxy.runChannels(chans, clientConn, &client)
	if err != nil {
		return err
	}

	proxy.app.sshClients.delete(proxy.RemoteAddr())
	proxy.app.Log.Tracef("SSH proxy: connection closed from: %s", proxy.RemoteAddr())

	return nil
}

// run a SSH agent server as filter between the client and the real agent
func (proxy *SSHProxy) newSshAgentProxy(realAgent ssh.Channel, client ssh.Channel, clientInfo *sshServerClient) {
	agent := NewSSHProxyAgent(realAgent, client, clientInfo.vm, clientInfo.apiKey, proxy.app.Log)
	go func() {
		agent.Serve()
		proxy.app.Log.Tracef("SSH internal agent closed")

		realAgent.Close()
		client.Close()
	}()
}

// ListenAndServeProxy of our own SSH server
func ListenAndServeProxy(
	addr string,
	serverConfig *ssh.ServerConfig,
	sshClients *sshServerClients,
	app *App,
) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	go func() {
		defer listener.Close()
		for {
			connListener, err := listener.Accept()
			app.Log.Tracef("SSH connection from %s", connListener.RemoteAddr())

			if err != nil {
				app.Log.Error(err.Error())
				return
			}

			sshconn := &SSHProxy{
				Conn:   connListener,
				config: serverConfig,
				app:    app,
			}

			go func() {
				if err := sshconn.serveProxy(); err != nil {
					app.Log.Tracef("SSH: proxy serving error: %s", err)
					return
				}

				app.Log.Tracef("SSH: connection closed (%s)", connListener.RemoteAddr())
			}()
		}
	}()

	return nil
}
