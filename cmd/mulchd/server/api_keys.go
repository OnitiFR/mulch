package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ryanuber/go-glob"
	"golang.org/x/crypto/ssh"
)

const apiKeyMinLength = 64

// APIKey describes an API key
type APIKey struct {
	Comment                string
	Key                    string
	SSHPrivate             string
	SSHPublic              string
	Rights                 []APIRight
	SSHAllowedFingerprints []APISSHFingerprint
}

// APIRight is a parsed "Rights" line
type APIRight struct {
	Method  string
	Path    string
	Headers map[string]string
}

// APISSHFingerprint is a fingerprint the user allows to be forwarded to a VM
type APISSHFingerprint struct {
	VMName      string
	Fingerprint string
	AddedAt     time.Time
}

// APIKeyDatabase describes a persistent API Key database
type APIKeyDatabase struct {
	filename string
	keys     []*APIKey
	rand     *rand.Rand
	mutex    sync.Mutex
}

// NewAPIKeyDatabase creates a new API key database
func NewAPIKeyDatabase(filename string, log *Log, rand *rand.Rand) (*APIKeyDatabase, error) {
	db := &APIKeyDatabase{
		filename: filename,
		rand:     rand,
	}

	// if the file exists, load it
	if _, err := os.Stat(db.filename); err == nil {
		err = db.load(log)
		if err != nil {
			return nil, err
		}
	} else {
		log.Warningf("no API keys database found, creating a new one with a default key")
		key, err := db.AddNew("default-key")
		if err != nil {
			return nil, err
		}
		log.Infof("key = %s", key.Key)
	}

	// save the file to check if it's writable
	err := db.save()
	if err != nil {
		return nil, err
	}

	return db, nil
}

func (db *APIKeyDatabase) load(log *Log) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	f, err := os.Open(db.filename)
	if err != nil {
		return err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return err
	}

	requiredMode, err := strconv.ParseInt("0600", 8, 32)
	if err != nil {
		return err
	}

	if stat.Mode() != os.FileMode(requiredMode) {
		return fmt.Errorf("%s: only the owner should be able to read/write this file (mode 0600)", db.filename)
	}

	dec := json.NewDecoder(f)
	err = dec.Decode(&db.keys)
	if err != nil {
		return fmt.Errorf("decoding %s: %s", db.filename, err)
	}

	log.Infof("found %d API key(s) in database %s", len(db.keys), db.filename)

	for _, key := range db.keys {
		if len(key.Key) < apiKeyMinLength {
			log.Warningf("API key '%s' is too short, disabling it (minimum length: %d)", key.Comment, apiKeyMinLength)
			key.Key = "INVALID"
			continue
		}
		if strings.HasPrefix(key.SSHPublic, "ssh-rsa") {
			log.Warningf("migrate %s API key from RSA to ED25519", key.Comment)
			priv, pub, err := MakeSSHKey()
			if err != nil {
				return err
			}
			key.SSHPrivate = priv
			key.SSHPublic = pub
		}
	}

	return nil
}

// Save the database on the disk
func (db *APIKeyDatabase) Save() error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	return db.save()
}

// Save the database on the disk, without locking (internal use)
func (db *APIKeyDatabase) save() error {
	f, err := os.OpenFile(db.filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	err = enc.Encode(&db.keys)
	if err != nil {
		return err
	}
	return nil
}

// IsValidKey return true if the key exists in the database
// (and returns the key as the second return value)
func (db *APIKeyDatabase) IsValidKey(key string) (bool, *APIKey) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if len(key) < apiKeyMinLength {
		return false, nil
	}

	for _, candidate := range db.keys {
		if candidate.Key == key {
			return true, candidate
		}
	}
	return false, nil
}

// keyExists return true if the key address exists in the database
func (db *APIKeyDatabase) keyExists(key *APIKey) bool {
	for _, candidate := range db.keys {
		if candidate == key {
			return true
		}
	}
	return false
}

// List returns all keys (comments only)
func (db *APIKeyDatabase) ListComments() []string {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	var comments []string
	for _, key := range db.keys {
		comments = append(comments, key.Comment)
	}

	return comments
}

