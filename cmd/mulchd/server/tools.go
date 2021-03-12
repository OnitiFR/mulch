package server

import (
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"regexp"
)

// IsValidName returns true if argument use only allowed chars for a name
func IsValidName(token string) bool {
	match, _ := regexp.MatchString("^[A-Za-z0-9_]*$", token)
	return match
}

// IsValidGroupName returns true if group is a valid group name (@ + isValidName)
func IsValidGroupName(group string) bool {
	match, _ := regexp.MatchString("^@[A-Za-z0-9_]*$", group)
	return match
}

// IsValidWord returns true if argument use only allowed chars for a name
func IsValidWord(token string) bool {
	match, _ := regexp.MatchString("^[A-Za-z0-9_-]*$", token)
	return match
}

// RandString generate a random string of A-Za-z0-9 runes
func RandString(n int, rand *rand.Rand) string {
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

// GetContentFromURL returns a ReadCloser to the file at the given URL
// Caller must Close() the returned value.
func GetContentFromURL(url string) (io.ReadCloser, error) {
	if len(url) > 7 && url[:7] == "file://" {
		filename := url[7:]
		file, err := os.Open(filename)
		if err != nil {
			return nil, err
		}
		return file, nil
	}

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("response was %s (%v)", resp.Status, resp.StatusCode)
	}

	return resp.Body, nil
}
