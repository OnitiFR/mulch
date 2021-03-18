package server

import (
	"bytes"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"

	"github.com/mikesmitty/edkey"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh"
)

// MakeSSHKey generates a OpenSSH formatted key pair (ED25519)
func MakeSSHKey() (private string, public string, err error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("key generation: %s", err)
	}

	bufPriv := new(bytes.Buffer)
	privateKeyPEM := &pem.Block{
		Type:  "OPENSSH PRIVATE KEY",
		Bytes: edkey.MarshalED25519PrivateKey(privateKey),
	}

	if errE := pem.Encode(bufPriv, privateKeyPEM); err != nil {
		return "", "", fmt.Errorf("pem encoding: %s", errE)
	}

	// generate and write public key
	pub, err := ssh.NewPublicKey(publicKey)
	if err != nil {
		return "", "", fmt.Errorf("public key generation: %s", err)
	}
	return bufPriv.String(), string(ssh.MarshalAuthorizedKey(pub)), nil
}

// SearchSSHAuthorizedKey search a public key in an authorized_keys formatted file
// and return key & comment
func SearchSSHAuthorizedKey(searchedPubKey ssh.PublicKey, authorizedKeysFile string) (ssh.PublicKey, string, error) {

	if authorizedKeysFile == "" {
		return nil, "", nil
	}

	// check mode
	stat, err := os.Stat(authorizedKeysFile)
	if err != nil {
		return nil, "", err
	}

	requiredMode, err := strconv.ParseInt("0600", 8, 32)
	if err != nil {
		return nil, "", err
	}

	if stat.Mode() != os.FileMode(requiredMode) {
		return nil, "", fmt.Errorf("%s: only the owner should be able to read/write this file (chmod 0600 %s)", authorizedKeysFile, authorizedKeysFile)
	}

	authorizedKeysBytes, err := ioutil.ReadFile(authorizedKeysFile)
	if err != nil {
		return nil, "", err
	}

	for len(authorizedKeysBytes) > 0 {
		pubKey, comment, _, rest, errP := ssh.ParseAuthorizedKey(authorizedKeysBytes)
		authorizedKeysBytes = rest

		if errP != nil {
			continue
		}
		if bytes.Equal(pubKey.Marshal(), searchedPubKey.Marshal()) {
			return pubKey, comment, nil
		}
	}

	return nil, "", nil
}
