package common

import (
	"os"
)

// PathExist returns true if a file or directory exists
func PathExist(path string) bool {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}

	// I know Stat() may fail for a lot of reasons, but
	// os.IsNotExist is not enough, see ENOTDIR for
	// things like /etc/passwd/test
	if err != nil {
		return false
	}

	return true
}
