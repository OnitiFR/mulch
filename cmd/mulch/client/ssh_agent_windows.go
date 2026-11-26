//go:build windows

package client

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// named pipe of the Windows OpenSSH agent service
const windowsSSHAgentPipe = `\\.\pipe\openssh-ssh-agent`

// dialSSHAgent connects to the SSH agent: the Windows OpenSSH agent service
// by default, or SSH_AUTH_SOCK when set (named pipe or AF_UNIX socket)
func dialSSHAgent() (io.ReadWriter, error) {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		sock = windowsSSHAgentPipe
	}

	pipe := filepath.FromSlash(sock)
	if strings.HasPrefix(strings.ToLower(pipe), `\\.\pipe\`) {
		f, err := os.OpenFile(pipe, os.O_RDWR, 0)
		if err == nil {
			return f, nil
		}
		if errors.Is(err, fs.ErrNotExist) && strings.EqualFold(pipe, windowsSSHAgentPipe) {
			return nil, errors.New("unable to connect to SSH agent: is the Windows 'ssh-agent' service enabled and started? (see 'mulch help key ssh')")
		}
		return nil, fmt.Errorf("unable to connect to SSH agent: %s", err)
	}

	// Cygwin/MSYS agents (ex: Git Bash's ssh-agent) use emulated sockets,
	// which fail here
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to SSH agent (SSH_AUTH_SOCK=%s): %s\nGit Bash / MSYS / Cygwin agents are not supported, unset SSH_AUTH_SOCK to use the Windows 'ssh-agent' service", sock, err)
	}
	return conn, nil
}
