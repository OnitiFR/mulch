package server

import (
	"crypto/rsa"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/OnitiFR/mulch/common"
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

	// personal SSH public keys allowed to authenticate as this API key
	// (API, using a signed challenge, and SSH proxy)
	SSHAuthorizedKeys []APIAuthorizedSSHKey `json:",omitempty"`

	// if true, static secrets (Key and SSHPrivate/SSHPublic pair) are
	// disabled: only SSHAuthorizedKeys can be used
	SSHAuthOnly bool `json:",omitempty"`
}

// APIAuthorizedSSHKey is a personal SSH public key of the key owner
type APIAuthorizedSSHKey struct {
	PublicKey string // authorized_keys format
	AddedAt   time.Time
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
	mutex    sync.Mutex
}

// NewAPIKeyDatabase creates a new API key database
func NewAPIKeyDatabase(filename string, log *Log) (*APIKeyDatabase, error) {
	db := &APIKeyDatabase{
		filename: filename,
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

	// note: keys in SSH-only mode are returned too, the caller must check
	for _, candidate := range db.keys {
		if subtle.ConstantTimeCompare([]byte(candidate.Key), []byte(key)) == 1 {
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

func (db *APIKeyDatabase) ListEntries() common.APIKeyListEntries {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	entries := common.APIKeyListEntries{}
	for _, key := range db.keys {
		entries = append(entries, common.APIKeyListEntry{
			Comment:          key.Comment,
			RightCount:       len(key.Rights),
			FingerprintCount: len(key.SSHAllowedFingerprints),
			SSHKeyCount:      len(key.SSHAuthorizedKeys),
			SSHAuthOnly:      key.SSHAuthOnly,
		})
	}

	return entries
}

// GenKey generates a new random API key
func (db *APIKeyDatabase) genKey() (string, error) {
	return RandString(apiKeyMinLength)
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

	secret, err := db.genKey()
	if err != nil {
		return nil, err
	}

	key := &APIKey{
		Comment:    comment,
		Key:        secret,
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
		// personal keys
		if key.hasSSHAuthorizedKey(pub) {
			return key, nil
		}

		// static (generated) key pair
		if key.SSHAuthOnly {
			continue
		}

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

// GetBySSHAuthorizedKey returns the API key owning this personal SSH
// public key, or nil if not found
func (db *APIKeyDatabase) GetBySSHAuthorizedKey(pub ssh.PublicKey) *APIKey {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	marshaled := string(pub.Marshal())
	for _, key := range db.keys {
		if key.hasSSHAuthorizedKey(marshaled) {
			return key
		}
	}
	return nil
}

// GetBySSHAuthorizedKeyFingerprint returns the API key owning the personal
// SSH key with this SHA256 fingerprint, or nil if not found
func (db *APIKeyDatabase) GetBySSHAuthorizedKeyFingerprint(fingerprint string) *APIKey {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	for _, key := range db.keys {
		for _, authKey := range key.SSHAuthorizedKeys {
			pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(authKey.PublicKey))
			if err == nil && ssh.FingerprintSHA256(pub) == fingerprint {
				return key
			}
		}
	}
	return nil
}

// APISSHFingerprintTrust is a VM an SSH key is forwarded to, by an API key
type APISSHFingerprintTrust struct {
	APIKey *APIKey
	VMName string
}

// GetTrustsByFingerprint returns every VM this SSH key fingerprint is
// forwarded to ('trust forward'), for all API keys
func (db *APIKeyDatabase) GetTrustsByFingerprint(fingerprint string) []APISSHFingerprintTrust {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	var res []APISSHFingerprintTrust
	for _, key := range db.keys {
		for _, fp := range key.SSHAllowedFingerprints {
			if fp.Fingerprint == fingerprint {
				res = append(res, APISSHFingerprintTrust{APIKey: key, VMName: fp.VMName})
			}
		}
	}
	return res
}

// IsSSHKeyStillAuthorized returns true if the key still exists in the
// database and still authorizes the (marshaled) SSH public key
func (db *APIKeyDatabase) IsSSHKeyStillAuthorized(key *APIKey, pub string) bool {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	return db.keyExists(key) && key.hasSSHAuthorizedKey(pub)
}

// hasSSHAuthorizedKey returns true if the (marshaled) public key is
// one of the personal keys of this API key (no locking)
func (key *APIKey) hasSSHAuthorizedKey(pub string) bool {
	for _, authKey := range key.SSHAuthorizedKeys {
		pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(authKey.PublicKey))
		if err != nil {
			continue
		}
		if string(pubKey.Marshal()) == pub {
			return true
		}
	}
	return false
}

// CheckSSHAuthKeyType returns an error if this SSH public key type is
// not accepted for authentication
func CheckSSHAuthKeyType(pub ssh.PublicKey) error {
	switch pub.Type() {
	case ssh.KeyAlgoED25519, ssh.KeyAlgoSKED25519,
		ssh.KeyAlgoECDSA256, ssh.KeyAlgoECDSA384, ssh.KeyAlgoECDSA521,
		ssh.KeyAlgoSKECDSA256:
		return nil
	case ssh.KeyAlgoRSA:
		cryptoPub, ok := pub.(ssh.CryptoPublicKey)
		if !ok {
			return errors.New("unable to read RSA key")
		}
		rsaPub, ok := cryptoPub.CryptoPublicKey().(*rsa.PublicKey)
		if !ok {
			return errors.New("unable to read RSA key")
		}
		if rsaPub.N.BitLen() < 2048 {
			return fmt.Errorf("RSA key is too short (%d bits, 2048 minimum)", rsaPub.N.BitLen())
		}
		return nil
	default:
		return fmt.Errorf("unsupported key type '%s'", pub.Type())
	}
}

// AddSSHAuthorizedKey adds a personal SSH public key (authorized_keys
// format) to the API key
func (db *APIKeyDatabase) AddSSHAuthorizedKey(key *APIKey, authorizedKey string) (ssh.PublicKey, error) {
	pub, comment, _, _, err := ssh.ParseAuthorizedKey([]byte(authorizedKey))
	if err != nil {
		return nil, fmt.Errorf("invalid public key: %s", err)
	}

	err = CheckSSHAuthKeyType(pub)
	if err != nil {
		return nil, err
	}

	db.mutex.Lock()
	defer db.mutex.Unlock()

	if !db.keyExists(key) {
		return nil, errors.New("key not found in database")
	}

	marshaled := string(pub.Marshal())
	for _, other := range db.keys {
		if other.hasSSHAuthorizedKey(marshaled) {
			if other == key {
				return nil, errors.New("this SSH key is already authorized for this API key")
			}
			return nil, errors.New("this SSH key is already used by another API key")
		}
	}

	// normalize (type, key, comment), dropping any authorized_keys options
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub)))
	if comment != "" {
		line += " " + comment
	}

	key.SSHAuthorizedKeys = append(key.SSHAuthorizedKeys, APIAuthorizedSSHKey{
		PublicKey: line,
		AddedAt:   time.Now(),
	})

	err = db.save()
	if err != nil {
		return nil, err
	}
	return pub, nil
}

// RemoveSSHAuthorizedKey removes a personal SSH public key, using its
// SHA256 fingerprint
func (db *APIKeyDatabase) RemoveSSHAuthorizedKey(key *APIKey, fingerprint string) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if !db.keyExists(key) {
		return errors.New("key not found in database")
	}

	newList := make([]APIAuthorizedSSHKey, 0, len(key.SSHAuthorizedKeys))
	for _, authKey := range key.SSHAuthorizedKeys {
		pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(authKey.PublicKey))
		if err == nil && ssh.FingerprintSHA256(pub) == fingerprint {
			continue
		}
		newList = append(newList, authKey)
	}

	if len(newList) == len(key.SSHAuthorizedKeys) {
		return fmt.Errorf("SSH key '%s' not found", fingerprint)
	}

	if len(newList) == 0 && key.SSHAuthOnly {
		return errors.New("cannot remove the last SSH key of a key in SSH-only mode (disable SSH-only mode first)")
	}

	key.SSHAuthorizedKeys = newList
	return db.save()
}

