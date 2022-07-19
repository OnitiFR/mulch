package client

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

var callCountForSFTPCopy = 0

// SFTPCopy is a WIP
func SFTPCopy(vmName string, user string, filename string) error {
	// only on first call
	if callCountForSFTPCopy == 0 {
		err := CreateSSHMulchDir()
		if err != nil {
			return err
		}

		call := GlobalAPI.NewCall("GET", "/sshpair", map[string]string{})
		call.JSONCallback = sftpPairCB
		call.Do()
	}
	callCountForSFTPCopy++

	privKeyFile := GetSSHPath(MulchSSHSubDir + SSHKeyPrefix + GlobalConfig.Server.Name)

	sshUser := user + "@" + vmName
	remote, err := GetSSHHost()
	if err != nil {
		return err
	}
	port := strconv.Itoa(GlobalConfig.Server.SSHPort)

	hostKeyCallback, err := knownhosts.New(GetSSHPath("known_hosts"))
	if err != nil {
		return err
	}

	config := &ssh.ClientConfig{
		User: sshUser,
		Auth: []ssh.AuthMethod{
			publicKeyAuthFromPubFile(privKeyFile),
		},
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			err := hostKeyCallback(hostname, remote, key)
			var keyError *knownhosts.KeyError
			if errors.As(err, &keyError) {
				fmt.Println("Client warning: host key is unknown, authenticity can't be established!")
				return nil
			}
			return err
		},
	}

	// connect
	conn, err := ssh.Dial("tcp", remote+":"+port, config)
	if err != nil {
		return err
	}
	defer conn.Close()

	// create new SFTP client
	client, err := sftp.NewClient(conn)
	if err != nil {
		return err
	}
	defer client.Close()

	// open source file
	srcFile, err := client.Open(filename)
	if err != nil {
		return err
	}

	reader := bufio.NewReader(srcFile)
	destFile := filepath.Base(filename)

	err = downloadFile(destFile, reader)
	if err != nil {
		return err
	}

	return nil
}

func sftpPairCB(reader io.Reader, _ http.Header) {
	_, _, err := WriteSSHPair(reader)
	if err != nil {
		// no other (easy) choice here: log.Fatal
		log.Fatal(err.Error())
	}
}

func publicKeyAuthFromPubFile(file string) ssh.AuthMethod {
	buffer, err := ioutil.ReadFile(file)
	if err != nil {
		return nil
	}

	key, err := ssh.ParsePrivateKey(buffer)
	if err != nil {
		return nil
	}
	return ssh.PublicKeys(key)
}
