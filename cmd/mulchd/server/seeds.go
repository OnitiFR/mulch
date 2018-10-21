package server

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/libvirt/libvirt-go"
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

	for _, seed := range db.db {
		if seed.Ready == false {
			// Reset LastModified to restart download
			seed.LastModified = time.Time{}
		}
	}

	// reconciliate the DB with the config:
	// 1 - add new entries and refresh existing ones
	for name, configEntry := range app.Config.Seeds {
		seed, exists := db.db[name]
		if exists {
			if seed.As != configEntry.As {
				app.Log.Warningf("changing seed 'as' setting is not supported yet! (remove seed %s, launch mulchd, re-create seed)", name)
			}
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
	for name, oldSeed := range db.db {
		_, exists := app.Config.Seeds[name]
		if exists == false {
			app.Log.Infof("removing old seed '%s'", name)
			delete(db.db, name)
			app.Libvirt.RemoveVolume(oldSeed.As, app.Libvirt.Pools.Seeds)
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

// GetByName returns a seed using its name (or an error)
func (db *SeedDatabase) GetByName(name string) (*Seed, error) {
	seed, exits := db.db[name]
	if exits == false {
		return nil, fmt.Errorf("seed %s does not exists", name)
	}
	return seed, nil
}

// GetNames returns a list of seed names
func (db *SeedDatabase) GetNames() []string {
	keys := make([]string, 0, len(db.db))
	for key := range db.db {
		keys = append(keys, key)
	}
	return keys
}

// Run the seeder (check Last-Modified dates, download new releases)
func (db *SeedDatabase) Run() {
	// small cooldown (app init)
	time.Sleep(1 * time.Second)

	for {
		db.runStep()
		time.Sleep(1 * time.Hour)
	}
}

func (db *SeedDatabase) runStep() {
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
			db.app.Log.Infof("downloading seed '%s'", name)
			seed.Ready = false
			db.save()

			tmpFile, err := db.seedDownload(seed)
			if err != nil {
				db.app.Log.Errorf("seeder '%s': unable to download image: %s", name, err)
				continue
			}
			defer os.Remove(tmpFile)

			// upload to libvirt seed storage
			db.app.Log.Infof("moving seed '%s' to storage", name)

			errR := db.app.Libvirt.RemoveVolume(seed.As, db.app.Libvirt.Pools.Seeds)
			if err != nil {
				virtErr := errR.(libvirt.Error)
				if !(virtErr.Domain == libvirt.FROM_STORAGE && virtErr.Code == libvirt.ERR_NO_STORAGE_VOL) {
					db.app.Log.Errorf("seeder '%s': unable to delete old image: %s", name, errR)
					continue
				}
			}

			err = db.app.Libvirt.UploadFileToLibvirt(
				db.app.Libvirt.Pools.Seeds,
				db.app.Libvirt.Pools.SeedsXML,
				db.app.Config.GetTemplateFilepath("volume.xml"),
				tmpFile,
				seed.As,
				db.app.Log)
			if err != nil {
				db.app.Log.Errorf("seeder '%s': unable to move image to storage: %s", name, err)
				continue
			}

			seed.Ready = true
			seed.LastModified = t
			db.save()
			db.app.Log.Infof("seed '%s' is now ready", name)
			os.Remove(tmpFile) // remove now (already deferred, but let's free disk space)
		}
	}
}

func (db *SeedDatabase) seedDownload(seed *Seed) (string, error) {

	tmpfile, err := ioutil.TempFile("", "mulch-seed-image")
	if err != nil {
		return "", err
	}
	defer tmpfile.Close()

	resp, err := http.Get(seed.CurrentURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	_, err = io.Copy(tmpfile, resp.Body)
	if err != nil {
		return "", err
	}

	return tmpfile.Name(), nil
}
