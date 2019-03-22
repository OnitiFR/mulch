package common

// APIStatus describes host status
type APIStatus struct {
	HostCPUs           int
	HostMemoryTotalMB  int
	VMs                int
	ActiveVMs          int
	VMCPUs             int
	VMActiveCPUs       int
	VMMemMB            int
	VMActiveMemMB      int
	FreeStorageMB      int
	FreeBackupMB       int
	ProvisionedDisksMB int
	AllocatedDisksMB   int
}
