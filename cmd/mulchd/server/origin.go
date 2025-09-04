package server

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/transport/ssh"
	"github.com/go-git/go-git/v6/storage/memory"
)

const (
	// git cache expires 30 seconds after last use
	OriginGitCacheExpiration = 30 * time.Second

	// maximum git cache life
	OriginGitCacheMaxLife = 10 * time.Minute
)

type Origins struct {
	Origins map[string]*Origin
}

type Origin struct {
	Log      *Log
	Config   *ConfigOrigin
	gitCache *OriginGitCache
}

type OriginGitCache struct {
	createdDate  time.Time
	lastUsedDate time.Time
	fs           billy.Filesystem
}

// NewOrigins creates a new Origin list
func NewOrigins(app *App) *Origins {
	origins := make(map[string]*Origin)
	for name, origin := range app.Config.Origins {
		origins[name] = &Origin{
			Config: origin,
			Log:    app.Log,
		}
	}

	return &Origins{
		Origins: origins,
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
			return o.getContentFromOrigin(o.Origins[origin], subpath)
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

	if _, exists := o.Origins[origin]; !exists {
		return "", "", fmt.Errorf("origin %s not found", origin)
	}

	return origin, strings.Join(parts[1:], "/"), nil
}

// getContentFromOrigin returns a ReadCloser thru the provided origin
func (*Origins) getContentFromOrigin(origin *Origin, pathStr string) (io.ReadCloser, error) {

	switch origin.Config.Type {
	case OriginTypeHTTP:
		u, err := url.Parse(origin.Config.Path)
		if err != nil {
			return nil, err
		}
		u.Path = path.Join(u.Path, pathStr)
		return getContentFromHttpURL(u.String())
	case OriginTypeFile:
		u, err := url.Parse(origin.Config.Path)
		if err != nil {
			return nil, err
		}
		u.Path = path.Join(u.Path, pathStr)
		return getContentFromFileURL(u.String())
	case OriginTypeGIT:
		return getContentFromGitOrigin(origin, pathStr)
	default:
		return nil, fmt.Errorf("origin type '%s' not implemented", origin.Config.Type)
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
func getContentFromGitOrigin(origin *Origin, pathStr string) (io.ReadCloser, error) {
	originConf := origin.Config

	if origin.gitCache != nil {
		// check if the git cache has expired
		if origin.gitCache.lastUsedDate.Add(OriginGitCacheExpiration).Before(time.Now()) ||
			origin.gitCache.createdDate.Add(OriginGitCacheMaxLife).Before(time.Now()) {
			// thread safe: opened file handles are not closed
			origin.gitCache = nil
			origin.Log.Tracef("git cache invalidated for origin '%s'", originConf.Name)
		}
	}

	// no cache? let's create one
	// thread safe: worst case, the cache is created twice and one expires directly
	if origin.gitCache == nil {
		options := &git.CloneOptions{
			URL:           originConf.Path,
			Depth:         1,
			SingleBranch:  true,
			ReferenceName: plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", originConf.Branch)),
		}

		// go-git default is to use ssh-agent
		if originConf.SSHKeyFile != "" {
			sshKey, err := os.ReadFile(originConf.SSHKeyFile)
			if err != nil {
				return nil, err
			}

			publicKey, keyError := ssh.NewPublicKeys("git", []byte(sshKey), "")
			if keyError != nil {
				return nil, keyError
			}
			options.Auth = publicKey
		}

		origin.Log.Tracef("creating git cache for origin '%s'", originConf.Name)

		fs := memfs.New()

		start := time.Now()
		_, err := git.Clone(memory.NewStorage(), fs, options)
		if err != nil {
			return nil, err
		}
		origin.Log.Tracef("git cache created for origin '%s', cloned in %s", originConf.Name, time.Since(start))

		origin.gitCache = &OriginGitCache{
			createdDate: time.Now(),
			fs:          fs,
		}
	}

	origin.gitCache.lastUsedDate = time.Now()

	fp, err := origin.gitCache.fs.Open(pathStr)
	if err != nil {
		return nil, err
	}

	return fp, nil
}
