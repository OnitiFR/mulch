package server

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// SSHConnection stores connection informations
type SSHConnection struct {
	User  string
	Auths []ssh.AuthMethod
	Host  string
	Port  int
	// Ciphers []string
	Session *ssh.Session
	Client  *ssh.Client
	Log     *Log
}

// Close will clone the connection and the session
func (connection *SSHConnection) Close() error {
	var (
		sessionError error
		clientError  error
	)

	connection.Log.Tracef("SSH closing connection (%s)", connection.Host)

	if connection.Session != nil {
		sessionError = connection.Session.Close()
	}
	if connection.Client != nil {
		clientError = connection.Client.Close()
	}

	if clientError != nil {
		return clientError
	}

	return sessionError
}

// knownHostHash hash hostname using salt64 like ssh is
// doing for "hashed" .ssh/known_hosts files
/*func knownHostHash(hostname string, salt64 string) string {
	buffer, err := base64.StdEncoding.DecodeString(salt64)
	if err != nil {
		return ""
	}
	h := hmac.New(sha1.New, buffer)
	h.Write([]byte(hostname))
	res := h.Sum(nil)

	hash := base64.StdEncoding.EncodeToString(res)
	return hash
}*/

// We don't check SSH fingerprint, because it changes at each VM reconstruction
// and we're not using SSH outside of local host
func hostKeyBilndTrustChecker(hostname string, remote net.Addr, key ssh.PublicKey) error {
	return nil
}

// Connect will dial SSH server and open a session
func (connection *SSHConnection) Connect() error {
	sshConfig := &ssh.ClientConfig{
		User: connection.User,
		Auth: connection.Auths,
	}

	sshConfig.HostKeyCallback = hostKeyBilndTrustChecker

	// if len(connection.Ciphers) > 0 {
	// 	sshConfig.Config = ssh.Config{
	// 		Ciphers: connection.Ciphers,
	// 	}
	// }

	dial, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", connection.Host, connection.Port), sshConfig)
	connection.Log.Tracef("SSH connection to %s@%s:%d", connection.User, connection.Host, connection.Port)
	if err != nil {
		return fmt.Errorf("failed to dial: %s", err)
	}
	connection.Client = dial

	session, err := dial.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %s", err)
	}
	connection.Session = session

	return nil
}

// PublicKeyFile returns an AuthMethod using a private key file
func PublicKeyFile(file string) ssh.AuthMethod {
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

// SSHAgent returns an AuthMethod using SSH agent connection. The pubkeyFile
// params restricts the AuthMethod to only one key, so it wont spam the
// SSH server if the agent holds multiple keys.
func SSHAgent(pubkeyFile string, log *Log) (ssh.AuthMethod, error) {
	sshAgent, errd := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	if errd == nil {
		agent := agent.NewClient(sshAgent)

		// we'll try every key, then
		if pubkeyFile == "" {
			return ssh.PublicKeysCallback(agent.Signers), nil
		}

		agentSigners, err := agent.Signers()
		if err != nil {
			return nil, fmt.Errorf("requesting SSH agent key/signer list: %s", err)
		}

		buffer, err := ioutil.ReadFile(pubkeyFile)
		if err != nil {
			return nil, fmt.Errorf("reading public key '%s': %s", pubkeyFile, err)
		}

		fields := strings.Fields(string(buffer))

		if len(fields) < 3 {
			return nil, fmt.Errorf("invalid field count for public key '%s'", pubkeyFile)
		}

		buffer2, err := base64.StdEncoding.DecodeString(fields[1])
		if err != nil {
			return nil, fmt.Errorf("decoding public key '%s': %s", pubkeyFile, err)
		}

		key, err := ssh.ParsePublicKey(buffer2)
		if err != nil {
			return nil, fmt.Errorf("parsing public key '%s': %s", pubkeyFile, err)
		}

		for _, potentialSigner := range agentSigners {
			if bytes.Equal(key.Marshal(), potentialSigner.PublicKey().Marshal()) {
				log.Tracef("successfully found %s key in the SSH agent (%s)", pubkeyFile, fields[2])
				cb := func() ([]ssh.Signer, error) {
					signers := []ssh.Signer{potentialSigner}
					return signers, nil
				}
				return ssh.PublicKeysCallback(cb), nil
			}
		}
		return nil, fmt.Errorf("can't find '%s' key in the SSH agent", pubkeyFile)
	}
	return nil, fmt.Errorf("SSH agent: %v (check SSH_AUTH_SOCK?)", errd)
}
