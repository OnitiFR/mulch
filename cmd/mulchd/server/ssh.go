package server

import (
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
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
func hostKeyBlindTrustChecker(hostname string, remote net.Addr, key ssh.PublicKey) error {
	return nil
}

// Connect will dial SSH server and open a session
func (connection *SSHConnection) Connect() error {
	sshConfig := &ssh.ClientConfig{
		User: connection.User,
		Auth: connection.Auths,
	}

	sshConfig.HostKeyCallback = hostKeyBlindTrustChecker

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
	buffer, err := os.ReadFile(file)
	if err != nil {
		return nil
	}

	key, err := ssh.ParsePrivateKey(buffer)
	if err != nil {
		return nil
	}
	return ssh.PublicKeys(key)
}

// SSHSendKeepAlive sends a keepalive request using a timeout
func SSHSendKeepAlive(sshConn ssh.Conn, timeout time.Duration) error {
	errChannel := make(chan error, 2)
	if timeout > 0 {
		time.AfterFunc(timeout, func() {
			// we will always timeout, but if we had a response, this new
			// error will go nowhere, so… no error.
			errChannel <- errors.New("[timeout]")
		})
	}

	go func() {
		_, _, err := sshConn.SendRequest("keepalive@golang.org", true, nil)
		errChannel <- err
	}()

	err := <-errChannel
	return err
}
