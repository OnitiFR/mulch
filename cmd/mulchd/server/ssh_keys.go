package server

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"

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
