package store

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/miguelmota/ipdr/server/registry/net"
)

// FileStore layout
// <location>/<name>/
//                  blobs/
//                       <digest>
//                       ...
//                  manifests/
//                       <reference>
//                       ...
type FileStore struct {
	location string
	resolver net.CIDResolver
}

// NewFileStore creates a local file system store
// file:/path
func NewFileStore(uri string) Store {
	u, _ := url.Parse(uri)
	resolver, _ := net.NewFileResolver("file:" + filepath.Join(u.Path, "cids"))
	return &FileStore{
		location: u.Path,
		resolver: resolver,
	}
}

func (r *FileStore) String() string {
	return fmt.Sprintf("file:%s", r.location)
}

func (r *FileStore) Save(image *Image) (string, error) {
	if image == nil || len(image.Manifests) == 0 {
		return "", ErrInvalid
	}

	cid, err := computeCID(image)
	if err != nil {
		return "", err
	}

	if err := saveImage(r.location, cid, image); err != nil {
		return "", err
	}

	if err := saveCID(r.location, cid, image); err != nil {
		return "", err
	}

	return cid, nil
}

func (r *FileStore) Resolve(name string, ref string) ([]string, error) {
	return r.resolver.Resolve(name, ref)
}

func (r *FileStore) HasBlob(cid, digest string) bool {
	return r.check(r.blobPath(cid, digest))
}

func (r *FileStore) GetBlob(cid, digest string) ([]byte, error) {
	return r.read(r.blobPath(cid, digest))
}

func (r *FileStore) GetManifest(cid, reference string) ([]byte, error) {
	return r.read(r.manifestPath(cid, reference))
}

func (r *FileStore) read(p string) ([]byte, error) {
	content, err := ioutil.ReadFile(p)
	if err != nil {
		return nil, err
	}
	return content, nil
}

func (r *FileStore) check(p string) bool {
	if _, err := os.Stat(p); err == nil {
		return true
	} else if os.IsNotExist(err) {
		return false
	}
	// file may or may not exist
	return false
}

func (r *FileStore) cidPath(name, reference string) string {
	return filepath.Join(r.location, "cids", name, ":"+reference)
}

func (r *FileStore) blobPath(cid, digest string) string {
	return filepath.Join(r.location, cid, "blobs", digest)
}

func (r *FileStore) manifestPath(cid, reference string) string {
	return filepath.Join(r.location, cid, "manifests", reference)
}

func LoadImage(location string) (*Image, error) {
	dir := func(p string) ([]string, error) {
		files, err := ioutil.ReadDir(p)
		if err != nil {
			return nil, err
		}
		ns := []string{}
		for _, f := range files {
			if !f.IsDir() {
				ns = append(ns, f.Name())
			}
		}
		return ns, nil
	}

	manifests := make(Manifests)
	blobs := make(Blobs)

	mfiles, err := dir(filepath.Join(location, "manifests"))
	if err != nil {
		return nil, err
	}
	if len(mfiles) == 0 {
		return nil, ErrInvalid
	}

	var hash string
	for _, n := range mfiles {
		b, err := ioutil.ReadFile(filepath.Join(location, "manifests", n))
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(n, "sha256:") {
			hash = n
		}
		manifests[n] = b
	}

	bfiles, err := dir(filepath.Join(location, "blobs"))
	if err != nil {
		return nil, err
	}
	if len(bfiles) == 0 || hash == "" {
		return nil, ErrInvalid
	}
	for _, n := range bfiles {
		b, err := ioutil.ReadFile(filepath.Join(location, "blobs", n))
		if err != nil {
			return nil, err
		}
		blobs[n] = b
	}
	return &Image{
		Blobs:     blobs,
		Manifests: manifests,
	}, nil
}

func saveImage(location string, cid string, image *Image) error {
	blobPath := func(cid, digest string) string {
		return filepath.Join(location, cid, "blobs", digest)
	}

	manifestPath := func(cid, reference string) string {
		return filepath.Join(location, cid, "manifests", reference)
	}

	if err := os.MkdirAll(manifestPath(cid, "/"), os.ModePerm); err != nil {
		return err
	}
	for k, v := range image.Manifests {
		p := manifestPath(cid, k)
		if err := ioutil.WriteFile(p, v, 0644); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(blobPath(cid, "/"), os.ModePerm); err != nil {
		return err
	}

	for k, v := range image.Blobs {
		p := blobPath(cid, k)
		if err := ioutil.WriteFile(p, v, 0644); err != nil {
			return err
		}
	}

	return nil
}
