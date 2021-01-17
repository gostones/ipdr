package store

import (
	"errors"
	"net/url"
	// "os"
)

type Manifests map[string][]byte
type Blobs map[string][]byte

type Image struct {
	Name      string
	Manifests Manifests
	Blobs     Blobs
}

func NewImage(name string, manifests map[string][]byte, blobs map[string][]byte) *Image {
	return &Image{
		Name:      name,
		Manifests: manifests,
		Blobs:     blobs,
	}
}

type Store interface {
	Save(image *Image) (string, error)
	Resolve(name, reference string) ([]string, error)
	HasBlob(cid, digest string) bool
	GetBlob(cid, digest string) ([]byte, error)
	GetManifest(cid, reference string) ([]byte, error)
}

// NotFoundError not found
type NotFoundError struct {
	Name string
	Err  error
}

func (e *NotFoundError) Error() string {
	return e.Name + " not found"
}

func (e *NotFoundError) Unwrap() error {
	return e.Err
}

func NewNotFoundError(name string) error {
	return &NotFoundError{
		Name: name,
	}
}

var ErrNotSupported = errors.New("not suppored")

var ErrInvalid = errors.New("invalid")

// type DummyStore struct {
// }

// func NewDummyStore() Store {
// 	return &DummyStore{}
// }
// func (r *DummyStore) Save(image *Image) (string, error) {
// 	return "", nil
// }
// func (r *DummyStore) Resolve(repo string, reference string) ([]string, error) {
// 	return nil, NewNotFoundError(name)
// }
// func (r *DummyStore) HasBlob(cid, digest string) bool {
// 	return false
// }
// func (r *DummyStore) GetBlob(cid, digest string) ([]byte, error) {
// 	return nil, nil
// }
// func (r *DummyStore) GetManifest(cid, reference string) ([]byte, error) {
// 	return nil, nil
// }

// Create store
func Create(s string) Store {
	var imageStore Store
	u, _ := url.Parse(s)
	switch u.Scheme {
	case "memory":
		imageStore = NewMemoryStore(s)
	case "ipfs", "":
		imageStore = NewIpfsStore(s)
	case "file":
		imageStore = NewFileStore(s)
	case "scp":
		imageStore = NewScpStore(s)
	}
	return imageStore
}
