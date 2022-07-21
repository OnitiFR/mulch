package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
)

// GetContentFromURL returns a ReadCloser to the file at the given URL
// Caller must Close() the returned value.
func GetContentFromURL(urlStr string) (io.ReadCloser, error) {
	scheme, err := GetURLScheme(urlStr)

	if err != nil {
		return nil, err
	}

	switch scheme {
	case "http", "https":
		return getContentFromHttpURL(urlStr)
	case "file":
		return getContentFromFileURL(urlStr)
	case "ssh":
		dotGitPos := strings.Index(urlStr, ".git")
		if dotGitPos == -1 {
			return nil, fmt.Errorf("%s : only git ssh protocol is currently implemented", urlStr)
		}
		return getContentFromGithubURL(urlStr)
	default:
		return nil, fmt.Errorf("unsupported protocol %s", scheme)
	}
}

// file:// protocol
func getContentFromFileURL(url string) (io.ReadCloser, error) {
	filename := url[7:]
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	return file, nil
}

// http:// or https:// protocol
func getContentFromHttpURL(url string) (io.ReadCloser, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("response was %s (%v)", resp.Status, resp.StatusCode)
	}

	return resp.Body, nil
}

// git ssh:// protocol (git:// is a basic non-authenticated protocol)
func getContentFromGithubURL(urlStr string) (io.ReadCloser, error) {
	dotGitPos := strings.Index(urlStr, ".git")
	host := urlStr[:dotGitPos+4]
	uri := urlStr[dotGitPos+4:]

	parts := strings.Split(uri, "/")

	if len(parts) < 3 {
		return nil, errors.New("invalid github url, ex: ssh://git@github.com/user/repo.git/master/test.sh")
	}

	branch := parts[1]
	file := "/" + strings.Join(parts[2:], "/")

	fs := memfs.New()
	_, err := git.Clone(memory.NewStorage(), fs, &git.CloneOptions{
		URL:           host,
		Depth:         1,
		SingleBranch:  true,
		ReferenceName: plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", branch)),
	})

	if err != nil {
		return nil, err
	}

	fp, err := fs.Open(file)
	if err != nil {
		panic(err)
	}

	return fp, nil
}