// GenKey generates a new random API key
func (db *APIKeyDatabase) genKey() string {
	return RandString(apiKeyMinLength, db.rand)
}

// AddNew generates a new key and adds it to the database
func (db *APIKeyDatabase) AddNew(comment string) (*APIKey, error) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	for _, key := range db.keys {
		if key.Comment == comment {
			return nil, fmt.Errorf("duplicated comment in database: '%s'", comment)
		}
	}

	priv, pub, err := MakeSSHKey()
	if err != nil {
		return nil, err
	}

	key := &APIKey{
		Comment:    comment,
		Key:        db.genKey(),
		SSHPrivate: priv,
		SSHPublic:  pub,
	}
	db.keys = append(db.keys, key)

	err = db.save()
	if err != nil {
		return nil, err
	}

	return key, nil
}

// Delete removes a key from the database
func (db *APIKeyDatabase) Delete(comment string) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	found := false
	for i, key := range db.keys {
		fmt.Println(key.Comment)
		if key.Comment == comment {
			db.keys = append(db.keys[:i], db.keys[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return errors.New("key not found in database")
	}

	err := db.save()
	return err
}

// GetByPubKey returns an API key by its (marshaled) public key
// Returns nil and no error when key was not found
func (db *APIKeyDatabase) GetByPubKey(pub string) (*APIKey, error) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	for _, key := range db.keys {
		pubKey, _, _, _, errP := ssh.ParseAuthorizedKey([]byte(key.SSHPublic))
		if errP != nil {
			return nil, errP
		}

		if string(pubKey.Marshal()) == pub {
			return key, nil
		}
	}

	return nil, nil
}

// GetByComment returns an API key by its comment, or nil if not found
func (db *APIKeyDatabase) GetByComment(comment string) *APIKey {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	for _, key := range db.keys {
		if key.Comment == comment {
			return key
		}
	}
	return nil
}

// AllowSSHFingerprint allows a fingertprint to be forwarded to a VM
func (db *APIKeyDatabase) AllowSSHFingerprint(key *APIKey, fp APISSHFingerprint) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if !db.keyExists(key) {
		return errors.New("key not found in database")
	}

	if key.IsTrustedFingerprint(fp) {
		return errors.New("fingerprint already trusted")
	}

	fp.AddedAt = time.Now()
	key.SSHAllowedFingerprints = append(key.SSHAllowedFingerprints, fp)

	err := db.save()
	if err != nil {
		return err
	}

	return nil
}

