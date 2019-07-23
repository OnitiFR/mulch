package server

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/OnitiFR/mulch/common"
)

// CertManager for HTTPS API server, using mulch-proxy certificates
type CertManager struct {
	CertDir     string
	Domain      string
	Log         *Log
	certModTime time.Time
	cachedCert  *tls.Certificate
	mutex       sync.Mutex
}

// -- Ripped from acme/autocert package
// Attempt to parse the given private key DER block. OpenSSL 0.9.8 generates
// PKCS#1 private keys by default, while OpenSSL 1.0.0 generates PKCS#8 keys.
// OpenSSL ecparam generates SEC1 EC private keys for ECDSA. We try all three.
//
// Inspired by parsePrivateKey in crypto/tls/tls.go.
func parsePrivateKey(der []byte) (crypto.Signer, error) {
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		switch key := key.(type) {
		case *rsa.PrivateKey:
			return key, nil
		case *ecdsa.PrivateKey:
			return key, nil
		default:
			return nil, errors.New("unknown private key type in PKCS#8 wrapping")
		}
	}
	if key, err := x509.ParseECPrivateKey(der); err == nil {
		return key, nil
	}

	return nil, errors.New("failed to parse private key")
}

// GetAPICertificate implements tls.Config GetCertificate callback
func (cm *CertManager) GetAPICertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	name := hello.ServerName
	if name != cm.Domain {
		return nil, fmt.Errorf("unkown host '%s'", name)
	}

	certPath := path.Clean(cm.CertDir + "/" + name)

	if !common.PathExist(certPath) {
		// the certificate does not exists (yet), let's try to create it
		cm.Log.Tracef("'%s' does not exists yet", certPath)
		cm.selfCall()
	}

	infos, err := os.Stat(certPath)
	if err != nil {
		return nil, fmt.Errorf("stat '%s': %s", certPath, err)
	}

	// unmodified mtime, returning cached certificate
	if infos.ModTime().Equal(cm.certModTime) {
		return cm.cachedCert, nil
	}

	cm.Log.Trace("reading API TLS cert from disk")
	content, err := ioutil.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("reading '%s': %s", certPath, err)
	}

	// private
	priv, pub := pem.Decode(content)
	if priv == nil || !strings.Contains(priv.Type, "PRIVATE") {
		return nil, errors.New("can't find private key")
	}

	privKey, err := parsePrivateKey(priv.Bytes)
	if err != nil {
		return nil, err
	}

	// public
	var pubDER [][]byte
	for len(pub) > 0 {
		var b *pem.Block
		b, pub = pem.Decode(pub)
		if b == nil {
			break
		}
		pubDER = append(pubDER, b.Bytes)
	}
	if len(pub) > 0 {
		// Leftover content not consumed by pem.Decode. Corrupt. Ignore.
		return nil, errors.New("corrupted public key")
	}

	tlscert := &tls.Certificate{
		Certificate: pubDER,
		PrivateKey:  privKey,
	}

	cm.certModTime = infos.ModTime()
	cm.cachedCert = tlscert

	return tlscert, nil
}

func (cm *CertManager) selfCall() error {
	cm.Log.Trace("self HTTPS URL call to generate/renew certificate")

	timeout := time.Duration(10 * time.Second)
	client := http.Client{
		Timeout: timeout,
	}
	res, err := client.Get("https://" + cm.Domain + "/")
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return nil
}

// ScheduleSelfCalls call our own API HTTPS URL every 24 hour, refreshing
// the TLS certificate.
func (cm *CertManager) ScheduleSelfCalls() {
	time.Sleep(1 * time.Second)
	go func() {
		err := cm.selfCall()
		if err != nil {
			cm.Log.Warningf("unable to call our own HTTPS domain: %s", err)
		}
		time.Sleep(24 * time.Hour)
	}()
}
