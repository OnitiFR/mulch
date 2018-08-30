package server

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strconv"
)

// TODO: lock this database with a mutex?

const apiKeyMinLength = 64

// APIKey describes an API key
type APIKey struct {
	Comment string
	Key     string
}

// APIKeyDatabase describes a persistent API Key database
type APIKeyDatabase struct {
	filename string
	keys     []*APIKey
}

// NewAPIKeyDatabase creates a new API key database
func NewAPIKeyDatabase(filename string, log *Log, rand *rand.Rand) (*APIKeyDatabase, error) {
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
		log.Warningf("no API keys database found, creating a new one with a default key (%s)", filename)
		db.keys = []*APIKey{
			&APIKey{
				Comment: "default-key",
				Key:     RandString(64, rand),
			},
		}
	}

	// save the file to check if it's writable
	err := db.Save()
	if err != nil {
		return nil, err
	}

	return db, nil
}

func (db *APIKeyDatabase) load(log *Log) error {
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
		return err
	}

	log.Infof("found %d API key(s) in database %s", len(db.keys), db.filename)

	for _, key := range db.keys {
		if len(key.Key) < apiKeyMinLength {
			log.Warningf("API key '%s' is too short, disabling it (minimum length: %d)", key.Comment, apiKeyMinLength)
			key.Key = "INVALID"
		}
	}

	return nil
}

// Save the database on the disk
func (db *APIKeyDatabase) Save() error {
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