// RemoveSSHFingerprint removes a fingerprint from the list of allowed fingerprints
func (db *APIKeyDatabase) RemoveSSHFingerprint(key *APIKey, fp APISSHFingerprint) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if !db.keyExists(key) {
		return errors.New("key not found in database")
	}

	found := false
	for i, f := range key.SSHAllowedFingerprints {
		if f.VMName == fp.VMName && f.Fingerprint == fp.Fingerprint {
			key.SSHAllowedFingerprints = append(key.SSHAllowedFingerprints[:i], key.SSHAllowedFingerprints[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return errors.New("fingerprint not found in the list of allowed fingerprints")
	}

	err := db.save()
	if err != nil {
		return err
	}

	return nil
}

// RemoveAllSSHFingerprint removes all fingerprints allowed for a VM of a key
func (db *APIKeyDatabase) RemoveAllSSHFingerprint(key *APIKey, vmName string) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	res := make([]APISSHFingerprint, 0)

	found := false
	for _, k := range key.SSHAllowedFingerprints {
		if k.VMName == vmName {
			found = true
			continue
		}
		res = append(res, k)
	}

	if !found {
		return errors.New("this VM has no allowed fingerprints")
	}

	key.SSHAllowedFingerprints = res

	err := db.save()
	if err != nil {
		return err
	}

	return nil
}

// IsTrustedFingerprint returns true if the fingerprint allowed to be forwarded to a VM
func (key *APIKey) IsTrustedFingerprint(fp APISSHFingerprint) bool {
	for _, v := range key.SSHAllowedFingerprints {
		if v.VMName == fp.VMName && v.Fingerprint == fp.Fingerprint {
			return true
		}
	}
	return false
}

// IsTrustedVM returns true if the VM has any allowed fingerprint
func (key *APIKey) IsTrustedVM(vmName string) bool {
	for _, fp := range key.SSHAllowedFingerprints {
		if fp.VMName == vmName {
			return true
		}
	}
	return false
}

// IsAllowed will return true if the APIKey is allowed to request this method/path/headers
// (req is optional, but will deny the access if the needed right requires some headers)
func (key *APIKey) IsAllowed(method string, path string, req *http.Request) bool {
	if len(key.Rights) == 0 {
		// no restrictions for this key
		return true
	}

	for _, right := range key.Rights {
		// wrong method?
		if !glob.Glob(right.Method, method) {
			continue
		}

		// wrong path?
		if !glob.Glob(right.Path, path) {
			continue
		}

		// need to check headers?
		if req != nil {
			headersOK := true
			for name, expr := range right.Headers {
				val := req.FormValue(name)
				if !glob.Glob(expr, val) {
					headersOK = false
					break
				}
			}

			if !headersOK {
				// at least on header failed
				continue
			}
		} else {
			if len(right.Headers) != 0 {
				// headers needed but no request provided: denied
				continue
			}
		}

		return true
	}

	return false
}

// AddNewRight parse + add the right to the key
// WARNING: you may have to save the APIKeyDatabase to the disk!
// (see APIRight.String() form informations about the format)
func (key *APIKey) AddNewRight(rightStr string) error {
	spaces := regexp.MustCompile(`\s+`)

	rightStr = strings.TrimSpace(rightStr)
	rightStr = spaces.ReplaceAllString(rightStr, " ")

	parts := strings.Split(rightStr, " ")

	if len(parts) < 2 {
		return errors.New("need at least method and path")
	}

	method := strings.ToUpper(strings.TrimSpace(parts[0]))
	path := strings.TrimSpace(parts[1])

	switch method {
	case "GET", "POST", "PUT", "DELETE", "SSH", "*":
	default:
		return fmt.Errorf("'%s' is an unsupported method", method)
	}

	if len(path) < 1 || (path[0] != '/' && path[1] != '*') {
		return fmt.Errorf("'%s' is not a valid path", path)
	}

	right := APIRight{
		Method:  method,
		Path:    path,
		Headers: make(map[string]string),
	}

	headers := parts[2:]
	for _, header := range headers {
		header = strings.TrimSpace(header)
		hParts := strings.Split(header, "=")
		if len(hParts) != 2 {
			return fmt.Errorf("invalid header format '%s'", header)
		}
		name := strings.TrimSpace(hParts[0])
		value := strings.TrimSpace(hParts[1])

		if name == "" {
			return fmt.Errorf("invalid header name in '%s'", header)
		}

		right.Headers[name] = value
	}

	// very basic duplication check
	rs := right.String()
	for _, r := range key.Rights {
		if r.String() == rs {
			return fmt.Errorf("right '%s' is duplicated", rs)
		}
	}

	key.Rights = append(key.Rights, right)

	return nil
}

// RemoveRight will remove the parsed right from the key
func (key *APIKey) RemoveRight(rightStr string) error {
	// TODO: clean the provided right with "parse + .String()"
	spaces := regexp.MustCompile(`\s+`)
	rightStr = strings.TrimSpace(rightStr)
	rightStr = spaces.ReplaceAllString(rightStr, " ")

	for i, right := range key.Rights {
		if right.String() == rightStr {
			key.Rights = append(key.Rights[:i], key.Rights[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("right '%s' not found in this key", rightStr)
}

// String will convert a right to a string
func (right *APIRight) String() string {
	var str string

	str = right.Method + " " + right.Path

	var headers []string
	for header, value := range right.Headers {
		h := header + "=" + value
		headers = append(headers, h)
	}
	if len(headers) > 0 {
		str = str + " " + strings.Join(headers, " ")
	}
	return str
}
