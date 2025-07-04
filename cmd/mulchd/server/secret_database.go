package server

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/OnitiFR/mulch/common"
)

type SecretDatabaseEntries map[string]*Secret

type SecretDatabase struct {
	dbFilename   string
	db           SecretDatabaseEntries
	passphrase   []byte
	passFilename string
	mutex        sync.Mutex
	app          *App
}

type Secret struct {
	Key       string
	Value     string
	Modified  time.Time
	AuthorKey string
	Deleted   bool
}

// NewSecretDatabase instanciates a new SecretDatabase, creating a new
// passphrase if needed.
func NewSecretDatabase(dbFilename string, passFilename string, app *App) (*SecretDatabase, error) {
	db := &SecretDatabase{
		dbFilename:   dbFilename,
		db:           make(SecretDatabaseEntries),
		passFilename: passFilename,
		app:          app,
	}

	// if the passphrase file exists, load it
	if _, err := os.Stat(db.passFilename); err == nil {
		err = db.loadPassphrase()
		if err != nil {
			return nil, err
		}
	} else {
		err = db.generatePassphrase()
		if err != nil {
			return nil, err
		}

		err = db.savePassphrase()
		if err != nil {
			return nil, err
		}

		app.Log.Warningf("generated new passphrase for secret database, you should backup %s content", db.passFilename)
	}

	// if the db file exists, load it
	if _, err := os.Stat(db.dbFilename); err == nil {
		err = db.load()
		if err != nil {
			return nil, err
		}
	}

	// save the file to check if it's writable
	err := db.save()
	if err != nil {
		return nil, err
	}

	return db, nil
}

// Get a secret value
func (db *SecretDatabase) Get(key string) (*Secret, error) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	secret, exists := db.db[key]
	if !exists || secret.Deleted {
		return nil, fmt.Errorf("secret '%s' not found", key)
	}

	return secret, nil
}

// set a secret value (low-level)
func (db *SecretDatabase) set(key string, value string, authorKey string) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	db.db[key] = &Secret{
		Key:       key,
		Value:     value,
		Modified:  time.Now(),
		AuthorKey: authorKey,
		Deleted:   false,
	}
}

// Set a secret value
func (db *SecretDatabase) Set(key string, value string, authorKey string) error {

	db.set(key, value, authorKey)

	err := db.Save()
	if err != nil {
		return err
	}

	if err = db.SyncPeers(); err != nil {
		db.app.Log.Error(err.Error())
	}

	return nil
}

// delete a secret (low-level)
func (db *SecretDatabase) delete(key string, authorKey string) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	secret, exists := db.db[key]
	if !exists || secret.Deleted {
		return fmt.Errorf("secret '%s' not found", key)
	}

	secret.Value = ""
	secret.Deleted = true
	secret.Modified = time.Now()
	secret.AuthorKey = authorKey

	return nil
}

// Delete a secret value
func (db *SecretDatabase) Delete(key string, authorKey string) error {
	err := db.delete(key, authorKey)
	if err != nil {
		return err
	}

	err = db.Save()
	if err != nil {
		return err
	}

	if err = db.SyncPeers(); err != nil {
		db.app.Log.Error(err.Error())
	}

	return nil
}

// CleanKey returns a cleaned key path, if possible
func (db *SecretDatabase) CleanKey(keyPath string) (string, error) {
	resPath := strings.TrimSpace(keyPath)

	if resPath == "" {
		return "", errors.New("empty key")
	}

	// remove leading slash
	if resPath[0] == '/' {
		resPath = resPath[1:]
	}

	resPath = path.Clean(resPath)

	parts := strings.Split(resPath, "/")
	for _, part := range parts {
		if !IsValidName(part) {
			return "", fmt.Errorf("invalid path part: %s", part)
		}
	}

	// extract environment variable name
	env := parts[len(parts)-1]

	if env == "" {
		return "", errors.New("empty environment name")
	}

	if env != strings.ToUpper(env) {
		return "", fmt.Errorf("environment name should be upper case: %s", env)
	}

	return resPath, nil
}

// GetKeys returns all keys
func (db *SecretDatabase) GetKeys() []string {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	keys := make([]string, 0, len(db.db))
	for key, secret := range db.db {
		if !secret.Deleted {
			keys = append(keys, key)
		}
	}

	return keys
}

// Save the database to disk
func (db *SecretDatabase) Save() error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	return db.save()
}

// Save the database to a writer
func (db *SecretDatabase) SaveToWriter(writer io.Writer) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	return db.saveToWriter(writer)
}