// SetSSHAuthOnly enables or disables SSH-only mode for this key
func (db *APIKeyDatabase) SetSSHAuthOnly(key *APIKey, enable bool) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if !db.keyExists(key) {
		return errors.New("key not found in database")
	}

	if enable && len(key.SSHAuthorizedKeys) == 0 {
		return errors.New("this key has no SSH key, add one first")
	}

	key.SSHAuthOnly = enable
	return db.save()
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

// parseRight will parse a right string
// (see APIRight.String() form informations about the format)
func parseRight(rightStr string) (*APIRight, error) {
	spaces := regexp.MustCompile(`\s+`)

	rightStr = strings.TrimSpace(rightStr)
	rightStr = spaces.ReplaceAllString(rightStr, " ")

	parts := strings.Split(rightStr, " ")

	if len(parts) < 2 {
		return nil, errors.New("need at least method and path")
	}

	method := strings.ToUpper(strings.TrimSpace(parts[0]))
	path := strings.TrimSpace(parts[1])

	switch method {
	case "GET", "POST", "PUT", "DELETE", "SSH", "CREATE", "*":
	default:
		return nil, fmt.Errorf("'%s' is an unsupported method", method)
	}

	if len(path) < 1 || (path[0] != '/' && path[0] != '*') {
		return nil, fmt.Errorf("'%s' is not a valid path", path)
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
			return nil, fmt.Errorf("invalid header format '%s'", header)
		}
		name := strings.TrimSpace(hParts[0])
		value := strings.TrimSpace(hParts[1])

		if name == "" {
			return nil, fmt.Errorf("invalid header name in '%s'", header)
		}

		right.Headers[name] = value
	}

	return &right, nil
}

