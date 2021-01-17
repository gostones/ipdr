package store

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/miguelmota/ipdr/server/registry/net"
)

// ScpStore uses scp.
type ScpStore struct {
	shell    *net.ScpShell
	resolver net.CIDResolver
}

// NewScpStore creates a remote store via ssh
// IPDR_STORE=scp://user[:pass]@host:port/path
func NewScpStore(uri string) Store {
	u, _ := url.Parse(uri)
	u.Path = filepath.Join(u.Path, "cids")
	resolver, _ := net.NewScpResolver(u.String())

	return &ScpStore{
		shell:    net.NewScpShell(uri),
		resolver: resolver,
	}
}

func (r *ScpStore) String() string {
	return fmt.Sprintf("%s", r.shell)
}

func (r *ScpStore) Save(image *Image) (string, error) {
	if image == nil || len(image.Manifests) == 0 {
		return "", ErrInvalid
	}

	return SendImage(r.shell, "/", image)
}

func (r *ScpStore) Resolve(name string, ref string) ([]string, error) {
	return r.resolver.Resolve(name, ref)
}

func (r *ScpStore) HasBlob(cid, digest string) bool {
	return r.shell.FileExist(filepath.Join(cid, "blobs", digest))
}

func (r *ScpStore) GetBlob(cid, digest string) ([]byte, error) {
	return r.shell.ReadFile(filepath.Join(cid, "blobs", digest))
}

func (r *ScpStore) GetManifest(cid, reference string) ([]byte, error) {
	return r.shell.ReadFile(filepath.Join(cid, "manifests", reference))
}

func SendImage(shell *net.ScpShell, location string, image *Image) (string, error) {
	cid, err := computeCID(image)
	if err != nil {
		return "", err
	}

	blobPath := func(digest string) string {
		return filepath.Join(location, cid, "blobs", digest)
	}

	manifestPath := func(reference string) string {
		return filepath.Join(location, cid, "manifests", reference)
	}

	cidPath := func(name, reference string) string {
		return filepath.Join(location, "cids", name, ":"+reference)
	}

	_, err = shell.Do(func(t *net.Transfer) ([]byte, error) {
		now := time.Now()
		mode := os.FileMode(0644)

		mkdir := func() error {
			dirs := []string{
				manifestPath("/"),
				blobPath("/"),
				filepath.Dir(cidPath(image.Name, "/")),
			}
			return t.Mkdir(dirs...)
		}

		write := func(p string, v []byte) error {
			info := net.NewFileInfo(p, int64(len(v)), mode, now, now)
			return t.Send(info, bytes.NewBuffer(v), p)
		}

		// prepare directories
		if err := mkdir(); err != nil {
			return nil, err
		}

		for k, v := range image.Manifests {
			p := manifestPath(k)
			if err := write(p, v); err != nil {
				return nil, err
			}
		}

		for k, v := range image.Blobs {
			p := blobPath(k)
			if err := write(p, v); err != nil {
				return nil, err
			}
		}

		if image.Name != "" {
			for k := range image.Manifests {
				if strings.HasPrefix(k, "sha256:") {
					continue
				}
				p := cidPath(image.Name, k)
				if err := write(p, []byte(cid)); err != nil {
					return nil, err
				}
			}
		}
		return nil, nil
	})

	if err != nil {
		return "", err
	}
	return cid, nil
}
