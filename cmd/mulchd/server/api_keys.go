package server

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strconv"
)

// TODO: lock this database with a mutex?

// APIKey describes an API key
type APIKey struct {
	Comment string
	Key     string
}

// APIKeyDatabase describes a persistent API Key database
type APIKeyDatabase struct {
	filename string
	keys     []APIKey
}

// NewAPIKeyDatabase creates a new API key database
func NewAPIKeyDatabase(filename string, log *Log, rand *rand.Rand) (*APIKeyDatabase, error) {
	db := &APIKeyDatabase{
		filename: filename,
	}

	// if the file exists, load it
	if _, err := os.Stat(db.filename); err == nil {
		err = db.load()
		if err != nil {
			return nil, err
		}
	} else {
		log.Warningf("no API keys database found, creating a new one with a default key (%s)", filename)
		db.keys = []APIKey{
			APIKey{
				Comment: "default-key",
				Key:     "yeurae4eim1Ooqu0booS0zohH7queeju7ayohgh0baiFaeShohngoachaekahv7w",
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

func (db *APIKeyDatabase) load() error {
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