// save the database to disk (without a mutex lock)
func (db *SecretDatabase) save() error {
	f, err := os.OpenFile(db.dbFilename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	err = db.saveToWriter(f)
	f.Sync()

	return err
}

// save the database to a writer (without a mutex lock)
func (db *SecretDatabase) saveToWriter(writer io.Writer) error {

	buf, err := json.Marshal(db.db)
	if err != nil {
		return err
	}

	// encrypt the JSON data
	data, err := db.Encrypt(buf)
	if err != nil {
		return err
	}

	_, err = writer.Write(data)
	if err != nil {
		return err
	}

	return nil
}

func (db *SecretDatabase) load() error {
	f, err := os.Open(db.dbFilename)
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
		return fmt.Errorf("%s: only the owner should be able to read/write this file (mode 0600)", db.dbFilename)
	}

	// read file in a buffer
	buf, err := io.ReadAll(f)
	if err != nil {
		return err
	}

	// decrypt the JSON data
	data, err := db.Decrypt(buf)
	if err != nil {
		return fmt.Errorf("failed to decrypt database %s: %s, check that the secret key is correct (%s)", db.dbFilename, err, db.passFilename)
	}

	err = json.Unmarshal(data, &db.db)
	if err != nil {
		return err
	}

	return nil
}

func (db *SecretDatabase) loadPassphrase() error {
	f, err := os.Open(db.passFilename)
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
		return fmt.Errorf("%s: only the owner should be able to read/write this file (mode 0600)", db.passFilename)
	}

	// read file content as string
	b64, err := io.ReadAll(f)
	if err != nil {
		return err
	}

	// decode base64
	passphrase, err := base64.StdEncoding.DecodeString(string(b64))
	if err != nil {
		return err
	}

	db.passphrase = passphrase

	return nil
}

func (db *SecretDatabase) savePassphrase() error {
	f, err := os.OpenFile(db.passFilename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	str := base64.StdEncoding.EncodeToString(db.passphrase)

	_, err = f.Write([]byte(str))
	if err != nil {
		return err
	}

	return nil
}

func (db *SecretDatabase) generatePassphrase() error {
	passphrase := make([]byte, 32)

	_, err := db.app.Rand.Read(passphrase)
	if err != nil {
		return err
	}

	db.passphrase = passphrase
	return nil
}

// encrypt data with the passphrase using AES and GCM
func (db *SecretDatabase) Encrypt(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(db.passphrase)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}

	encrypted := gcm.Seal(nonce, nonce, data, nil)

	return encrypted, nil
}

// decrypt data with the passphrase using AES and GCM
func (db *SecretDatabase) Decrypt(data []byte) ([]byte, error) {
	bloc, err := aes.NewCipher(db.passphrase)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(bloc)
	if err != nil {
		return nil, err
	}

	nonce, text := data[:gcm.NonceSize()], data[gcm.NonceSize():]

	decrypted, err := gcm.Open(nil, nonce, text, nil)
	if err != nil {
		return nil, err
	}

	return decrypted, nil
}

// SyncPeers syncs the secret database with peers
func (db *SecretDatabase) SyncPeers() error {
	errors := make([]string, 0)

	for _, peer := range db.app.Config.Peers {
		if !peer.SyncSecrets {
			continue
		}

		err := db.SyncPeer(peer)
		if err != nil {
			str := fmt.Sprintf("peer %s: %s", peer.Name, err)
			errors = append(errors, str)
		}
	}

	if len(errors) > 0 {
		msg := strings.Join(errors, ", ")
		return fmt.Errorf("failed to sync secrets: %s", msg)
	}

	return nil
}

// SyncPeer syncs the secret database with a peer
func (db *SecretDatabase) SyncPeer(peer ConfigPeer) error {
	db.app.Log.Tracef("syncing secrets with peer %s", peer.Name)

	// get our db as a JSON string
	buf, err := json.Marshal(db.db)
	if err != nil {
		return err
	}

	// encrypt the JSON data
	data, err := db.Encrypt(buf)
	if err != nil {
		return err
	}

	call := &PeerCall{
		Peer:   peer,
		Method: "POST",
		Path:   "/secret-sync",
		Args:   map[string]string{},
		UploadString: &PeerCallStringFile{
			FieldName: "db",
			FileName:  "db.bin",
			Content:   string(data),
		},
		Log: db.app.Log,
		BinaryCallback: func(reader io.Reader, _ http.Header) error {
			// read the response
			buf, err := io.ReadAll(reader)
			if err != nil {
				return err
			}

			// decrypt the JSON data
			data, err := db.Decrypt(buf)
			if err != nil {
				return err
			}

			// unmarshal the JSON data
			newer := make(SecretDatabaseEntries)
			err = json.Unmarshal(data, &newer)
			if err != nil {
				return err
			}

			// the decrypted response is a JSON string including all newer entries
			// from the remote peer

			// merge the newer entries into our database
			_, err = db.SyncWithDatabase(newer)
			if err != nil {
				return err
			}

			return nil
		},
	}
	err = call.Do()
	if err != nil {
		return err
	}

	return nil
}

