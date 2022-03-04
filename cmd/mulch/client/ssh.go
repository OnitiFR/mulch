package client

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path"

	"github.com/OnitiFR/mulch/common"
)

// MulchSSHSubDir is the name of mulch dedicated .ssh sub-directory
const MulchSSHSubDir = "mulch/"

// SSHKeyPrefix is the prefix for SSH keys (was 'id_rsa_' but we
// switched to ed25519, so let's use a generic name)
const SSHKeyPrefix = "id_"

// GetSSHPath returns the path of a file in the user SSH config path
func GetSSHPath(file string) string {
	return path.Clean(GlobalHome + "/.ssh/" + file)
}

// CreateSSHMulchDir creates (if needed) user SSH config path and, inside,
// mulch directory.
func CreateSSHMulchDir() error {
	sshPath := GetSSHPath("")
	mulchPath := GetSSHPath(MulchSSHSubDir)

	if !common.PathExist(sshPath) {
		err := os.Mkdir(sshPath, 0700)
		if err != nil {
			return err
		}
	}

	if !common.PathExist(mulchPath) {
		err := os.Mkdir(mulchPath, 0755)
		if err != nil {
			return err
		}
	}

	return nil
}

// GetSSHHost returns the SSH server hostname based on
// mulchd API URL
func GetSSHHost() (string, error) {
	url, err := url.Parse(GlobalConfig.Server.URL)
	if err != nil {
		return "", err
	}

	return url.Hostname(), nil
}

// WriteSSHPair overwrites public and private API SSH files,
// using an APISSHPair reader (see "GET /sshpair") as input.
// Returns public key filepath, private filepath and error.
func WriteSSHPair(reader io.Reader) (string, string, error) {
	var data common.APISSHPair
	dec := json.NewDecoder(reader)
	err := dec.Decode(&data)
	if err != nil {
		return "", "", err
	}
	// save files using current server name
	privFilePath := GetSSHPath(MulchSSHSubDir + SSHKeyPrefix + GlobalConfig.Server.Name)
	pubFilePath := privFilePath + ".pub"

	err = ioutil.WriteFile(privFilePath, []byte(data.Private), 0600)
	if err != nil {
		return "", "", err
	}

	err = ioutil.WriteFile(pubFilePath, []byte(data.Public), 0644)
	if err != nil {
		return "", "", err
	}
	return pubFilePath, privFilePath, err
}
