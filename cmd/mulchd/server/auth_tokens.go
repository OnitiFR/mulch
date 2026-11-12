package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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

// maximum number of pending challenges (flood protection), globally and
// per SSH key (so a single known public key can't block everyone)
const authMaxPendingChallenges = 10000
const authMaxPendingChallengesPerKey = 10

// AuthTokenPrefix is the prefix of session tokens (helps to recognize them)
const AuthTokenPrefix = "mulch-token-"

// ErrAuthChallenge is returned for any invalid / expired / unknown challenge
var ErrAuthChallenge = errors.New("invalid or expired challenge")

type authChallenge struct {
	pubKey  string // marshaled
	expires time.Time
}

type authToken struct {
	apiKey  *APIKey
	pubKey  string // marshaled
	expires time.Time
}

// AuthTokenStore stores pending SSH challenges and session tokens, in
// memory only (a mulchd restart invalidates every token, clients will
// transparently authenticate again)
type AuthTokenStore struct {
	challenges map[string]*authChallenge
	tokens     map[string]*authToken // key: sha256(token)
	mutex      sync.Mutex
}

// NewAuthTokenStore creates a new (empty) store
func NewAuthTokenStore() *AuthTokenStore {
	return &AuthTokenStore{
		challenges: make(map[string]*authChallenge),
		tokens:     make(map[string]*authToken),
	}
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
	for id, c := range s.challenges {
		if now.After(c.expires) {
			delete(s.challenges, id)
		}
	}
	for id, t := range s.tokens {
		if now.After(t.expires) {
			delete(s.tokens, id)
		}
	}
}

// NewChallenge creates a single-use challenge, bound to the given public key
func (s *AuthTokenStore) NewChallenge(pub ssh.PublicKey) (string, time.Time, error) {
	challenge, err := randomBase64(32)
	if err != nil {
		return "", time.Time{}, err
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.purge()
	if len(s.challenges) >= authMaxPendingChallenges {
		return "", time.Time{}, errors.New("too many pending challenges, try again later")
	}

	pubKey := string(pub.Marshal())
	count := 0
	for _, c := range s.challenges {
		if c.pubKey == pubKey {
			count++
		}
	}
	if count >= authMaxPendingChallengesPerKey {
		return "", time.Time{}, errors.New("too many pending challenges for this key, try again later")
	}

	expires := time.Now().Add(AuthChallengeTTL)
	s.challenges[challenge] = &authChallenge{
		pubKey:  pubKey,
		expires: expires,
	}
	return challenge, expires, nil
}

// ConsumeChallenge checks that the challenge exists, is not expired and
// was issued for this public key. The challenge is deleted in any case.
func (s *AuthTokenStore) ConsumeChallenge(challenge string, pub ssh.PublicKey) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	c, exists := s.challenges[challenge]
	if !exists {
		return ErrAuthChallenge
	}
	delete(s.challenges, challenge)

	if time.Now().After(c.expires) || c.pubKey != string(pub.Marshal()) {
		return ErrAuthChallenge
	}
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