// SyncWithDatabase syncs our secret database with another database (ex: from another peer)
// It returns (our) "newer" entries so the remote peer can merge them into its own database.
func (db *SecretDatabase) SyncWithDatabase(other SecretDatabaseEntries) (SecretDatabaseEntries, error) {
	db.app.Log.Tracef("syncing with database, %d entries in", len(other))

	db.mutex.Lock()
	defer db.mutex.Unlock()

	// build a map of our existing keys
	responseKeys := make(map[string]bool)
	for key := range db.db {
		responseKeys[key] = true
	}

	for _, entry := range other {
		my, exists := db.db[entry.Key]
		if !exists || entry.Modified.After(my.Modified) {
			db.db[entry.Key] = entry
			delete(responseKeys, entry.Key)
		}

		if exists && !my.Modified.After(entry.Modified) {
			delete(responseKeys, entry.Key)
		}
	}

	newer := make(SecretDatabaseEntries)

	// add all entries that were not in the other database, or that were older.
	for key := range responseKeys {
		newer[key] = db.db[key]
	}

	db.app.Log.Tracef("syncing with database, %d entries out", len(newer))

	return newer, db.save()
}

// GetVMsUsingSecret returns a list of VMs that use a given secret,
// including other peers.
func (db *SecretDatabase) GetVMsUsingSecret(key string) ([]string, error) {
	res := make([]string, 0)

	vmNames := db.app.VMDB.GetNames()
	for _, vmName := range vmNames {
		vm, err := db.app.VMDB.GetByName(vmName)
		if err != nil {
			return nil, fmt.Errorf("VM '%s': %s", vmName, err)
		}
		for _, secret := range vm.Config.Secrets {
			if secret == key {
				res = append(res, vmName.ID())
			}
		}
	}

	return res, nil
}

// GetPeersVMsUsingSecret returns a list of VMs that use a given secret on
// all our peers.
func (db *SecretDatabase) GetPeersVMsUsingSecret(key string) ([]string, error) {
	res := make([]string, 0)

	for _, peer := range db.app.Config.Peers {
		if !peer.SyncSecrets {
			continue
		}

		call := &PeerCall{
			Peer:   peer,
			Method: "GET",
			Path:   "/vm/with-secret/" + key,
			Args:   map[string]string{},
			Log:    db.app.Log,
			JSONCallback: func(reader io.Reader, _ http.Header) error {
				var vms []string
				dec := json.NewDecoder(reader)
				err := dec.Decode(&vms)
				if err != nil {
					return err
				}

				for _, vm := range vms {
					res = append(res, vm+"@"+peer.Name)
				}

				return nil
			},
		}

		err := call.Do()
		if err != nil {
			return nil, err
		}

	}

	return res, nil
}

// GetAllVMsUsingSecret returns a list of VMs that use a given secret,
// including on other peers.
func (db *SecretDatabase) GetAllVMsUsingSecret(key string) ([]string, error) {
	res, err := db.GetVMsUsingSecret(key)
	if err != nil {
		return nil, err
	}

	peers, err := db.GetPeersVMsUsingSecret(key)
	if err != nil {
		return nil, err
	}

	res = append(res, peers...)

	return res, nil
}

// GetSecretsUsage returns a list of secrets and the number of VMs using them
func (db *SecretDatabase) GetSecretsUsage(with_peers bool) (common.APISecretUsageEntries, error) {
	// create a temporary map of secrets, faster for lookups
	secrets := make(map[string]*common.APISecretUsageEntry)

	// get our secrets
	keys := db.GetKeys()
	for _, key := range keys {
		secret, err := db.Get(key)
		if err != nil {
			return nil, err
		}

		vms, err := db.GetVMsUsingSecret(key)
		if err != nil {
			return nil, err
		}

		entry := common.APISecretUsageEntry{
			Key:         secret.Key,
			LocalCount:  len(vms),
			RemoteCount: 0,
		}

		secrets[key] = &entry
	}

	// get peers secrets
	if with_peers {
		for _, peer := range db.app.Config.Peers {
			if !peer.SyncSecrets {
				continue
			}

			call := &PeerCall{
				Peer:   peer,
				Method: "GET",
				Path:   "/secret-usage",
				Args:   map[string]string{},
				Log:    db.app.Log,
				JSONCallback: func(reader io.Reader, _ http.Header) error {
					var remoteSecrets common.APISecretUsageEntries
					dec := json.NewDecoder(reader)
					err := dec.Decode(&remoteSecrets)
					if err != nil {
						return err
					}

					for _, entry := range remoteSecrets {
						_, exists := secrets[entry.Key]
						if !exists {
							secrets[entry.Key] = &entry
						} else {
							secrets[entry.Key].RemoteCount += entry.LocalCount
						}
					}

					return nil
				},
			}

			err := call.Do()
			if err != nil {
				return nil, err
			}
		}
	}

	// convert the map to a slice
	res := make(common.APISecretUsageEntries, 0, len(secrets))
	for _, entry := range secrets {
		res = append(res, *entry)
	}

	return res, nil
}

// GetStats returns statistics about the secrets
func (db *SecretDatabase) GetStats() (common.APISecretStats, error) {
	stats := common.APISecretStats{}

	db.mutex.Lock()
	defer db.mutex.Unlock()

	// get db file size
	stat, err := os.Stat(db.dbFilename)
	if err != nil {
		return stats, err
	}
	stats.FileSize = stat.Size()

	for _, secret := range db.db {
		if secret.Deleted {
			stats.TrashCount++
		} else {
			stats.ActiveCount++
		}
	}

	return stats, nil
}
