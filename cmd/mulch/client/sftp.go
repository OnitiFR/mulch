package client

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
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
	port := strconv.Itoa(SSHPort)

	config := &ssh.ClientConfig{
		User: sshUser,
		Auth: []ssh.AuthMethod{
			publicKeyAuthFromPubFile(privKeyFile),
		},
	}

	// try to get remote host public key
	// first try: with full [host]:port format
	hostKey, err := getHostKey("[" + remote + "]:" + port)
	if err != nil {
		fmt.Printf("warning: %s\n", err)
	}
	if hostKey == nil {
		// second try: with host:port format
		hostKey, err = getHostKey(remote + ":" + port)
		if err != nil {
			fmt.Printf("warning: %s\n", err)
		}
	}

	if hostKey == nil {
		fmt.Printf("WARNING: unable to find remote host key, file transfer is insecure and subject to MITM attacks!\n")
		config.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	} else {
		config.HostKeyCallback = ssh.FixedHostKey(hostKey)
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

func sftpPairCB(reader io.Reader, headers http.Header) {
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

func getHostKey(host string) (ssh.PublicKey, error) {
	// parse OpenSSH known_hosts file
	// ssh or use ssh-keyscan to get initial key
	file, err := os.Open(GetSSHPath("known_hosts"))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var hostKey ssh.PublicKey
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), " ")
		if len(fields) != 3 {
			continue
		}
		if fields[0] == host {
			var err error
			hostKey, _, _, _, err = ssh.ParseAuthorizedKey(scanner.Bytes())
			if err != nil {
				return nil, fmt.Errorf("error parsing %q: %v", fields[2], err)
			}
			break
		}
	}

	return hostKey, nil
}
