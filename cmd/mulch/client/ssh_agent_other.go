//go:build !windows

package client

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
)

// dialSSHAgent connects to the SSH agent unix socket (SSH_AUTH_SOCK)
func dialSSHAgent() (io.ReadWriter, error) {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil, errors.New("ssh_pub_key is set, but no SSH agent is available (SSH_AUTH_SOCK is not set)")
	}

	conn, err := net.Dial("unix", sock)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to SSH agent: %s", err)
	}
	return conn, nil
}
