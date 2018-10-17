package topics

import (
	"os"
	"path"
	"net/url"

	"github.com/Xfennec/mulch/common"
)

const mulchSubDir = "mulch/"
const sshKeyPrefix = "id_rsa_"
const sshPort = 8022

// GetSSHPath returns the path of a file in the user SSH config path
func GetSSHPath(file string) string {
	return path.Clean(globalHome + "/.ssh/" + file)
}

// CreateSSHMulchDir creates (if needed) user SSH config path and, inside,
// mulch directory.
func CreateSSHMulchDir() error {
	sshPath := GetSSHPath("")
	mulchPath := GetSSHPath(mulchSubDir)

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
	url, err := url.Parse(globalConfig.Server.URL)
	if err != nil {
		return "", err
	}

	return url.Hostname(), nil
}
