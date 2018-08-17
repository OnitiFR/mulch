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
	conn       *libvirt.Connect
	Pools      LibvirtPools
	Network    *libvirt.Network
	NetworkXML *libvirtxml.Network
}

// LibvirtPools stores needed libvirt Pools for mulchd
type LibvirtPools struct {
	CloudInit    *libvirt.StoragePool
	Seeds        *libvirt.StoragePool
	Disks        *libvirt.StoragePool
	CloudInitXML *libvirtxml.StoragePool
	SeedsXML     *libvirtxml.StoragePool
	DisksXML     *libvirtxml.StoragePool
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

// GetConnection returns the current libvirt connection
func (lv *Libvirt) GetConnection() (*libvirt.Connect, error) {
	// TODO: test if connection is OK, THEN return it?
	// (what about storages, networks, etc?)
	return lv.conn, nil
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
func (lv *Libvirt) GetOrCreateStoragePool(poolName string, poolPath string, templateFile string, mode string, log *Log) (*libvirt.StoragePool, *libvirtxml.StoragePool, error) {
	pool, errP := lv.conn.LookupStoragePoolByName(poolName)
	if errP != nil {
		virtErr := errP.(libvirt.Error)
		if virtErr.Domain == libvirt.FROM_STORAGE && virtErr.Code == libvirt.ERR_NO_STORAGE_POOL {
			log.Info(fmt.Sprintf("pool '%s' not found, let's create it", poolName))

			xml, err := ioutil.ReadFile(templateFile)
			if err != nil {
				return nil, nil, fmt.Errorf("GetOrCreateStoragePool: %s: %s", templateFile, err)
			}

			poolcfg := &libvirtxml.StoragePool{}
			err = poolcfg.Unmarshal(string(xml))
			if err != nil {
				return nil, nil, fmt.Errorf("GetOrCreateStoragePool: poolcfg.Unmarshal: %s", err)
			}

			poolcfg.Name = poolName
			// TODO: check full path rght access? (too specific?)
			absPoolPath, err := filepath.Abs(poolPath)
			if err != nil {
				return nil, nil, fmt.Errorf("GetOrCreateStoragePool: filepath.Abs: %s", err)
			}

			poolcfg.Target.Path = absPoolPath

			if mode != "" {
				poolcfg.Target.Permissions.Mode = mode
			}

			out, err := poolcfg.Marshal()
			if err != nil {
				return nil, nil, fmt.Errorf("GetOrCreateStoragePool: poolcfg.Marshal: %s", err)
			}

			pool, err = lv.conn.StoragePoolDefineXML(string(out), 0)
			if err != nil {
				return nil, nil, fmt.Errorf("GetOrCreateStoragePool: StoragePoolDefineXML: %s", err)
			}

			pool.SetAutostart(true)
			if err != nil {
				return nil, nil, fmt.Errorf("GetOrCreateStoragePool: pool.SetAutostart: %s", err)
			}

			// WITH_BUILD = will create target directory if net exists
			err = pool.Create(libvirt.STORAGE_POOL_CREATE_WITH_BUILD)
			if err != nil {
				return nil, nil, fmt.Errorf("GetOrCreateStoragePool: pool.Create: %s", err)
			}
		}
	}

	err := pool.Refresh(0)
	if err != nil {
		return nil, nil, fmt.Errorf("GetOrCreateStoragePool: pool.Refresh: %s", err)
	}

	xmldoc, err := pool.GetXMLDesc(0)
	if err != nil {
		return nil, nil, fmt.Errorf("GetOrCreateStoragePool: GetXMLDesc: %s", err)
	}

	poolcfg := &libvirtxml.StoragePool{}
	err = poolcfg.Unmarshal(xmldoc)
	if err != nil {
		return nil, nil, fmt.Errorf("GetOrCreateStoragePool: Unmarshal: %s", err)
	}

	return pool, poolcfg, nil
}

// GetOrCreateNetwork retreives (and create, if necessary) a libvirt network
func (lv *Libvirt) GetOrCreateNetwork(networkName string, templateFile string, log *Log) (*libvirt.Network, *libvirtxml.Network, error) {
	net, errN := lv.conn.LookupNetworkByName(networkName)
	if errN != nil {
		virtErr := errN.(libvirt.Error)
		if virtErr.Domain == libvirt.FROM_NETWORK && virtErr.Code == libvirt.ERR_NO_NETWORK {
			log.Info(fmt.Sprintf("network '%s' not found, it's OK, let's create it", networkName))

			xml, err := ioutil.ReadFile(templateFile)
			if err != nil {
				return nil, nil, fmt.Errorf("GetOrCreateNetwork: %s: %s", templateFile, err)
			}

			net, err = lv.conn.NetworkDefineXML(string(xml))
			if err != nil {
				return nil, nil, fmt.Errorf("GetOrCreateNetwork: NetworkDefineXML: %s", err)
			}

			err = net.SetAutostart(true)
			if err != nil {
				return nil, nil, fmt.Errorf("GetOrCreateNetwork: SetAutostart: %s", err)
			}

			err = net.Create()
			if err != nil {
				return nil, nil, fmt.Errorf("GetOrCreateNetwork: Create: %s", err)
			}
		} else {
			return nil, nil, fmt.Errorf("GetOrCreateNetwork: Unexpected error: %s", errN)
		}
	}

	xmldoc, err := net.GetXMLDesc(0)
	if err != nil {
		return nil, nil, fmt.Errorf("GetOrCreateNetwork: GetXMLDesc: %s", err)
	}

	netcfg := &libvirtxml.Network{}
	err = netcfg.Unmarshal(xmldoc)
	if err != nil {
		return nil, nil, fmt.Errorf("GetOrCreateNetwork: Unmarshal: %s", err)
	}

	return net, netcfg, nil
}

// CreateDiskFromSeed creates a disk (into "disks" pool) from seed image (from "seeds" pool)
func (lv *Libvirt) CreateDiskFromSeed(seed string, disk string, volumeTemplateFile string, log *Log) error {

	err := lv.Pools.Seeds.Refresh(0)
	if err != nil {
		return err
	}

	// find source volume
	volSrc, err := lv.Pools.Seeds.LookupStorageVolByName(seed)
	if err != nil {
		return err
	}
	defer volSrc.Free()

	// create dest volume
	xml, err := ioutil.ReadFile(volumeTemplateFile)
	if err != nil {
		return err
	}

	volcfg := &libvirtxml.StorageVolume{}
	err = volcfg.Unmarshal(string(xml))
	if err != nil {
		return err
	}
	volcfg.Name = disk

	volcfg.Target.Path = lv.Pools.DisksXML.Target.Path + "/" + disk
	// volObj.Target.Format.Type = "raw"

	xml2, err := volcfg.Marshal()
	if err != nil {
		return err
	}
	volDst, err := lv.Pools.Disks.StorageVolCreateXML(string(xml2), 0)
	if err != nil {
		return err
	}
	defer volDst.Free()

	vt, err := NewVolumeTransfert(lv.conn, volSrc, lv.conn, volDst)
	if err != nil {
		return err
	}

	bytesWritten, err := vt.Copy()
	if err != nil {
		return err
	}

	log.Infof("done: %s → %s (transfered %d MiB)", seed, disk, bytesWritten/1024/1024)
	return nil
}
