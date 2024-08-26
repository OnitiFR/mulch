package common

import "time"

// APITrustListEntries is a list of entries for "trust list"
type APITrustListEntries []APITrustListEntry

// APITrustListEntry is an entry for "trust list"
type APITrustListEntry struct {
	VM          string
	Fingerprint string
	AddedAt     time.Time
}
