package server

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/go-git/go-git/v5/storage/memory"
)

// TODO: add a cache system (for GIT at least)

type Origins struct {
	Config map[string]*ConfigOrigin
}

func NewOrigins(app *App) *Origins {
	return &Origins{
		Config: app.Config.Origins,
	}
}

// GetContent returns a ReadCloser to the file at the given URL/path
// - caller must Close() the returned value
func (o *Origins) GetContent(path string) (io.ReadCloser, error) {
	scheme, err := GetURLScheme(path)

	if err != nil {
		return nil, err
	}

	if scheme == "" {
		origin, subpath, err := o.GetOriginFromPath(path)
		if err != nil {
			return nil, err
		}

		if origin != "" {
			return o.getContentFromOrigin(o.Config[origin], subpath)
		} else {
			return nil, fmt.Errorf("invalid path %s (no scheme, no origin)", path)
		}
	}

	switch scheme {
	case "http", "https":
		return getContentFromHttpURL(path)
	case "file":
		return getContentFromFileURL(path)
	default:
		return nil, fmt.Errorf("unsupported protocol '%s'", scheme)
	}
}

// GetOriginFromPath returns the origin, the subpath and an error if any
// - if the path does not use an origin, it returns an empty origin and no error
func (o *Origins) GetOriginFromPath(path string) (string, string, error) {
	if len(path) < 1 || path[0] != '{' {
		return "", "", nil
	}

	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return "", "", nil
	}

	part0 := parts[0]
	if part0[len(part0)-1:] != "}" {
		return "", "", nil
	}

	origin := part0[1 : len(part0)-1]

	if _, exists := o.Config[origin]; !exists {
		return "", "", fmt.Errorf("origin %s not found", origin)
	}

	return origin, strings.Join(parts[1:], "/"), nil
}

// getContentFromOrigin returns a ReadCloser thru the provided origin
func (*Origins) getContentFromOrigin(origin *ConfigOrigin, pathStr string) (io.ReadCloser, error) {

	switch origin.Type {
	case OriginTypeHTTP:
		u, err := url.Parse(origin.Path)
		if err != nil {
			return nil, err
		}
		u.Path = path.Join(u.Path, pathStr)
		return getContentFromHttpURL(u.String())
	case OriginTypeFile:
		u, err := url.Parse(origin.Path)
		if err != nil {
			return nil, err
		}
		u.Path = path.Join(u.Path, pathStr)
		return getContentFromFileURL(u.String())
	case OriginTypeGIT:
		return getContentFromGitOrigin(origin, pathStr)
	default:
		return nil, fmt.Errorf("origin type '%s' not implemented", origin.Type)
	}
}

// file:// protocol (also accepts file paths: /home/user/file.txt)
func getContentFromFileURL(url string) (io.ReadCloser, error) {
	filename := url
	if strings.HasPrefix(url, "file://") {
		filename = url[7:]
	}
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

// returns a content using a git origin
func getContentFromGitOrigin(origin *ConfigOrigin, pathStr string) (io.ReadCloser, error) {
	options := &git.CloneOptions{
		URL:           origin.Path,
		Depth:         1,
		SingleBranch:  true,
		ReferenceName: plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", origin.Branch)),
	}

	// go-git default is to use ssh-agent
	if origin.SSHKeyFile != "" {
		sshKey, err := ioutil.ReadFile(origin.SSHKeyFile)
		if err != nil {
			return nil, err
		}

		publicKey, keyError := ssh.NewPublicKeys("git", []byte(sshKey), "")
		if keyError != nil {
			return nil, keyError
		}
		options.Auth = publicKey
	}

	fs := memfs.New()

	fmt.Println("Cloning", origin.Path, "to", origin.Branch)
	_, err := git.Clone(memory.NewStorage(), fs, options)
	if err != nil {
		return nil, err
	}
	fmt.Println("Cloned")

	fp, err := fs.Open(pathStr)
	if err != nil {
		return nil, err
	}

	return fp, nil
}
