package server

import (
	"errors"
	"io"
	"math/rand"
	"net/http"
	"net/url"
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

// GetURLScheme returns the scheme of the given URL
func GetURLScheme(urlStr string) (string, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}
	return u.Scheme, nil
}

// CopyHttpFlush
func CopyHttpFlush(dst io.Writer, src io.Reader) (written int64, err error) {

	size := 32 * 1024
	buf := make([]byte, size)

	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])

			if f, ok := dst.(http.Flusher); ok {
				f.Flush()
			}

			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = errors.New("invalid write result")
				}
			}
			written += int64(nw)
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	return written, err
}
