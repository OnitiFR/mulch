package server

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"
)

// BackupDatabase describes a persistent Backup instances database
type BackupDatabase struct {
	filename string
	db       map[string]*Backup
	mutex    sync.Mutex
	app      *App
}

// NewBackupDatabase instanciates a new BackupDatabase
func NewBackupDatabase(filename string, app *App) (*BackupDatabase, error) {
	db := &BackupDatabase{
		filename: filename,
		db:       make(map[string]*Backup),
		app:      app,
	}

	// if the file exists, load it
	if _, err := os.Stat(db.filename); err == nil {
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

// Run the database monitoring loop
func (db *BackupDatabase) Run() error {
	// small cooldown (app init)
	time.Sleep(5 * time.Second)

	for {
		db.deleteExpired()
		time.Sleep(10 * time.Minute)
	}
}

// This is done internaly, because it must be done with the mutex locked,
// but we can't lock it here, since save() is called by functions that
// are already locking the mutex.
func (db *BackupDatabase) save() error {
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

func (db *BackupDatabase) load() error {
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

// deleteExpired deletes all expired backups
func (db *BackupDatabase) deleteExpired() {
	expired := make([]string, 0)

	db.mutex.Lock()
	for _, backup := range db.db {
		if backup.Expire.IsZero() {
			continue
		}

		if backup.Expire.Before(time.Now()) {
			expired = append(expired, backup.DiskName)
		}
	}
	db.mutex.Unlock()

	for _, backup := range expired {
		db.app.Log.Infof("deleting expired backup '%s'", backup)
		err := BackupDelete(backup, db.app)
		if err != nil {
			db.app.Log.Errorf("error deleting expired backup '%s': %s", backup, err)
		}
	}
}

// delete the Backup from the database using its name
// (must be locked by the caller)
func (db *BackupDatabase) delete(name string) error {
	if _, exists := db.db[name]; !exists {
		return fmt.Errorf("backup '%s' was not found in database", name)
	}

	delete(db.db, name)

	err := db.save()
	if err != nil {
		return err
	}

	return nil
}

// Delete the Backup from the database using its name
func (db *BackupDatabase) Delete(name string) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	return db.delete(name)
}

// Add a new Backup in the database
func (db *BackupDatabase) Add(backup *Backup) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	if _, exists := db.db[backup.DiskName]; exists {
		return fmt.Errorf("backup '%s' already exists in database", backup.DiskName)
	}

	db.db[backup.DiskName] = backup
	err := db.save()
	if err != nil {
		return err
	}
	return nil
}

// GetNames of all Backups in the database
func (db *BackupDatabase) GetNames() []string {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	keys := make([]string, 0, len(db.db))
	for key := range db.db {
		keys = append(keys, key)
	}
	return keys
}

// GetByName lookups a Backup by its name, or nil if not found
func (db *BackupDatabase) GetByName(name string) *Backup {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	backup, exists := db.db[name]
	if !exists {
		return nil
	}
	return backup
}

// Count returns the number of Backups in the database
func (db *BackupDatabase) Count() int {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	return len(db.db)
}

// Expire defines the expiration date of a Backup (0 means no expiration)
func (db *BackupDatabase) Expire(name string, expire time.Time) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	_, exists := db.db[name]
	if !exists {
		return fmt.Errorf("backup '%s' was not found in database", name)
	}

	db.db[name].Expire = expire
	return nil
}
