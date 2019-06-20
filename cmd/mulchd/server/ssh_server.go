package server

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type sshServerClient struct {
	sshClient  *ssh.Client
	remoteAddr net.Addr
	vm         *VM
	vmID       string
	sshUser    string
	apiAuth    string
	startTime  time.Time
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

	destAuth, err := app.SSHPairDB.GetPublicKeyAuth(SSHSuperUserPair)
	if err != nil {
		return err
	}

	app.sshClients = make(map[net.Addr]*sshServerClient)

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

				user = parts[0]
				vmName = parts[1]
				client.apiAuth = apiKey.Comment
				app.Log.Infof("SSH Proxy: %s (API key '%s') %s@%s", c.RemoteAddr(), apiKey.Comment, user, vmName)
			} else {
				matchingPubKey, comment, errS := SearchSSHAuthorizedKey(pubKey, app.Config.ProxySSHExtraKeysFile)
				if errS != nil {
					return nil, errS
				}
				// Extra public key access
				if matchingPubKey != nil {
					client.apiAuth = "[pubKey] " + comment
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

					app.Log.Infof("SSH Proxy: %s (proxy_ssh_extra_keys_file) %s@%s", c.RemoteAddr(), user, vmName)
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
				revision, errA := strconv.Atoi(parts[1])
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

			app.Log.Infof("SSH Proxy: dial %s@%s", user, vm.LastIP)

			sshClient, errD := ssh.Dial("tcp", vm.LastIP+":22", clientConfig)
			if errD != nil {
				return nil, errD
			}

			client.sshClient = sshClient

			app.Log.Trace("SSH Proxy: adding client to the map")
			app.sshClients[c.RemoteAddr()] = &client
			return nil, nil
		},
	}

	// key of our server
	config.AddHostKey(ourPrivate)

	err = ListenAndServeProxy(
		app.Config.ProxyListenSSH,
		config,
		app.Log,
		func(c ssh.ConnMetadata) (*ssh.Client, error) {
			client, _ := app.sshClients[c.RemoteAddr()]
			// we could delete entry here, but we keep it for infos/stats (see status command)
			app.Log.Tracef("SSH proxy: connection accepted from %s forwarded to %s", c.RemoteAddr(), client.sshClient.RemoteAddr())

			return client.sshClient, err
		}, func(c ssh.ConnMetadata) error {
			delete(app.sshClients, c.RemoteAddr())
			app.Log.Tracef("SSH proxy: connection closed from: %s", c.RemoteAddr())
			return nil
		})
	if err != nil {
		return err
	}

	app.Log.Infof("SSH proxy server listening on %s", app.Config.ProxyListenSSH)
	return nil
}
