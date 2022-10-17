package common

import "time"

type APISecretListEntries []APISecretListEntry

type APISecretListEntry struct {
	Key       string
	Modified  time.Time
	AuthorKey string
}
