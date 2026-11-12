package controllers

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"

	"github.com/OnitiFR/mulch/cmd/mulchd/server"
	"github.com/OnitiFR/mulch/common"
	"golang.org/x/crypto/ssh"
)

// authFail logs the detailed reason and returns a generic error to the client
func authFail(req *server.Request, reason string) {
	ip, _, _ := net.SplitHostPort(req.HTTP.RemoteAddr)
	req.App.Log.Warningf("SSH auth failed from %s: %s", ip, reason)
	http.Error(req.Response, "SSH authentication failed", http.StatusForbidden)
}

func authParsePublicKey(req *server.Request) (ssh.PublicKey, bool) {
	pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(req.HTTP.FormValue("pubkey")))
	if err != nil {
		authFail(req, "invalid public key: "+err.Error())
		return nil, false
	}
	return pub, true
}

func authWriteJSON(req *server.Request, data any) {
	req.Response.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(req.Response)
	err := enc.Encode(data)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
	}
}

// AuthChallengeController returns a challenge to be signed by the
// given (personal) SSH key
func AuthChallengeController(req *server.Request) {
	pub, ok := authParsePublicKey(req)
	if !ok {
		return
	}
	fingerprint := ssh.FingerprintSHA256(pub)

	apiKey := req.App.APIKeysDB.GetBySSHAuthorizedKey(pub)
	if apiKey == nil {
		authFail(req, "unknown SSH key "+fingerprint)
		return
	}

	challenge, expires, err := req.App.AuthTokens.NewChallenge(pub)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), http.StatusServiceUnavailable)
		return
	}

	authWriteJSON(req, &common.APIAuthChallenge{
		Challenge: challenge,
		PublicKey: string(ssh.MarshalAuthorizedKey(pub)),
		Expires:   expires,
	})
}

// AuthTokenController checks the signed challenge and returns a session token
func AuthTokenController(req *server.Request) {
	pub, ok := authParsePublicKey(req)
	if !ok {
		return
	}
	fingerprint := ssh.FingerprintSHA256(pub)

	challenge := req.HTTP.FormValue("challenge")
	err := req.App.AuthTokens.ConsumeChallenge(challenge, pub)
	if err != nil {
		authFail(req, err.Error()+" for "+fingerprint)
		return
	}

	sigBytes, err := base64.StdEncoding.DecodeString(req.HTTP.FormValue("signature"))
	if err != nil {
		authFail(req, "invalid signature encoding for "+fingerprint)
		return
	}

	var sig ssh.Signature
	err = ssh.Unmarshal(sigBytes, &sig)
	if err != nil {
		authFail(req, "invalid signature format for "+fingerprint)
		return
	}

	// SHA-1 RSA signatures are not accepted
	if sig.Format == ssh.KeyAlgoRSA {
		authFail(req, "ssh-rsa (SHA-1) signature refused for "+fingerprint)
		return
	}

	// the signature is bound to our host name (see common.SSHAuthMessage)
	host := req.App.SSHAuthServerHost(req.HTTP)
	data := common.SSHSigSignedData(common.SSHAuthNamespace, common.SSHAuthMessage(challenge, host))
	err = pub.Verify(data, &sig)
	if err != nil {
		authFail(req, "bad signature for "+fingerprint+" (signed for another server than '"+host+"'?): "+err.Error())
		return
	}

	// the key may have been revoked during the challenge
	apiKey := req.App.APIKeysDB.GetBySSHAuthorizedKey(pub)
	if apiKey == nil {
		authFail(req, "unknown SSH key "+fingerprint)
		return
	}

	token, expires, err := req.App.AuthTokens.NewToken(apiKey, pub)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
		return
	}

	ip, _, _ := net.SplitHostPort(req.HTTP.RemoteAddr)
	req.App.Log.Infof("SSH auth: '%s' authenticated from %s with %s", apiKey.Comment, ip, fingerprint)

	authWriteJSON(req, &common.APIAuthToken{
		Token:      token,
		Expires:    expires,
		KeyComment: apiKey.Comment,
	})
}
