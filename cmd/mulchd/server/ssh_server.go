package server

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"strings"

	"golang.org/x/crypto/ssh"
)

// NewSSHProxyServer creates and starts our SSH proxy to VMs
func NewSSHProxyServer(app *App) {

	// generate this and store somewhere? do the same for mulchd vm admin key?
	ourKey := flag.String("our-key", "id_rsa_our", "rsa key to use for our identity")
	destKey := flag.String("dest-key", "id_rsa_dest", "rsa key for authentication on destination")

	// will be replaced with dynamic API key search
	authorized := flag.String("authorized", "", "authorized keys file")

	// ...
	dest := flag.String("dest", "xxxx:22", "destination address")
	flag.Parse()

	authorizedKeysBytes, err := ioutil.ReadFile(*authorized)
	if err != nil {
		log.Fatalf("Failed to load authorized keys, err: %v", err)
	}

	authorizedKeysMap := map[string]bool{}
	for len(authorizedKeysBytes) > 0 {
		pubKey, _, _, rest, errP := ssh.ParseAuthorizedKey(authorizedKeysBytes)
		if errP != nil {
			log.Fatal(errP)
		}

		authorizedKeysMap[string(pubKey.Marshal())] = true
		authorizedKeysBytes = rest
	}

	// our private key
	ourPrivateBytes, err := ioutil.ReadFile(*ourKey)
	if err != nil {
		fmt.Println(*ourKey)
		panic("Failed to load private key")
	}

	ourPrivate, err := ssh.ParsePrivateKey(ourPrivateBytes)
	if err != nil {
		fmt.Println(*ourKey)
		fmt.Println(err)
		panic("Failed to parse private key")
	}

	// private key for destination auth
	destPrivateBytes, err := ioutil.ReadFile(*destKey)
	if err != nil {
		fmt.Println(*destKey)
		panic("Failed to load private key")
	}

	destPrivate, err := ssh.ParsePrivateKey(destPrivateBytes)
	if err != nil {
		fmt.Println(*destKey)
		fmt.Println(err)
		panic("Failed to parse private key")
	}

	var clients = make(map[net.Addr]*ssh.Client)

	config := &ssh.ServerConfig{
		PublicKeyCallback: func(c ssh.ConnMetadata, pubKey ssh.PublicKey) (*ssh.Permissions, error) {
			if !authorizedKeysMap[string(pubKey.Marshal())] {
				return nil, fmt.Errorf("unknown public key for %q", c.User())
			}
			parts := strings.Split(c.User(), "@")
			if len(parts) != 2 {
				return nil, fmt.Errorf("wrong user format '%s' (user@vm needed)", c.User())
			}
			user := parts[0]
			vmName := parts[1]

			fmt.Printf("Login attempt: %s, user %s to vm %s\n", c.RemoteAddr(), user, vmName)

			clientConfig := &ssh.ClientConfig{}

			clientConfig.User = user
			clientConfig.Auth = []ssh.AuthMethod{
				ssh.PublicKeys(destPrivate), // key to auth to our destination
			}
			clientConfig.HostKeyCallback = ssh.InsecureIgnoreHostKey()

			client, errD := ssh.Dial("tcp", *dest, clientConfig)
			if errD != nil {
				fmt.Printf("dial failed: %s\n", errD)
				return nil, errD
			}

			clients[c.RemoteAddr()] = client
			return nil, nil
		},
	}

	// key of our server
	config.AddHostKey(ourPrivate)

	ListenAndServeProxy(app.Config.ProxyListenSSH, config, func(c ssh.ConnMetadata) (*ssh.Client, error) {
		client, _ := clients[c.RemoteAddr()]
		delete(clients, c.RemoteAddr())

		fmt.Printf("main: Connection accepted from %s forwarded to %s\n", c.RemoteAddr(), client.RemoteAddr())

		return client, err
	}, func(c ssh.ConnMetadata) error {
		fmt.Printf("main: Connection closed from: %s\n", c.RemoteAddr())
		return nil
	})
}
