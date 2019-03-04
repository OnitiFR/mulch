package server

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"

	"golang.org/x/crypto/ssh"
)

// MakeSSHKey generates a OpenSSH formatted key pair
func MakeSSHKey() (private string, public string, err error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return "", "", err
	}

	bufPriv := new(bytes.Buffer)
	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}

	if errE := pem.Encode(bufPriv, privateKeyPEM); err != nil {
		return "", "", errE
	}

	// generate and write public key
	pub, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", "", err
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
		if bytes.Compare(pubKey.Marshal(), searchedPubKey.Marshal()) == 0 {
			return pubKey, comment, nil
		}
	}

	return nil, "", nil
}
