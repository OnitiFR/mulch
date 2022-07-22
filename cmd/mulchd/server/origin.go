package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
)

type Origins struct {
	Config map[string]*ConfigOrigin
}

func NewOrigins(app *App) *Origins {
	return &Origins{
		Config: app.Config.Origins,
	}
}

//GetContent returns a ReadCloser to the file at the given URL/path
// Caller must Close() the returned value.
func (o *Origins) GetContent(path string) (io.ReadCloser, error) {
	scheme, err := GetURLScheme(path)

	if err != nil {
		return nil, err
	}

	if scheme == "" {
		origin, subpath, err := o.getOriginFromPath(path)
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
	// TODO: delete this, should only be available with an origin
	case "ssh":
		dotGitPos := strings.Index(path, ".git")
		if dotGitPos == -1 {
			return nil, fmt.Errorf("%s : only git ssh protocol is currently implemented", path)
		}
		return getContentFromGithubURL(path)
	default:
		return nil, fmt.Errorf("unsupported protocol %s", scheme)
	}
}

// getOriginFromPath returns the origin, the subpath and an error if any
// If the path does not use an origin, it returns an empty origin and no error
func (o *Origins) getOriginFromPath(path string) (string, string, error) {
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
	}

	return nil, fmt.Errorf("origin type %d not implemented, origin %s, path %s", origin.Type, origin.Name, pathStr)
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
