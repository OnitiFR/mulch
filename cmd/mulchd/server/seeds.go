package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/OnitiFR/mulch/common"
	"github.com/c2h5oh/datasize"
	"gopkg.in/libvirt/libvirt-go.v5"
)

// SeedDatabase describes a persistent DataBase of Seed structures
type SeedDatabase struct {
	filename string
	db       map[string]*Seed
	app      *App
}

// Seed entry in the DB
type Seed struct {
	Name         string
	URL          string
	Seeder       string
	Ready        bool
	LastModified time.Time
	Size         uint64
	Status       string
	StatusTime   time.Time
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
			if seed.URL != "" && configEntry.URL == "" {
				return nil, fmt.Errorf("seed '%s': converting URL seeds to Seeders is not supported", name)
			}
			if seed.Seeder != "" && configEntry.Seeder == "" {
				return nil, fmt.Errorf("seed '%s': converting Seeders to URL seeds is not supported", name)
			}
			seed.URL = configEntry.URL
			seed.Seeder = configEntry.Seeder
		} else {
			app.Log.Infof("adding a new seed '%s'", name)
			db.db[name] = &Seed{
				Name:  name,
				URL:   configEntry.URL,
				Ready: false,
			}
		}
	}

	// 2 - remove old entries
	for name, oldSeed := range db.db {
		_, exists := app.Config.Seeds[name]
		if exists == false {
			app.Log.Infof("removing old seed '%s'", name)
			delete(db.db, name)
			app.Libvirt.DeleteVolume(oldSeed.GetVolumeName(), app.Libvirt.Pools.Seeds)
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

func seedSendErrorAlert(app *App, seed string) {
	app.AlertSender.Send(&Alert{
		Type:    AlertTypeBad,
		Subject: "Seed error",
		Content: fmt.Sprintf("Error with seed %s, see seed status for details.", seed),
	})
}

func (db *SeedDatabase) runStep() {
	for name, seed := range db.db {
		var err error

		if seed.URL != "" {
			err = db.refreshSeed(seed)
		}
		if seed.Seeder != "" {
			err = db.refreshSeeder(seed)
		}

		if err != nil {
			msg := fmt.Sprintf("seeder '%s': %s", name, err)
			db.app.Log.Error(msg)
			seed.UpdateStatus(msg)
			seedSendErrorAlert(db.app, name)
		}
	}
}

func (db *SeedDatabase) refreshSeeder(seed *Seed) error {

	// TODO: check if a rebuild is necessary

	db.app.Log.Infof("rebuilding seed '%s'", seed.Name)
	log := NewLog(seed.Name, db.app.Hub, db.app.LogHistory)

	_, err := url.ParseRequestURI(seed.Seeder)
	if err != nil {
		return err
	}

	stream, err := GetContentFromURL(seed.Seeder)
	if err != nil {
		return err
	}
	defer stream.Close()

	conf, err := NewVMConfigFromTomlReader(stream, log)
	if err != nil {
		return fmt.Errorf("decoding config: %s", err)
	}

	operation := db.app.Operations.Add(&Operation{
		Origin:        "[seeder]",
		Action:        "rebuild",
		Ressource:     "seed",
		RessourceName: seed.Name,
	})
	defer db.app.Operations.Remove(operation)

	before := time.Now()
	_, vmName, err := NewVM(conf, VMInactive, VMStopOnScriptFailure, "[seeder]", db.app, log)
	if err != nil {
		log.Failuref("Cannot create VM: %s", err)
		return err
	}
	defer VMDelete(vmName, db.app, log)

	after := time.Now()
	log.Successf("VM %s created successfully (%s)", vmName, after.Sub(before))

	err = VMStopByName(vmName, db.app, log)
	if err != nil {
		return err
	}

	// copy disk to seed storage
	// update seed status
	// update rebuild date? (see at the top of this function)

	return nil
}

func (db *SeedDatabase) refreshSeed(seed *Seed) error {
	name := seed.Name
	res, err := http.Head(seed.URL)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("response was %s (%v)", res.Status, res.StatusCode)
	}

	lm := res.Header.Get("Last-Modified")
	if lm == "" {
		return errors.New("undefined Last-Modified header")
	}
	t, err := http.ParseTime(lm)
	if err != nil {
		return fmt.Errorf("can't parse Last-Modified header: %s", err)
	}
	if seed.LastModified != t {
		db.app.Log.Infof("downloading seed '%s'", name)
		seed.Ready = false
		db.save()

		before := time.Now()
		tmpFile, err := db.seedDownload(seed, db.app.Config.TempPath)
		if err != nil {
			return fmt.Errorf("unable to download image: %s", err)
		}
		defer os.Remove(tmpFile)

		// upload to libvirt seed storage
		db.app.Log.Infof("moving seed '%s' to storage", name)

		errR := db.app.Libvirt.DeleteVolume(seed.GetVolumeName(), db.app.Libvirt.Pools.Seeds)
		if err != nil {
			virtErr := errR.(libvirt.Error)
			if !(virtErr.Domain == libvirt.FROM_STORAGE && virtErr.Code == libvirt.ERR_NO_STORAGE_VOL) {
				return fmt.Errorf("unable to delete old image: %s", errR)
			}
		}

		err = db.app.Libvirt.UploadFileToLibvirt(
			db.app.Libvirt.Pools.Seeds,
			db.app.Libvirt.Pools.SeedsXML,
			db.app.Config.GetTemplateFilepath("volume.xml"),
			tmpFile,
			seed.GetVolumeName(),
			db.app.Log)
		if err != nil {
			return fmt.Errorf("unable to move image to storage: %s", err)
		}
		after := time.Now()

		seed.Ready = true
		seed.LastModified = t
		seed.UpdateStatus(fmt.Sprintf("downloaded and stored in %s", after.Sub(before)))
		db.save()
		db.app.Log.Infof("seed '%s' is now ready", name)
	}
	return nil
}

func (db *SeedDatabase) seedDownload(seed *Seed, tmpPath string) (string, error) {
	tmpfile, err := ioutil.TempFile(tmpPath, "mulch-seed-image")
	if err != nil {
		return "", err
	}
	defer tmpfile.Close()

	operation := db.app.Operations.Add(&Operation{
		Origin:        "[seeder]",
		Action:        "download",
		Ressource:     "seed",
		RessourceName: seed.GetVolumeName(),
	})
	defer db.app.Operations.Remove(operation)

	resp, err := http.Get(seed.URL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("response was %s (%v)", resp.Status, resp.StatusCode)
	}

	total, _ := strconv.Atoi(resp.Header.Get("Content-Length"))
	seed.Size = uint64(total)

	wc := &common.WriteCounter{
		Total: uint64(total),
		Step:  1024 * 1024, // 1 MB
		CB: func(current uint64, total uint64) {
			seed.UpdateStatus(fmt.Sprintf("downloading %s (%d%%)",
				(datasize.ByteSize(total) * datasize.B).HR(),
				(current*100)/total),
			)
		},
	}
	tee := io.TeeReader(resp.Body, wc)

	_, err = io.Copy(tmpfile, tee)
	if err != nil {
		return "", err
	}

	return tmpfile.Name(), nil
}

// UpdateStatus change status informations
func (seed *Seed) UpdateStatus(status string) {
	seed.Status = status
	seed.StatusTime = time.Now()
}

// GetVolumeName return the seed volume file name
func (seed *Seed) GetVolumeName() string {
	return seed.Name + ".qcow2"
}
