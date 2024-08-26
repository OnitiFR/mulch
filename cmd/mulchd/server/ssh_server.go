package server

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

type sshServerClient struct {
	sshClient     *ssh.Client
	remoteAddr    net.Addr
	vm            *VM
	sshUser       string
	apiKeyComment string
	apiKey        *APIKey
	startTime     time.Time
}

type sshServerClients struct {
	db    map[net.Addr]*sshServerClient
	mutex sync.Mutex
}

// NewSSHProxyServer creates and starts our SSH proxy to VMs
func NewSSHProxyServer(app *App) error {

	ourPair := app.SSHPairDB.GetByName(SSHProxyPair)
	if ourPair == nil {
		return fmt.Errorf("cannot find %s SSH key pair", SSHProxyPair)
	}

	ourPrivate, err := ssh.ParsePrivateKey([]byte(ourPair.Private))
	if err != nil {
		return err
	}

	app.sshClients = newSSHServerClients()

	config := &ssh.ServerConfig{
		PublicKeyCallback: func(c ssh.ConnMetadata, pubKey ssh.PublicKey) (*ssh.Permissions, error) {

			user := ""
			vmName := ""
			var client sshServerClient

			apiKey, errG := app.APIKeysDB.GetByPubKey(string(pubKey.Marshal()))
			if errG != nil {
				return nil, errG
			}

			if apiKey != nil {
				// API key access
				parts := strings.Split(c.User(), "@")
				if len(parts) != 2 {
					return nil, fmt.Errorf("wrong user format '%s' (user@vm needed)", c.User())
				}

				if !apiKey.IsAllowed("GET", "/sshpair", nil) {
					return nil, errors.New("permission denied (rights)")
				}

				user = parts[0]
				vmName = parts[1]
				client.apiKeyComment = apiKey.Comment
				client.apiKey = apiKey
				app.Log.Tracef("SSH Proxy: %s (API key '%s') %s@%s", c.RemoteAddr(), apiKey.Comment, user, vmName)
			} else {
				matchingPubKey, comment, errS := SearchSSHAuthorizedKey(pubKey, app.Config.ProxySSHExtraKeysFile)
				if errS != nil {
					return nil, errS
				}
				// Extra public key access
				if matchingPubKey != nil {
					client.apiKeyComment = "[pubKey] " + comment
					parts := strings.Split(comment, "@")
					if len(parts) != 2 {
						return nil, fmt.Errorf("wrong user format '%s' (user@vm needed)", c.User())
					}

					user = parts[0]
					vmName = parts[1]

					// if the user is authorized to connect any VM, use the
					// username as the VM name:
					if vmName == "*" {
						vmName = c.User()
					}

					app.Log.Tracef("SSH Proxy: %s (proxy_ssh_extra_keys_file) %s@%s", c.RemoteAddr(), user, vmName)
				}
			}

			if user == "" || vmName == "" {
				return nil, fmt.Errorf("no allowed public key found (%s)", c.RemoteAddr())
			}

			var vm *VM

			if strings.Contains(vmName, "-") {
				parts := strings.Split(vmName, "-")
				if len(parts) != 2 {
					return nil, fmt.Errorf("wrong VM-revision name '%s'", vmName)
				}

				name := parts[0]
				revStr := parts[1]

				// we accept vm-123 (old) and vm-r123 (new) formats
				if revStr[0] == 'r' {
					revStr = revStr[1:]
				}
				revision, errA := strconv.Atoi(revStr)
				if errA != nil {
					return nil, errA
				}
				vm, errG = app.VMDB.GetByName(NewVMName(name, revision))
				if errG != nil {
					return nil, errG
				}
			} else {
				vm, errG = app.VMDB.GetActiveByName(vmName)
				if errG != nil {
					return nil, errG
				}
			}

			destAuth, errP := app.SSHPairDB.GetPublicKeyAuth(vm.MulchSuperUserSSHKey)
			if errP != nil {
				return nil, errP
			}

			client.vm = vm
			client.sshUser = user
			client.startTime = time.Now()
			client.remoteAddr = c.RemoteAddr()

			clientConfig := &ssh.ClientConfig{}

			clientConfig.User = user
			clientConfig.Auth = []ssh.AuthMethod{
				destAuth,
			}
			clientConfig.HostKeyCallback = ssh.InsecureIgnoreHostKey()

			app.Log.Tracef("SSH Proxy: dial %s@%s", user, vm.LastIP)

			sshClient, errD := ssh.Dial("tcp", vm.LastIP+":22", clientConfig)
			if errD != nil {
				return nil, errD
			}

			client.sshClient = sshClient

			app.sshClients.add(c.RemoteAddr(), &client)
			return nil, nil
		},
	}

	// key of our server
	config.AddHostKey(ourPrivate)

	err = ListenAndServeProxy(
		app.Config.ProxyListenSSH,
		config,
		app.sshClients,
		app.Log,
		func(c ssh.ConnMetadata) (*sshServerClient, error) {
			client := app.sshClients.findByAddress(c.RemoteAddr())
			// we could delete entry here, but we keep it for infos/stats (see status command)
			app.Log.Tracef("SSH proxy: connection accepted from %s forwarded to %s", c.RemoteAddr(), client.sshClient.RemoteAddr())

			return client, err
		}, func(c ssh.ConnMetadata) error {
			app.sshClients.delete(c.RemoteAddr())
			app.Log.Tracef("SSH proxy: connection closed from: %s", c.RemoteAddr())
			return nil
		})
	if err != nil {
		return err
	}

	app.Log.Infof("SSH proxy server listening on %s", app.Config.ProxyListenSSH)
	return nil
}

// newSSHServerClients create a new sshClients database
func newSSHServerClients() *sshServerClients {
	return &sshServerClients{
		db: make(map[net.Addr]*sshServerClient),
	}
}

// add a client with address addr to the database
func (sc *sshServerClients) add(addr net.Addr, client *sshServerClient) {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()
	sc.db[addr] = client
}

// delete a client by its address
func (sc *sshServerClients) delete(addr net.Addr) {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()
	delete(sc.db, addr)
}

func (sc *sshServerClients) findByAddress(addr net.Addr) *sshServerClient {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()
	return sc.db[addr]
}

// getClients as an array
func (sc *sshServerClients) getClients() []*sshServerClient {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()

	clients := make([]*sshServerClient, 0, len(sc.db))
	for _, entry := range sc.db {
		clients = append(clients, entry)
	}
	return clients
}
