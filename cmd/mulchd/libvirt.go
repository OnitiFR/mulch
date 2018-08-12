package main

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/libvirt/libvirt-go"
	"github.com/libvirt/libvirt-go-xml"
)

// TODO: deal with keep-alive, disconnections, etc

// Libvirt is an interface to libvirt library
type Libvirt struct {
	conn *libvirt.Connect
	app  *App
}

// NewLibvirt create a new Libvirt instance
func NewLibvirt(uri string) (*Libvirt, error) {
	conn, err := libvirt.NewConnect(uri)
	if err != nil {
		return nil, err
	}

	return &Libvirt{
		conn: conn,
	}, nil
}

// CloseConnection close connection to libvirt
func (lv *Libvirt) CloseConnection() {
	lv.conn.Close()
}

// GetOrCreateStoragePool retreives (and create, if necessary) a storage pool
// (mode is the Unix access mode for the pool directory)
//
// I've seen strange things once in a while, like:
// - Code=38, Domain=0, Message='cannot open directory '…/storage/cloud-init': No such file or directory'
// - Code=55, Domain=18, Message='Requested operation is not valid: storage pool 'mulch-cloud-init' is not active
// Added more precise error messages to diagnose this.
func (lv *Libvirt) GetOrCreateStoragePool(poolName string, poolPath string, templateFile string, mode string, log *Log) (*libvirt.StoragePool, error) {
	pool, errP := lv.conn.LookupStoragePoolByName(poolName)
	if errP != nil {
		virtErr := errP.(libvirt.Error)
		if virtErr.Domain == libvirt.FROM_STORAGE && virtErr.Code == libvirt.ERR_NO_STORAGE_POOL {
			log.Info(fmt.Sprintf("pool '%s' not found, let's create it", poolName))

			xml, err := ioutil.ReadFile(templateFile)
			if err != nil {
				return nil, err
			}

			poolcfg := &libvirtxml.StoragePool{}
			err = poolcfg.Unmarshal(string(xml))
			if err != nil {
				return nil, fmt.Errorf("GetOrCreateStoragePool: poolcfg.Unmarshal: %s", err)
			}

			poolcfg.Name = poolName
			// check full path rght access? (too specific?)
			absPoolPath, err := filepath.Abs(poolPath)
			if err != nil {
				return nil, fmt.Errorf("GetOrCreateStoragePool: filepath.Abs: %s", err)
			}

			poolcfg.Target.Path = absPoolPath

			if mode != "" {
				poolcfg.Target.Permissions.Mode = mode
			}

			out, err := poolcfg.Marshal()
			if err != nil {
				return nil, fmt.Errorf("GetOrCreateStoragePool: poolcfg.Marshal: %s", err)
			}

			pool, err = lv.conn.StoragePoolDefineXML(string(out), 0)
			if err != nil {
				return nil, fmt.Errorf("GetOrCreateStoragePool: StoragePoolDefineXML: %s", err)
			}

			pool.SetAutostart(true)
			if err != nil {
				return nil, fmt.Errorf("GetOrCreateStoragePool: pool.SetAutostart: %s", err)
			}

			// WITH_BUILD = will create target directory if net exists
			err = pool.Create(libvirt.STORAGE_POOL_CREATE_WITH_BUILD)
			if err != nil {
				return nil, fmt.Errorf("GetOrCreateStoragePool: pool.Create: %s", err)
			}
		}
	}

	err := pool.Refresh(0)
	if err != nil {
		return nil, fmt.Errorf("GetOrCreateStoragePool: pool.Refresh: %s", err)
	}
	return pool, nil
}
