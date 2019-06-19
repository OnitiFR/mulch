package common

import "time"

// APISSHConnection describes a proxied SSH connection to a VM
type APISSHConnection struct {
	FromIP    string
	FromUser  string
	ToUser    string
	ToVMName  string
	StartTime time.Time
}

// APIStatus describes host status
type APIStatus struct {
	VMs                int
	ActiveVMs          int
	HostCPUs           int
	VMCPUs             int
	VMActiveCPUs       int
	HostMemoryTotalMB  int
	VMMemMB            int
	VMActiveMemMB      int
	FreeStorageMB      int
	FreeBackupMB       int
	ProvisionedDisksMB int
	AllocatedDisksMB   int
	SSHConnections     []APISSHConnection
}
