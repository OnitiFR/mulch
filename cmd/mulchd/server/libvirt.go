package server

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/OnitiFR/mulch/cmd/mulchd/volumes"
	"github.com/c2h5oh/datasize"
	libvirtxml "gopkg.in/libvirt/libvirt-go-xml.v7"
	"gopkg.in/libvirt/libvirt-go.v7"
)

// TODO: deal with keep-alive, disconnections, etc

// Libvirt is an interface to libvirt library
type Libvirt struct {
	connection *libvirt.Connect
	uri        string
	dhcpLeases *LibvirtDHCPLeases
	Pools      LibvirtPools
	Network    *libvirt.Network
	NetworkXML *libvirtxml.Network
}

// LibvirtPools stores needed libvirt Pools for mulchd
type LibvirtPools struct {
	Seeds   *libvirt.StoragePool
	Disks   *libvirt.StoragePool
	Backups *libvirt.StoragePool

	SeedsXML   *libvirtxml.StoragePool
	DisksXML   *libvirtxml.StoragePool
	BackupsXML *libvirtxml.StoragePool
}

// NewLibvirt create a new Libvirt instance
func NewLibvirt(uri string) (*Libvirt, error) {
	conn, err := libvirt.NewConnect(uri)
	if err != nil {
		return nil, err
	}

	return &Libvirt{
		connection: conn,
		uri:        uri,
		dhcpLeases: NewLibvirtDHCPLeases(),
	}, nil
}

// GetConnection returns the current libvirt connection
func (lv *Libvirt) GetConnection() (*libvirt.Connect, error) {

	// Test connection
	lv.connection.GetVersion()

	alive, err := lv.connection.IsAlive()
	if err != nil {
		return nil, err
	}

	if !alive {
		lv.CloseConnection()
		conn, err := libvirt.NewConnect(lv.uri)
		if err != nil {
			return nil, err
		}

		// replace connection
		lv.connection = conn

		lv.Pools.Seeds, err = conn.LookupStoragePoolByName(AppStorageSeeds)
		if err != nil {
			return nil, err
		}

		lv.Pools.Disks, err = conn.LookupStoragePoolByName(AppStorageDisks)
		if err != nil {
			return nil, err
		}

		lv.Pools.Backups, err = conn.LookupStoragePoolByName(AppStorageBackups)
		if err != nil {
			return nil, err
		}

		lv.Network, err = conn.LookupNetworkByName(AppNetwork)
		if err != nil {
			return nil, err
		}

	}
	return lv.connection, nil
}

