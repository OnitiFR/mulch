package common

type APISecretUsageEntries []APISecretUsageEntry

type APISecretUsageEntry struct {
	Key         string
	LocalCount  int
	RemoteCount int
}
