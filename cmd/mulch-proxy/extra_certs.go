package main

import (
	"crypto/tls"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
)

const ExtraCertsReloadDelay = 10 * time.Second

// ExtraCert is a certificate/key pair
type ExtraCert struct {
	Cert tls.Certificate

	Certfile             string
	CertfileLastModified time.Time
	Keyfile              string
	KeyfileLastModified  time.Time
}

// ExtraCertsDB is a database of ExtraCert
type ExtraCertsDB struct {
	configFile       string
	db               map[string]*ExtraCert
	fileLastModified time.Time
	mutex            sync.Mutex
	log              *Log
}

type tomlExtraCert struct {
	Domains  []string `toml:"domains"`
	Certfile string   `toml:"certfile"`
	Keyfile  string   `toml:"keyfile"`
}

type tomlExtraCerts struct {
	Certificate []tomlExtraCert `toml:"certificate"`
}

// NewExtraCertsDB creates and load a new ExtraCertsDB
func NewExtraCertsDB(configFile string, log *Log) (*ExtraCertsDB, error) {
	db := &ExtraCertsDB{
		configFile: configFile,
		db:         make(map[string]*ExtraCert),
		log:        log,
	}

	err := db.Load()
	if err != nil {
		return nil, err
	}

	go db.ScheduleReload()

	log.Infof("%d domain(s) with extra certificates", len(db.db))

	return db, nil
}

// ScheduleReload will schedule a reload of the ExtraCertsDB
func (db *ExtraCertsDB) ScheduleReload() {
	for {
		time.Sleep(ExtraCertsReloadDelay)
		if db.IsReloadNeeded() {
			db.log.Infof("extra certificates changed, reloading")
			err := db.Load()
			if err != nil {
				db.log.Errorf("error while reloading extra certificates: %s", err)
			}
		}
	}
}

// ExtraCertsDB will load the extra certificates from the configuration file
func (db *ExtraCertsDB) Load() error {
	newMap := make(map[string]*ExtraCert)

	db.log.Tracef("(re)loading extra certificates from %s", db.configFile)

	fi, err := os.Stat(db.configFile)
	if err != nil {
		db.log.Warningf("error while loading extra certificates from %s: %s", db.configFile, err)
		db.db = newMap
		db.fileLastModified = time.Time{}
		return nil
	}
	db.fileLastModified = fi.ModTime()

	var tomlConfig tomlExtraCerts
	meta, err := toml.DecodeFile(db.configFile, &tomlConfig)
	if err != nil {
		return err
	}

	undecoded := meta.Undecoded()
	for _, param := range undecoded {
		return fmt.Errorf("unknown setting '%s' (%s)", param, db.configFile)
	}

	for _, cert := range tomlConfig.Certificate {
		if len(cert.Domains) == 0 {
			return fmt.Errorf("no domain(s) specified for extra certificate (%s)", db.configFile)
		}

		certStats, err := os.Stat(cert.Certfile)
		if err != nil {
			return fmt.Errorf("error while loading extra certificate '%s': %s", cert.Certfile, err)
		}

		keyStats, err := os.Stat(cert.Keyfile)
		if err != nil {
			return fmt.Errorf("error while loading extra certificate key '%s': %s", cert.Keyfile, err)
		}

		cer, err := tls.LoadX509KeyPair(cert.Certfile, cert.Keyfile)
		if err != nil {
			return err
		}

		ec := &ExtraCert{
			Certfile:             cert.Certfile,
			CertfileLastModified: certStats.ModTime(),
			Keyfile:              cert.Keyfile,
			KeyfileLastModified:  keyStats.ModTime(),
			Cert:                 cer,
		}

		for _, domain := range cert.Domains {
			newMap[domain] = ec
		}
	}

	db.log.Tracef("loaded extra certificates for %d domain(s)", len(newMap))

	db.mutex.Lock()
	defer db.mutex.Unlock()
	db.db = newMap
	return nil
}

// Get the ExtraCert for the given domain (or nil if not found)
func (db *ExtraCertsDB) Get(domain string) *ExtraCert {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	return db.db[domain]
}

// IsReloadNeeded returns true if the configuration file or any certificate
// has been modified
func (db *ExtraCertsDB) IsReloadNeeded() bool {
	fi, err := os.Stat(db.configFile)
	if err != nil {
		// will empty the map if the file is deleted
		return len(db.db) != 0 // "len = 0 ? return false" (no reload)
	}

	if fi.ModTime() != db.fileLastModified {
		return true
	}

	for _, cert := range db.db {
		certStats, err := os.Stat(cert.Certfile)
		if err != nil {
			return false
		}

		keyStats, err := os.Stat(cert.Keyfile)
		if err != nil {
			return false
		}

		if certStats.ModTime() != cert.CertfileLastModified ||
			keyStats.ModTime() != cert.KeyfileLastModified {
			return true
		}
	}

	return false
}
