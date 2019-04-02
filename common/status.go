package common

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
}