// hasRight returns true if the key already has this exact right
// (very basic duplication check)
func (key *APIKey) hasRight(right *APIRight) bool {
	rs := right.String()
	for _, r := range key.Rights {
		if r.String() == rs {
			return true
		}
	}
	return false
}

// AddNewRight parse + add the right to the key
// WARNING: you may have to save the APIKeyDatabase to the disk!
func (key *APIKey) AddNewRight(rightStr string) error {
	right, err := parseRight(rightStr)
	if err != nil {
		return err
	}

	if key.hasRight(right) {
		return fmt.Errorf("right '%s' is duplicated", right.String())
	}

	key.Rights = append(key.Rights, *right)

	return nil
}

// AddNewRights parse + add multiple rights to the key, atomically: if any
// right is invalid, nothing is added. Duplicates are skipped and returned.
// WARNING: you may have to save the APIKeyDatabase to the disk!
func (key *APIKey) AddNewRights(rightStrs []string) (added int, duplicates []string, err error) {
	var rights []APIRight
	seen := make(map[string]bool)

	for _, rightStr := range rightStrs {
		right, err := parseRight(rightStr)
		if err != nil {
			return 0, nil, fmt.Errorf("'%s': %s", strings.TrimSpace(rightStr), err)
		}

		rs := right.String()
		if seen[rs] || key.hasRight(right) {
			duplicates = append(duplicates, rs)
			continue
		}
		seen[rs] = true
		rights = append(rights, *right)
	}

	key.Rights = append(key.Rights, rights...)

	return len(rights), duplicates, nil
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
	// stable output (map order is random), needed for comparisons
	sort.Strings(headers)
	if len(headers) > 0 {
		str = str + " " + strings.Join(headers, " ")
	}
	return str
}
