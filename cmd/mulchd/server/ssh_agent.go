package server

import (
	"errors"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

type SSHAgentProxy struct {
	agent  agent.ExtendedAgent
	client ssh.Channel
	apiKey *APIKey
	vm     *VM
	log    *Log
}

var ErrSSHProxyNotAllowed = errors.New("this action is not allowed through the SSH proxy")
var ErrSSHProxyNotForwarded = errors.New("this key is not forwarded to this VM")

// NewSSHProxyAgent creates a new SSH agent proxy
func NewSSHProxyAgent(realAgent ssh.Channel, client ssh.Channel, vm *VM, apiKey *APIKey, log *Log) *SSHAgentProxy {
	return &SSHAgentProxy{
		agent:  agent.NewClient(realAgent),
		client: client,
		vm:     vm,
		apiKey: apiKey,
		log:    log,
	}
}

func (p *SSHAgentProxy) isKeyAllowed(key ssh.PublicKey) bool {
	if p.apiKey == nil {
		return false
	}

	for _, fingerprint := range p.apiKey.SSHAllowedFingerprints {
		if fingerprint.VMName != p.vm.Config.Name {
			continue
		}

		if ssh.FingerprintSHA256(key) == fingerprint.Fingerprint {
			return true
		}
	}
	return false
}

// List returns the identities known to the agent
func (p *SSHAgentProxy) List() ([]*agent.Key, error) {
	keys, err := p.agent.List()
	if err != nil {
		return nil, err
	}

	res := make([]*agent.Key, 0)
	for _, v := range keys {
		if !p.isKeyAllowed(v) {
			continue
		}
		// p.log.Tracef("SSH agent: List: %s %s", ssh.FingerprintSHA256(v), v.Comment)
		res = append(res, v)
	}

	return res, nil
}

// Sign has the agent sign the data using a protocol 2 key as defined
// in [PROTOCOL.agent] section 2.6.2
func (p *SSHAgentProxy) Sign(key ssh.PublicKey, data []byte) (*ssh.Signature, error) {
	if !p.isKeyAllowed(key) {
		p.log.Tracef("SSH agent error: Sign: %s", ErrSSHProxyNotForwarded)
		return nil, ErrSSHProxyNotForwarded
	}
	return p.agent.Sign(key, data)
}

// SignWithFlags has the agent sign the data using a protocol 2 key as defined
// in [PROTOCOL.agent] section 2.6.2
func (p *SSHAgentProxy) SignWithFlags(key ssh.PublicKey, data []byte, flags agent.SignatureFlags) (*ssh.Signature, error) {
	if !p.isKeyAllowed(key) {
		p.log.Tracef("SSH agent error: SignWithFlags: %s", ErrSSHProxyNotForwarded)
		return nil, ErrSSHProxyNotForwarded
	}
	return p.agent.SignWithFlags(key, data, flags)
}

// Extension sends a vendor-specific extension message to the agent
func (p *SSHAgentProxy) Extension(extensionType string, contents []byte) ([]byte, error) {
	p.log.Tracef("SSH agent: Extension")
	return p.agent.Extension(extensionType, contents)
}

// Add adds a private key to the agent
func (p *SSHAgentProxy) Add(key agent.AddedKey) error {
	p.log.Tracef("SSH agent error: Add: %s", ErrSSHProxyNotAllowed)
	return ErrSSHProxyNotAllowed
}

// Remove removes all identities with the given public key
func (p *SSHAgentProxy) Remove(key ssh.PublicKey) error {
	p.log.Tracef("SSH agent error: Remove: %s", ErrSSHProxyNotAllowed)
	return ErrSSHProxyNotAllowed
}

// RemoveAll removes all identities
func (p *SSHAgentProxy) RemoveAll() error {
	p.log.Tracef("SSH agent error: RemoveAll: %s", ErrSSHProxyNotAllowed)
	return ErrSSHProxyNotAllowed
}

// Lock locks the agent
func (p *SSHAgentProxy) Lock(passphrase []byte) error {
	p.log.Tracef("SSH agent error: Lock: %s", ErrSSHProxyNotAllowed)
	return ErrSSHProxyNotAllowed
}

// Unlock undoes a previous Lock
func (p *SSHAgentProxy) Unlock(passphrase []byte) error {
	p.log.Tracef("SSH agent error: Unlock: %s", ErrSSHProxyNotAllowed)
	return ErrSSHProxyNotAllowed
}

// Signers returns signers for all keys
func (p *SSHAgentProxy) Signers() ([]ssh.Signer, error) {
	p.log.Tracef("SSH agent error: Signers: %s", ErrSSHProxyNotAllowed)
	return nil, ErrSSHProxyNotAllowed
}

// Serve starts the SSH agent proxy
func (p *SSHAgentProxy) Serve() {
	agent.ServeAgent(p, p.client)
}
