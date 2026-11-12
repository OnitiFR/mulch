package common

import (
	"crypto/sha512"
	"net"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHAuthNamespace is the SSHSIG namespace used to sign authentication
// challenges. Using a dedicated namespace prevents a signature made for
// mulch from being reused in another context (git commit, file, SSH session…)
const SSHAuthNamespace = "mulch-auth"

// SSHRequestNamespace is the SSHSIG namespace used to sign sensitive
// requests (see SSHRequestMessage). It's different from SSHAuthNamespace
// so a login signature can't be used as a request signature, and vice versa.
const SSHRequestNamespace = "mulch-request"

// HTTP headers used for request signatures: the server sends a challenge
// (with a 401 status) when a request must be signed, the client sends the
// request again with the challenge and its signature.
const (
	SSHRequestChallengeHeader = "Mulch-Request-Challenge"
	SSHRequestSignatureHeader = "Mulch-Request-Signature"
)

// SSHAuthHost normalizes a host name ("host", "host:port", "[::1]:8686"…):
// lowercase, without port, IPv6 brackets or trailing dot
func SSHAuthHost(hostport string) string {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	host = strings.TrimSuffix(host, ".")
	return strings.ToLower(host)
}

// SSHAuthMessage returns the message signed to authenticate: the challenge,
// bound to the server host name as seen by the client. This way, a
// signature obtained by a (compromised) server can't be relayed to another
// mulch server where the same SSH key is registered.
func SSHAuthMessage(challenge string, host string) []byte {
	return []byte(challenge + "\n" + SSHAuthHost(host))
}

// SSHRequestMessage returns the message signed to confirm a sensitive
// request: the challenge, the server host name (as for SSHAuthMessage),
// the method, the path and every parameter, so the signature can't be
// used for any other request.
func SSHRequestMessage(challenge string, host string, method string, path string, params url.Values) []byte {
	// Encode() sorts parameters by name
	return []byte(challenge + "\n" + SSHAuthHost(host) + "\n" + method + "\n" + path + "\n" + params.Encode())
}

// SSHSigSignedData returns the blob that must be signed (by an SSH agent,
// for instance) for the given namespace and message, following the
// OpenSSH SSHSIG format (see PROTOCOL.sshsig). This is the same data
// "ssh-keygen -Y sign -n <namespace>" signs.
func SSHSigSignedData(namespace string, message []byte) []byte {
	hash := sha512.Sum512(message)

	blob := struct {
		Namespace string
		Reserved  string
		HashAlg   string
		Hash      []byte
	}{
		Namespace: namespace,
		Reserved:  "",
		HashAlg:   "sha512",
		Hash:      hash[:],
	}

	return append([]byte("SSHSIG"), ssh.Marshal(blob)...)
}

// APIAuthChallenge is returned by "POST /auth/challenge"
type APIAuthChallenge struct {
	Challenge string
	PublicKey string // authorized_keys format, the key that must sign
	Expires   time.Time
}

// APIAuthToken is returned by "POST /auth/token"
type APIAuthToken struct {
	Token      string
	Expires    time.Time
	KeyComment string
}

// APIKeySSHEntries is a list of SSH public keys authorized for an API key
type APIKeySSHEntries []APIKeySSHEntry

// APIKeySSHEntry is an SSH public key authorized for an API key
type APIKeySSHEntry struct {
	Type        string
	Fingerprint string
	Comment     string
	AddedAt     time.Time
}
