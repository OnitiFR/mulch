package main

import (
	"fmt"
	"os"
	"path"
)

// LEProductionString is the magic string to use production LE directory
const LEProductionString = "LETS_ENCRYPT_PRODUCTION"

// InitCertCache will check mode of the given certificate storage path
// and create it if necessary
func InitCertCache(certPath string) (string, error) {
	cacheDir := path.Clean(certPath)

	stat, err := os.Stat(cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			errM := os.Mkdir(cacheDir, 0700)
			if errM != nil {
				return "", errM
			}
			return cacheDir, nil
		}
		return "", err
	}

	if stat.IsDir() == false {
		return "", fmt.Errorf("%s is not a directory", cacheDir)
	}

	if stat.Mode() != os.ModeDir|os.FileMode(0700) {
		fmt.Println(stat.Mode())
		return "", fmt.Errorf("%s: only the owner should be able to read/write this directory (mode 0700)", cacheDir)
	}

	return cacheDir, nil
}
