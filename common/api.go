package common

import (
	"net/url"
	"path"
	"strings"
)

// CleanURL clean the URL using path.Clean rules
func CleanURL(urlIn string) (string, error) {
	urlObj, err := url.Parse(urlIn)
	if err != nil {
		return urlIn, err
	}
	urlObj.Path = path.Clean(urlObj.Path)
	return urlObj.String(), nil
}

// RemoveAPIKeyFromString replace any API key appearance in the string
func RemoveAPIKeyFromString(in string, key string) string {
	if key == "" {
		return in
	}
	return strings.Replace(in, key, "xxx", -1)
}
