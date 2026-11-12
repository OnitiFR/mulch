package client

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/OnitiFR/mulch/common"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// a cached token is considered expired this long before its real expiration
const tokenExpirationMargin = 60 * time.Second

// sshAuthState is the (lazily) resolved personal SSH key
type sshAuthState struct {
	agent agent.ExtendedAgent
	key   ssh.PublicKey
}

// SSHAuthEnabled returns true if the current server uses SSH key authentication
func SSHAuthEnabled() bool {
	return GlobalConfig != nil && GlobalConfig.Server.SSHPubKey != ""
}

func expandHome(p string) string {
	if p == "~" {
		return GlobalHome
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(GlobalHome, p[2:])
	}
	return p
}

// readPublicKeyFile reads an authorized_keys formatted public key file.
// The ".pub" suffix is checked first, so a private key is never read.
func readPublicKeyFile(path string) (ssh.PublicKey, error) {
	if !strings.HasSuffix(path, ".pub") {
		return nil, fmt.Errorf("ssh_pub_key: '%s' must be a public key file (*.pub)", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ssh_pub_key: %s", err)
	}

	pub, _, _, _, err := ssh.ParseAuthorizedKey(data)
	if err != nil {
		return nil, fmt.Errorf("ssh_pub_key: '%s' is not a valid public key: %s", path, err)
	}
	return pub, nil
}

// resolveSSHAuth connects to the SSH agent and finds the configured key in it
func (api *API) resolveSSHAuth() (*sshAuthState, error) {
	if api.sshAuth != nil {
		return api.sshAuth, nil
	}

	wanted, err := readPublicKeyFile(expandHome(api.SSHPubKey))
	if err != nil {
		return nil, err
	}

	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil, errors.New("ssh_pub_key is set, but no SSH agent is available (SSH_AUTH_SOCK is not set)")
	}

	conn, err := net.Dial("unix", sock)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to SSH agent: %s", err)
	}

	ag := agent.NewClient(conn)
	keys, err := ag.List()
	if err != nil {
		return nil, fmt.Errorf("unable to list SSH agent keys: %s", err)
	}

	for _, key := range keys {
		if string(key.Marshal()) == string(wanted.Marshal()) {
			api.sshAuth = &sshAuthState{agent: ag, key: key}
			return api.sshAuth, nil
		}
	}

	return nil, fmt.Errorf("ssh_pub_key %s is not loaded in your SSH agent (use ssh-add)", api.SSHPubKey)
}

// tokenCacheFile returns the path of the token cache file for this
// server and SSH key. We use XDG_RUNTIME_DIR (tmpfs, per-user, cleared
// at logout) when available.
func (api *API) tokenCacheFile(state *sshAuthState) (string, error) {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		var err error
		dir, err = os.UserCacheDir()
		if err != nil {
			return "", err
		}
	}
	dir = filepath.Join(dir, "mulch")

	err := os.MkdirAll(dir, 0700)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256([]byte(api.ServerURL + "\n" + ssh.FingerprintSHA256(state.key)))
	return filepath.Join(dir, "token-"+hex.EncodeToString(sum[:8])+".json"), nil
}

func (api *API) readCachedToken(state *sshAuthState) string {
	file, err := api.tokenCacheFile(state)
	if err != nil {
		return ""
	}

	data, err := os.ReadFile(file)
	if err != nil {
		return ""
	}

	var token common.APIAuthToken
	if json.Unmarshal(data, &token) != nil {
		return ""
	}

	if time.Now().Add(tokenExpirationMargin).After(token.Expires) {
		os.Remove(file)
		return ""
	}
	return token.Token
}

func (api *API) writeCachedToken(state *sshAuthState, token *common.APIAuthToken) error {
	file, err := api.tokenCacheFile(state)
	if err != nil {
		return err
	}

	data, err := json.Marshal(token)
	if err != nil {
		return err
	}

	// write + rename, so a concurrent mulch never reads a partial file
	tmp := fmt.Sprintf("%s.%d", file, os.Getpid())
	err = os.WriteFile(tmp, data, 0600)
	if err != nil {
		return err
	}
	return os.Rename(tmp, file)
}

func (api *API) clearCachedToken() {
	state, err := api.resolveSSHAuth()
	if err != nil {
		return
	}
	file, err := api.tokenCacheFile(state)
	if err != nil {
		return
	}
	os.Remove(file)
}

// getSecret returns the secret to send in the Mulch-Key header (a static
// API key or a session token) and whether it comes from the token cache
func (api *API) getSecret() (string, bool, error) {
	if api.SSHPubKey == "" {
		return api.APIKey, false, nil
	}

	state, err := api.resolveSSHAuth()
	if err != nil {
		return "", false, err
	}

	if token := api.readCachedToken(state); token != "" {
		return token, true, nil
	}

	token, err := api.sshAuthenticate(state)
	return token, false, err
}

