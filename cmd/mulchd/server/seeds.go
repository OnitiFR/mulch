package server

import (
	"encoding/json"
	"net/http"
	"os"
	"time"
)

// SeedDatabase describes a persistent DataBase of Seed structures
type SeedDatabase struct {
	filename string
	db       map[string]*Seed
	app      *App
}

// Seed entry in the DB
type Seed struct {
	CurrentURL   string
	As           string
	Ready        bool
	LastModified time.Time
}

// NewSeeder instanciates a new VMDatabase
func NewSeeder(filename string, app *App) (*SeedDatabase, error) {
	db := &SeedDatabase{
		app:      app,
		filename: filename,
		db:       make(map[string]*Seed),
	}

	// if the file exists, load it
	if _, err := os.Stat(db.filename); err == nil {
		err = db.load()
		if err != nil {
			return nil, err
		}
	}

	// reconciliate the DB with the config:
	// 1 - add new entries and refresh existing ones
	for name, configEntry := range app.Config.Seeds {
		seed, exists := db.db[name]
		if exists {
			seed.As = configEntry.As
			seed.CurrentURL = configEntry.CurrentURL
		} else {
			app.Log.Infof("adding a new seed '%s'", name)
			db.db[name] = &Seed{
				As:         configEntry.As,
				CurrentURL: configEntry.CurrentURL,
				Ready:      false,
			}
		}
	}

	// 2 - remove old entries
	for name := range db.db {
		_, exists := app.Config.Seeds[name]
		if exists == false {
			app.Log.Infof("removing old seed '%s'", name)
			delete(db.db, name)
			// TODO: delete from storage
		}
	}

	// save the file to check if it's writable
	err := db.save()
	if err != nil {
		return nil, err
	}

	return db, nil
}

func (db *SeedDatabase) save() error {
	f, err := os.OpenFile(db.filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	err = enc.Encode(&db.db)
	if err != nil {
		return err
	}
	return nil
}

func (db *SeedDatabase) load() error {
	f, err := os.Open(db.filename)
	if err != nil {
		return err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	err = dec.Decode(&db.db)
	if err != nil {
		return err
	}
	return nil
}

// Run the seeder (check Last-Modified dates, download new releases)
func (db *SeedDatabase) Run() {
	// small cooldown (app init)
	time.Sleep(1 * time.Second)

	for {
		modified := false
		for name, seed := range db.db {
			res, err := http.Head(seed.CurrentURL)
			if err != nil {
				db.app.Log.Errorf("seeder '%s': %s", name, err)
				continue
			}
			lm := res.Header.Get("Last-Modified")
			if lm == "" {
				db.app.Log.Errorf("seeder '%s': undefined Last-Modified header", name)
				continue
			}
			t, err := http.ParseTime(lm)
			if err != nil {
				db.app.Log.Errorf("seeder '%s': can't parse Last-Modified header: %s", name, err)
				continue
			}
			if seed.LastModified != t {
				db.app.Log.Infof("update needed for seed '%s'", name)
				seed.LastModified = t
				seed.Ready = false
				modified = true
				// TODO: download file + upload to libvirt
			}
		}
		if modified {
			db.save()
		}
		time.Sleep(1 * time.Hour)
	}
}