// CloseConnection close connection to libvirt
func (lv *Libvirt) CloseConnection() {
	lv.connection.Close()
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

	if active, errI := pool.IsActive(); !active || errI != nil {
		return nil, nil, fmt.Errorf("GetOrCreateStoragePool: pool %s is not active: %s", poolName, errI)
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

	if active, errI := net.IsActive(); !active || errI != nil {
		return nil, nil, fmt.Errorf("GetOrCreateNetwork: network %s is not active: %s", networkName, errI)
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

// GetOrCreateNWFilter create (if necessary) and return a libvirt network filter
func (lv *Libvirt) GetOrCreateNWFilter(filterName string, templateFile string, log *Log) (*libvirt.NWFilter, error) {
	conn, errC := lv.GetConnection()
	if errC != nil {
		return nil, errC
	}

	filter, errL := conn.LookupNWFilterByName(filterName)
	if errL != nil {
		virtErr := errL.(libvirt.Error)
		if virtErr.Domain == libvirt.FROM_NWFILTER && virtErr.Code == libvirt.ERR_NO_NWFILTER {
			log.Info(fmt.Sprintf("nwfilter '%s' not found, it's OK, let's create it", filterName))

			xml, err := ioutil.ReadFile(templateFile)
			if err != nil {
				return nil, fmt.Errorf("GetOrCreateNWFilter: %s: %s", templateFile, err)
			}

			filter, err = conn.NWFilterDefineXML(string(xml))
			if err != nil {
				return nil, fmt.Errorf("GetOrCreateNWFilter: NWFilterDefineXML: %s", err)
			}
		} else {
			return nil, fmt.Errorf("GetOrCreateNetwork: Unexpected error: %s", errL)
		}
	}

	return filter, nil
}

// CloneVolume clones a source volume to a destination volume in the same pool
func (lv *Libvirt) CloneVolume(srcVolName string, srcPool *libvirt.StoragePool, dstVolName string, dstPool *libvirt.StoragePool, dstPoolXML *libvirtxml.StoragePool, volumeTemplateFile string, log *Log) error {
	conn, errC := lv.GetConnection()
	if errC != nil {
		return errC
	}

	err := lv.Pools.Seeds.Refresh(0)
	if err != nil {
		return err
	}

	// find source volume
	volSrc, err := srcPool.LookupStorageVolByName(srcVolName)
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
	volcfg.Name = dstVolName

	volcfg.Target.Path = dstPoolXML.Target.Path + "/" + dstVolName
	// volObj.Target.Format.Type = "raw"

	xml2, err := volcfg.Marshal()
	if err != nil {
		return err
	}
	volDst, err := dstPool.StorageVolCreateXML(string(xml2), 0)
	if err != nil {
		return err
	}
	defer volDst.Free()

	vt, err := volumes.NewVolumeTransfert(conn, volSrc, conn, volDst)
	if err != nil {
		return err
	}

	bytesWritten, err := vt.Copy()
	if err != nil {
		return err
	}

	log.Infof("volume '%s' created from '%s' (transfered %s)", dstVolName, srcVolName, (datasize.ByteSize(bytesWritten) * datasize.B).HR())
	return nil
}

// CreateDiskFromSeed creates a disk (into "disks" pool) from seed image (from "seeds" pool)
func (lv *Libvirt) CreateDiskFromSeed(seed string, disk string, volumeTemplateFile string, log *Log) error {
	return lv.CloneVolume(seed, lv.Pools.Seeds, disk, lv.Pools.Disks, lv.Pools.DisksXML, volumeTemplateFile, log)
}

// UploadFileToLibvirtFromReader uploads a file to libvirt storage
func (lv *Libvirt) UploadFileToLibvirtFromReader(pool *libvirt.StoragePool, poolXML *libvirtxml.StoragePool, template string, sourceRC io.ReadCloser, asName string, log *Log) error {
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

	vu, err := volumes.NewVolumeUploadFromReader(sourceRC, conn, volDst)
	if err != nil {
		return err
	}

	bytesWritten, err := vu.Copy()
	if err != nil {
		return err
	}

	log.Infof("upload to storage pool '%s' as '%s' (transfered %s)", poolXML.Name, asName, (datasize.ByteSize(bytesWritten) * datasize.B).HR())
	return nil
}

// UploadFileToLibvirt is a variant using a file as source
func (lv *Libvirt) UploadFileToLibvirt(pool *libvirt.StoragePool, poolXML *libvirtxml.StoragePool, template string, localSourceFile string, asName string, log *Log) error {
	streamSrc, err := os.Open(localSourceFile)
	if err != nil {
		return err
	}

	return lv.UploadFileToLibvirtFromReader(pool, poolXML, template, streamSrc, asName, log)
}

// VolumeDownloadToWriter return a *VolumeDownload for a download operation to a writer
func (lv *Libvirt) VolumeDownloadToWriter(srcVolName string, pool *libvirt.StoragePool, dst io.WriteCloser) (*volumes.VolumeDownload, error) {
	conn, errC := lv.GetConnection()
	if errC != nil {
		return nil, errC
	}

	err := lv.Pools.Seeds.Refresh(0)
	if err != nil {
		return nil, err
	}

	// find source volume
	vol, err := pool.LookupStorageVolByName(srcVolName)
	if err != nil {
		return nil, err
	}
	defer vol.Free()

	vd, err := volumes.NewVolumeDownloadToWriter(vol, conn, dst)
	if err != nil {
		return nil, err
	}

	return vd, nil
}

// ResizeDisk will change volume ("disk") size
// (do not reduce a volume without knowing what you are doing!)
func (lv *Libvirt) ResizeDisk(disk string, size uint64, pool *libvirt.StoragePool, log *Log) error {

	err := lv.Pools.Seeds.Refresh(0)
	if err != nil {
		return err
	}

	vol, err := pool.LookupStorageVolByName(disk)
	if err != nil {
		return err
	}
	defer vol.Free()

	err = vol.Resize(size, 0)
	if err != nil {
		return err
	}
	log.Infof("disk '%s' resized to %s", disk, (datasize.ByteSize(size) * datasize.B).HR())
	return nil
}

// GetDomainByName returns a domain or nil if domain is not foud.
// Remember to call dom.Free() after use.
func (lv *Libvirt) GetDomainByName(domainName string) (*libvirt.Domain, error) {
	conn, errC := lv.GetConnection()
	if errC != nil {
		return nil, errC
	}

	dom, err := conn.LookupDomainByName(domainName)
	if err != nil {
		errDetails := err.(libvirt.Error)
		if errDetails.Domain == libvirt.FROM_QEMU && errDetails.Code == libvirt.ERR_NO_DOMAIN {
			return nil, nil // not found, but no error
		}
		return nil, err
	}
	return dom, nil
}

// LibvirtDomainStateToString translate a DomainState to string
func LibvirtDomainStateToString(state libvirt.DomainState) string {
	switch state {
	case libvirt.DOMAIN_NOSTATE:
		return "no state"
	case libvirt.DOMAIN_RUNNING:
		return "up"
	case libvirt.DOMAIN_BLOCKED:
		return "blocked on resource"
	case libvirt.DOMAIN_PAUSED:
		return "paused by user"
	case libvirt.DOMAIN_SHUTDOWN:
		return "going down"
	case libvirt.DOMAIN_CRASHED:
		return "crashed"
	case libvirt.DOMAIN_PMSUSPENDED:
		return "sleeping"
	case libvirt.DOMAIN_SHUTOFF:
		return "down"
	default:
		return "unknown"
	}
}

// DeleteVolume for specified pool
func (lv *Libvirt) DeleteVolume(name string, pool *libvirt.StoragePool) error {
	vol, errDef := pool.LookupStorageVolByName(name)
	if errDef != nil {
		return errDef
	}
	defer vol.Free()
	errDef = vol.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
	if errDef != nil {
		return errDef
	}
	return nil
}

// VolumeInfos returns volume informations, like physical allocated size
func (lv *Libvirt) VolumeInfos(name string, pool *libvirt.StoragePool) (*libvirt.StorageVolInfo, error) {
	vol, err := pool.LookupStorageVolByName(name)
	if err != nil {
		return nil, err
	}
	defer vol.Free()
	info, err := vol.GetInfo()
	if err != nil {
		return nil, err
	}
	return info, nil
}

// BackupCompress will TRY to compress backup
func (lv *Libvirt) BackupCompress(volName string, template string, tmpPath string, log *Log) error {
	conn, err := lv.GetConnection()
	if err != nil {
		return err
	}

	infos, err := lv.VolumeInfos(volName, lv.Pools.Backups)
	if err != nil {
		return err
	}

	// is qemu-img available?
	_, err = exec.Command("qemu-img", "-V").CombinedOutput()
	if err != nil {
		log.Error(err.Error())
		log.Info("qemu-img test failure, cancel compression")
		return nil
	}

	tmpfileUncomp, err := ioutil.TempFile(tmpPath, "mulch-backup-uncomp")
	if err != nil {
		return err
	}
	defer os.Remove(tmpfileUncomp.Name())
	tmpfileUncomp.Close()

	tmpfileComp, err := ioutil.TempFile(tmpPath, "mulch-backup-comp")
	if err != nil {
		return err
	}
	defer os.Remove(tmpfileComp.Name())
	tmpfileComp.Close()

	volSrc, err := lv.Pools.Backups.LookupStorageVolByName(volName)
	if err != nil {
		return nil
	}
	defer volSrc.Free()

	vd, err := volumes.NewVolumeDownload(volSrc, conn, tmpfileUncomp.Name())
	if err != nil {
		return err
	}

	_, err = vd.Copy()
	if err != nil {
		return err
	}

	log.Infof("compressing backup")
	output, err := exec.Command("qemu-img", "convert", "-O", "qcow2", "-c", tmpfileUncomp.Name(), tmpfileComp.Name()).CombinedOutput()
	if err != nil {
		log.Error(err.Error())
		log.Error(strings.TrimSpace(string(output)))
		log.Info("qemu-img failure, cancel compression")
		return nil
	}

	err = volSrc.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
	if err != nil {
		return err
	}

	err = lv.UploadFileToLibvirt(
		lv.Pools.Backups,
		lv.Pools.BackupsXML,
		template,
		tmpfileComp.Name(),
		volName,
		log,
	)
	if err != nil {
		return err
	}
	// get new size
	infos2, err := lv.VolumeInfos(volName, lv.Pools.Backups)
	if err != nil {
		return err
	}

	log.Infof("backup compression: from %s to %s",
		(datasize.ByteSize(infos.Allocation) * datasize.B).HR(),
		(datasize.ByteSize(infos2.Allocation) * datasize.B).HR(),
	)

	return nil
}
