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
	conn, errC := lv.GetConnection()
	if errC != nil {
		return nil, nil, errC
	}

	pool, errP := conn.LookupStoragePoolByName(poolName)
	if errP != nil {
		virtErr := errP.(libvirt.Error)
		if virtErr.Domain == libvirt.FROM_STORAGE && virtErr.Code == libvirt.ERR_NO_STORAGE_POOL {
			log.Info(fmt.Sprintf("storage pool '%s' not found, let's create it", poolName))

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

			pool, err = conn.StoragePoolDefineXML(string(out), 0)
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
	conn, errC := lv.GetConnection()
	if errC != nil {
		return nil, nil, errC
	}

	net, errN := conn.LookupNetworkByName(networkName)
	if errN != nil {
		virtErr := errN.(libvirt.Error)
		if virtErr.Domain == libvirt.FROM_NETWORK && virtErr.Code == libvirt.ERR_NO_NETWORK {
			log.Info(fmt.Sprintf("network '%s' not found, it's OK, let's create it", networkName))

			xml, err := ioutil.ReadFile(templateFile)
			if err != nil {
				return nil, nil, fmt.Errorf("GetOrCreateNetwork: %s: %s", templateFile, err)
			}

			net, err = conn.NetworkDefineXML(string(xml))
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
	conn, errC := lv.GetConnection()
	if errC != nil {
		return errC
	}

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

	vt, err := NewVolumeTransfert(conn, volSrc, conn, volDst)
	if err != nil {
		return err
	}

	bytesWritten, err := vt.Copy()
	if err != nil {
		return err
	}

	log.Infof("disk '%s' created from seed '%s' (transfered %s)", disk, seed, FormatByteSize(bytesWritten))
	return nil
}

// UploadFileToLibvirt uploads a file to libvirt storage
func (lv *Libvirt) UploadFileToLibvirt(pool *libvirt.StoragePool, poolXML *libvirtxml.StoragePool, template string, localSourceFile string, asName string, log *Log) error {
	conn, errC := lv.GetConnection()
	if errC != nil {
		return errC
	}

	// create dest volume
	xml, err := ioutil.ReadFile(template)
	if err != nil {
		return err
	}

	volcfg := &libvirtxml.StorageVolume{}
	err = volcfg.Unmarshal(string(xml))
	if err != nil {
		return err
	}
	volcfg.Name = asName

	volcfg.Target.Path = poolXML.Target.Path + "/" + asName
	// volObj.Target.Format.Type = "raw"

	xml2, err := volcfg.Marshal()
	if err != nil {
		return err
	}
	volDst, err := pool.StorageVolCreateXML(string(xml2), 0)
	if err != nil {
		return err
	}
	defer volDst.Free()

	vu, err := NewVolumeUpload(localSourceFile, conn, volDst)
	if err != nil {
		return err
	}

	bytesWritten, err := vu.Copy()
	if err != nil {
		return err
	}

	log.Infof("upload '%s' to storage pool '%s' as '%s' (transfered %s)", localSourceFile, poolXML.Name, asName, FormatByteSize(bytesWritten))
	return nil
}

// ResizeDisk will change volume ("disk") size
// (do not reduce a volume without knowing what you are doing!)
func (lv *Libvirt) ResizeDisk(disk string, size uint64, log *Log) error {

	err := lv.Pools.Seeds.Refresh(0)
	if err != nil {
		return err
	}

	vol, err := lv.Pools.Disks.LookupStorageVolByName(disk)
	if err != nil {
		return err
	}
	defer vol.Free()

	err = vol.Resize(size, 0)
	if err != nil {
		return err
	}
	log.Infof("disk '%s' resized to %s", disk, FormatByteSize(int64(size)))
	return nil
}
