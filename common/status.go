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

// APIOperation is currently exactly matching Operation struct
type APIOperation struct {
	Origin        string
	Action        string
	Ressource     string
	RessourceName string
	StartTime     time.Time
}

type APIOrigin struct {
	Name string
	Type string
	Path string
}

// APIStatus describes host status
type APIStatus struct {
	StartTime          time.Time
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
	Origins            []APIOrigin
	SSHConnections     []APISSHConnection
	Operations         []APIOperation
}