// sshAuthenticate gets a new session token from the server, signing a
// challenge with the personal SSH key (using the agent)
func (api *API) sshAuthenticate(state *sshAuthState) (string, error) {
	pubStr := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(state.key)))
	fingerprint := ssh.FingerprintSHA256(state.key)

	var challenge common.APIAuthChallenge
	call := api.NewCall("POST", "/auth/challenge", map[string]string{
		"pubkey": pubStr,
	})
	call.noAuth = true
	call.JSONCallback = func(reader io.Reader, _ http.Header) {
		if err := json.NewDecoder(reader).Decode(&challenge); err != nil {
			log.Fatal(err)
		}
	}
	call.Do()

	serverURL, err := url.Parse(api.ServerURL)
	if err != nil {
		return "", err
	}
	message := common.SSHAuthMessage(challenge.Challenge, serverURL.Host)
	sig, err := sshSign(state, common.SSHAuthNamespace, message)
	if err != nil {
		return "", err
	}

	var token common.APIAuthToken
	call = api.NewCall("POST", "/auth/token", map[string]string{
		"pubkey":    pubStr,
		"challenge": challenge.Challenge,
		"signature": base64.StdEncoding.EncodeToString(ssh.Marshal(sig)),
	})
	call.noAuth = true
	call.JSONCallback = func(reader io.Reader, _ http.Header) {
		if err := json.NewDecoder(reader).Decode(&token); err != nil {
			log.Fatal(err)
		}
	}
	call.Do()

	if token.Token == "" {
		return "", errors.New("server returned an empty token")
	}

	err = api.writeCachedToken(state, &token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mulch: warning, unable to cache session token: %s\n", err)
	}

	if api.Trace {
		fmt.Fprintf(os.Stderr, "mulch: authenticated as '%s' with %s (session valid until %s)\n",
			token.KeyComment, fingerprint, token.Expires.Local().Format("15:04"))
	}

	return token.Token, nil
}

// sshSign signs a message (SSHSIG format) with the personal key, using the agent
func sshSign(state *sshAuthState, namespace string, message []byte) (*ssh.Signature, error) {
	fingerprint := ssh.FingerprintSHA256(state.key)

	if strings.HasPrefix(state.key.Type(), "sk-") {
		fmt.Fprintf(os.Stderr, "mulch: confirm user presence on your security key (%s)…\n", fingerprint)
	}

	var flags agent.SignatureFlags
	if state.key.Type() == ssh.KeyAlgoRSA {
		flags = agent.SignatureFlagRsaSha512
	}

	data := common.SSHSigSignedData(namespace, message)
	sig, err := state.agent.SignWithFlags(state.key, data, flags)
	if err != nil {
		return nil, fmt.Errorf("SSH agent refused to sign with %s: %s", fingerprint, err)
	}
	return sig, nil
}

// signRequest signs a sensitive request (see common.SSHRequestMessage),
// using the challenge sent by the server
func (call *APICall) signRequest(challenge string) (*requestSignature, error) {
	if len(call.files) > 0 {
		return nil, errors.New("file upload is not supported for signed requests")
	}

	state, err := call.api.resolveSSHAuth()
	if err != nil {
		return nil, err
	}

	apiURL, err := call.url()
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(apiURL)
	if err != nil {
		return nil, err
	}

	method := strings.ToUpper(call.Method)
	message := common.SSHRequestMessage(challenge, u.Host, method, u.Path, call.params())
	sig, err := sshSign(state, common.SSHRequestNamespace, message)
	if err != nil {
		return nil, err
	}

	return &requestSignature{
		challenge: challenge,
		signature: base64.StdEncoding.EncodeToString(ssh.Marshal(sig)),
	}, nil
}

// SSHAuthIdentityFile returns the public key file of the personal key,
// usable with "ssh -i" (with IdentitiesOnly), once checked it's in the agent
func SSHAuthIdentityFile() (string, error) {
	_, err := GlobalAPI.resolveSSHAuth()
	if err != nil {
		return "", err
	}
	return expandHome(GlobalConfig.Server.SSHPubKey), nil
}

// SSHAuthMethod returns an SSH auth method using the personal key, via the agent
func SSHAuthMethod() (ssh.AuthMethod, error) {
	state, err := GlobalAPI.resolveSSHAuth()
	if err != nil {
		return nil, err
	}

	signers, err := state.agent.Signers()
	if err != nil {
		return nil, err
	}

	for _, signer := range signers {
		if string(signer.PublicKey().Marshal()) == string(state.key.Marshal()) {
			return ssh.PublicKeys(signer), nil
		}
	}
	return nil, errors.New("personal SSH key not found in agent signers")
}

// RemoveLegacySSHPair deletes the generated SSH key pair of the current
// server (~/.ssh/mulch/id_<server>), useless with ssh_pub_key. It's
// generated again by 'mulch ssh' if the user goes back to a static key.
// Only call it once SSH aliases referencing it have been rewritten.
func RemoveLegacySSHPair() {
	privFile := GetSSHPath(MulchSSHSubDir + SSHKeyPrefix + GlobalConfig.Server.Name)
	for _, file := range []string{privFile, privFile + ".pub"} {
		err := os.Remove(file)
		if err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "mulch: warning, unable to remove obsolete %s: %s\n", file, err)
		}
	}
}
