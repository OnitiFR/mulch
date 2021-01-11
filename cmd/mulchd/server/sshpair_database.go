package server

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"golang.org/x/crypto/ssh"
)

// Mulchd SSH key pairs (generated during launch if needed)
const (
	SSHProxyPair = "mulch_ssh_proxy"
)

// SSHPair describes an OpenSSH formatted key pair
type SSHPair struct {
	Name    string
	Private string
	Public  string
}

// SSHPairDatabase describes a persistent SSHPair instances database
type SSHPairDatabase struct {
	filename string
	db       map[string]*SSHPair
}

// NewSSHPairDatabase instanciates a new SSHPairDatabase
func NewSSHPairDatabase(filename string) (*SSHPairDatabase, error) {
	db := &SSHPairDatabase{
		filename: filename,
		db:       make(map[string]*SSHPair),
	}

	// if the file exists, load it
	if _, err := os.Stat(db.filename); err == nil {
		err = db.load()
		if err != nil {
			return nil, err
		}
	}

	// save the file to check if it's writable
	err := db.Save()
	if err != nil {
		return nil, err
	}

	return db, nil
}

// Save the DB to disk
func (db *SSHPairDatabase) Save() error {
	f, err := os.OpenFile(db.filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	err = enc.Encode(&db.db)
	if err != nil {
		return err
	}
	return nil
}

func (db *SSHPairDatabase) load() error {
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
	err = dec.Decode(&db.db)
	if err != nil {
		return err
	}
	return nil
}

// AddNew and add a SSH pair
func (db *SSHPairDatabase) AddNew(name string) error {
	if _, exists := db.db[name]; exists == true {
		return fmt.Errorf("SSH Pair '%s' already exists in database", name)
	}

	private, public, err := MakeSSHKey()
	if err != nil {
		return fmt.Errorf("SSH Pair error: '%s'", err)
	}

	db.db[name] = &SSHPair{
		Name:    name,
		Private: private,
		Public:  public,
	}

	err = db.Save()
	if err != nil {
		return err
	}
	return nil
}

// GetByName lookups a SSHPair by its name, or nil if not found
func (db *SSHPairDatabase) GetByName(name string) *SSHPair {
	pair, exists := db.db[name]
	if !exists {
		return nil
	}
	return pair
}

// Count returns the number of SSHPair in the database
func (db *SSHPairDatabase) Count() int {
	return len(db.db)
}

// GetPublicKeyAuth return a PublicKey AuthMethod for named key pair
func (db *SSHPairDatabase) GetPublicKeyAuth(name string) (ssh.AuthMethod, error) {
	sshSuperUserPair := db.GetByName(name)
	if sshSuperUserPair == nil {
		return nil, fmt.Errorf("can't find '%s' key pair", name)
	}

	key, err := ssh.ParsePrivateKey([]byte(sshSuperUserPair.Private))
	if err != nil {
		return nil, fmt.Errorf("can't parse '%s' SSH key pair", name)
	}
	return ssh.PublicKeys(key), nil
}
