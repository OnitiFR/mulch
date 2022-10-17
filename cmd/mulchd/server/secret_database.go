package server

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

type SecretDatabase struct {
	dbFilename   string
	db           map[string]*Secret
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
}

// NewSecretDatabase instanciates a new SecretDatabase, creating a new
// passphrase if needed.
func NewSecretDatabase(dbFilename string, passFilename string, app *App) (*SecretDatabase, error) {
	db := &SecretDatabase{
		dbFilename:   dbFilename,
		db:           make(map[string]*Secret),
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

// Set a secret value
func (db *SecretDatabase) Set(key string, value string, authorKey string) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	db.db[key] = &Secret{
		Key:       key,
		Value:     value,
		Modified:  time.Now(),
		AuthorKey: authorKey,
	}

	return db.save()
}

// Get a secret value
func (db *SecretDatabase) Get(key string) (*Secret, error) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	secret, exists := db.db[key]
	if !exists {
		return nil, fmt.Errorf("secret '%s' not found", key)
	}

	return secret, nil
}

// Delete a secret value
func (db *SecretDatabase) Delete(key string) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	_, exists := db.db[key]
	if !exists {
		return fmt.Errorf("secret '%s' not found", key)
	}

	delete(db.db, key)

	return db.save()
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
	for key := range db.db {
		keys = append(keys, key)
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

	return db.saveToWriter(f)
}

// save the database to a writer (without a mutex lock)
func (db *SecretDatabase) saveToWriter(writer io.Writer) error {

	buf, err := json.Marshal(db.db)
	if err != nil {
		return err
	}

	// encrypt the JSON data
	data, err := db.encrypt(buf)
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

	// TODO: decode data with the passphrase

	// read file in a buffer
	buf, err := io.ReadAll(f)
	if err != nil {
		return err
	}

	// decrypt the JSON data
	data, err := db.decrypt(buf)
	if err != nil {
		return err
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
func (db *SecretDatabase) encrypt(data []byte) ([]byte, error) {
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
func (db *SecretDatabase) decrypt(data []byte) ([]byte, error) {
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
