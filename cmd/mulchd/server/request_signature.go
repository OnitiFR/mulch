package server

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"

	"github.com/OnitiFR/mulch/common"
	"golang.org/x/crypto/ssh"
)

// maximum size of in-memory form parsing (same as net/http default)
const requestSignatureMaxMemory = 32 << 20

// checkRequestSignature checks that the request was signed by the SSH key
// used to open the session (RouteAuthFresh routes). Without any signature,
// the client gets a challenge (401) and is expected to sign the request and
// send it again. Returns false if the request must not go further (the
// response is already sent).
func (req *Request) checkRequestSignature(pub ssh.PublicKey) bool {
	r := req.HTTP
	w := req.Response
	app := req.App
	fingerprint := ssh.FingerprintSHA256(pub)

	challenge := r.Header.Get(common.SSHRequestChallengeHeader)
	sigB64 := r.Header.Get(common.SSHRequestSignatureHeader)

	if challenge == "" || sigB64 == "" {
		newChallenge, _, err := app.AuthTokens.NewChallenge(pub)
		if err != nil {
			app.Log.Error(err.Error())
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return false
		}
		w.Header().Set(common.SSHRequestChallengeHeader, newChallenge)
		http.Error(w, "this request must be signed with your SSH key", http.StatusUnauthorized)
		return false
	}

	err := req.verifyRequestSignature(pub, challenge, sigB64)
	if err != nil {
		app.Log.Warningf("request signature failed from %s for %s %s with %s: %s", req.RemoteIP(), r.Method, r.URL.Path, fingerprint, err)
		app.AlertSender.SecurityThrottled("Request signature refused", req.APIKey.Comment, fmt.Sprintf(
			"a valid session of %s was used without a valid signature for %s %s: stolen session token?",
			req.Author(), r.Method, r.URL.Path))
		http.Error(w, "request signature verification failed", http.StatusForbidden)
		return false
	}

	app.Log.Tracef("request signature verified for %s %s with %s", r.Method, r.URL.Path, fingerprint)
	return true
}

func (req *Request) verifyRequestSignature(pub ssh.PublicKey, challenge string, sigB64 string) error {
	r := req.HTTP

	err := req.App.AuthTokens.ConsumeChallenge(challenge, pub)
	if err != nil {
		return err
	}

	sigBytes, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return errors.New("invalid signature encoding")
	}

	var sig ssh.Signature
	err = ssh.Unmarshal(sigBytes, &sig)
	if err != nil {
		return errors.New("invalid signature format")
	}

	// SHA-1 RSA signatures are not accepted
	if sig.Format == ssh.KeyAlgoRSA {
		return errors.New("ssh-rsa (SHA-1) signature refused")
	}

	// parse every parameter (query string and body), the same way
	// handlers will (FormValue), so they're all covered by the signature
	err = r.ParseMultipartForm(requestSignatureMaxMemory)
	if err != nil && !errors.Is(err, http.ErrNotMultipart) {
		return err
	}
	// uploaded files are not covered by the signature
	if r.MultipartForm != nil && len(r.MultipartForm.File) > 0 {
		return errors.New("file upload is not supported for signed requests")
	}

	host := req.App.SSHAuthServerHost(r)
	message := common.SSHRequestMessage(challenge, host, r.Method, r.URL.Path, r.Form)
	data := common.SSHSigSignedData(common.SSHRequestNamespace, message)
	err = pub.Verify(data, &sig)
	if err != nil {
		return errors.New("bad signature (signed for another request or server than '" + host + "'?): " + err.Error())
	}
	return nil
}
