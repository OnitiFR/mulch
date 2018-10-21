package common

import "time"

// APISeedStatus expose seed informations
type APISeedStatus struct {
	Name       string
	As         string
	Ready      bool
	CurrentURL string
	Size       uint64
	StatusTime time.Time
	Status     string
}
