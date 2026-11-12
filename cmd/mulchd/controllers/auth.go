package controllers

import (
	"encoding/json"
	"errors"
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
// given (personal) SSH key. Challenges are issued for any key, registered
// or not, so this route can't be used to find out which keys are.
func AuthChallengeController(req *server.Request) {
	pub, ok := authParsePublicKey(req)
	if !ok {
		return
	}

	challenge, expires, err := req.App.AuthTokens.NewChallenge(pub)
	if err != nil {
		req.App.Log.Error(err.Error())
		http.Error(req.Response, err.Error(), 500)
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

	// unknown key (or revoked since the challenge). Checked first, so only
	// registered keys can fill the list of used challenges. The response
	// is the same as for a bad signature (no registered key disclosure).
	apiKey := req.App.APIKeysDB.GetBySSHAuthorizedKey(pub)
	if apiKey == nil {
		authFail(req, "unknown SSH key "+fingerprint)
		return
	}

	// the signature is bound to our host name (see common.SSHAuthMessage)
	challenge := req.HTTP.FormValue("challenge")
	host := req.App.SSHAuthServerHost(req.HTTP)
	data := common.SSHSigSignedData(common.SSHAuthNamespace, common.SSHAuthMessage(challenge, host))

	err := req.App.AuthTokens.ConsumeChallenge(challenge, pub, func() error {
		return server.VerifySSHSignature(pub, req.HTTP.FormValue("signature"), data)
	})
	if errors.Is(err, server.ErrAuthChallenge) {
		authFail(req, err.Error()+" for "+fingerprint)
		return
	}
	if err != nil {
		authFail(req, "bad signature for "+fingerprint+" (signed for another server than '"+host+"'?): "+err.Error())
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
