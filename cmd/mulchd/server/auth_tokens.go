package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// AuthChallengeTTL is the time a client has to sign a challenge
const AuthChallengeTTL = 60 * time.Second

// AuthTokenTTL is the lifetime of session tokens. Renewal is transparent
// with a software key (the agent signs silently), but needs a touch with
// a security key (-sk): 8h means one touch per working day.
const AuthTokenTTL = 8 * time.Hour

// AuthTokenPrefix is the prefix of session tokens (helps to recognize them)
const AuthTokenPrefix = "mulch-token-"

// challenge layout (before base64): expiration (unix time), random nonce,
// then HMAC-SHA256 of these two fields and the public key
const (
	challengeExpiresLen = 8
	challengeNonceLen   = 16
	challengePayloadLen = challengeExpiresLen + challengeNonceLen
	challengeLen        = challengePayloadLen + sha256.Size
)

// strict: a challenge has only one valid encoding (see usedChallenges)
var challengeEncoding = base64.RawURLEncoding.Strict()

// ErrAuthChallenge is returned for any invalid / expired / used challenge
var ErrAuthChallenge = errors.New("invalid or expired challenge")

type authToken struct {
	apiKey  *APIKey
	pubKey  string // marshaled
	expires time.Time
}

// AuthTokenStore issues SSH challenges and stores session tokens, in
// memory only (a mulchd restart invalidates every challenge and token,
// clients will transparently authenticate again).
//
// Challenges are stateless (HMAC, bound to a public key and an expiration):
// nothing is stored when a challenge is issued, so there's no limit to
// flood, and challenges can be issued for any key, registered or not.
// Only successfully used challenges are stored (until they expire), so
// they can't be used twice.
type AuthTokenStore struct {
	hmacKey        []byte
	usedChallenges map[string]time.Time  // challenge → expiration
	tokens         map[string]*authToken // key: sha256(token)
	mutex          sync.Mutex
}

// NewAuthTokenStore creates a new (empty) store
func NewAuthTokenStore() (*AuthTokenStore, error) {
	hmacKey := make([]byte, 32)
	if _, err := rand.Read(hmacKey); err != nil {
		return nil, err
	}

	return &AuthTokenStore{
		hmacKey:        hmacKey,
		usedChallenges: make(map[string]time.Time),
		tokens:         make(map[string]*authToken),
	}, nil
}

func randomBase64(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// purge expired entries (no locking)
func (s *AuthTokenStore) purge() {
	now := time.Now()
	for challenge, expires := range s.usedChallenges {
		if now.After(expires) {
			delete(s.usedChallenges, challenge)
		}
	}
	for id, t := range s.tokens {
		if now.After(t.expires) {
			delete(s.tokens, id)
		}
	}
}

// challengeMAC returns the HMAC of a challenge payload for this public key
// (no locking needed, hmacKey never changes)
func (s *AuthTokenStore) challengeMAC(payload []byte, pub ssh.PublicKey) []byte {
	mac := hmac.New(sha256.New, s.hmacKey)
	mac.Write(payload) // fixed size, can't overlap the key
	mac.Write(pub.Marshal())
	return mac.Sum(nil)
}

// NewChallenge creates a single-use challenge, bound to the given public key
func (s *AuthTokenStore) NewChallenge(pub ssh.PublicKey) (string, time.Time, error) {
	expires := time.Unix(time.Now().Add(AuthChallengeTTL).Unix(), 0)

	payload := make([]byte, challengePayloadLen)
	binary.BigEndian.PutUint64(payload, uint64(expires.Unix()))
	if _, err := rand.Read(payload[challengeExpiresLen:]); err != nil {
		return "", time.Time{}, err
	}

	raw := append(payload, s.challengeMAC(payload, pub)...)
	return challengeEncoding.EncodeToString(raw), expires, nil
}

// checkChallenge returns the challenge expiration if it was issued by us,
// for this public key, and is not expired
func (s *AuthTokenStore) checkChallenge(challenge string, pub ssh.PublicKey) (time.Time, error) {
	raw, err := challengeEncoding.DecodeString(challenge)
	if err != nil || len(raw) != challengeLen {
		return time.Time{}, ErrAuthChallenge
	}

	payload, mac := raw[:challengePayloadLen], raw[challengePayloadLen:]
	if !hmac.Equal(mac, s.challengeMAC(payload, pub)) {
		return time.Time{}, ErrAuthChallenge
	}

	expires := time.Unix(int64(binary.BigEndian.Uint64(payload)), 0)
	if time.Now().After(expires) {
		return time.Time{}, ErrAuthChallenge
	}
	return expires, nil
}

// ConsumeChallenge checks that the challenge was issued by us for this
// public key and is not expired, then calls verify (signature check). If
// verify succeeds, the challenge is marked as used, so it can't be used
// again. Returns ErrAuthChallenge or the error returned by verify.
func (s *AuthTokenStore) ConsumeChallenge(challenge string, pub ssh.PublicKey, verify func() error) error {
	expires, err := s.checkChallenge(challenge, pub)
	if err != nil {
		return err
	}

	err = verify()
	if err != nil {
		return err
	}

	// only verified challenges are stored (bounded memory), and check+mark
	// is atomic, so concurrent requests can't use the same challenge
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if _, used := s.usedChallenges[challenge]; used {
		return ErrAuthChallenge
	}
	s.purge()
	s.usedChallenges[challenge] = expires
	return nil
}

// NewToken creates a session token for the API key, authenticated
// with the given SSH public key
func (s *AuthTokenStore) NewToken(apiKey *APIKey, pub ssh.PublicKey) (string, time.Time, error) {
	random, err := randomBase64(32)
	if err != nil {
		return "", time.Time{}, err
	}
	token := AuthTokenPrefix + random

	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.purge()

	expires := time.Now().Add(AuthTokenTTL)
	s.tokens[hashToken(token)] = &authToken{
		apiKey:  apiKey,
		pubKey:  string(pub.Marshal()),
		expires: expires,
	}
	return token, expires, nil
}

// Get returns the API key and the (marshaled) SSH public key associated
// with this token, if the token is valid
func (s *AuthTokenStore) Get(token string) (*APIKey, string, bool) {
	if len(token) <= len(AuthTokenPrefix) || token[:len(AuthTokenPrefix)] != AuthTokenPrefix {
		return nil, "", false
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	id := hashToken(token)
	t, exists := s.tokens[id]
	if !exists {
		return nil, "", false
	}
	if time.Now().After(t.expires) {
		delete(s.tokens, id)
		return nil, "", false
	}
	return t.apiKey, t.pubKey, true
}
