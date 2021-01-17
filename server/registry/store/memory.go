package store

import (
	"net/url"
	"sync"
)

type MemoryStore struct {
	images   map[string]*Image
	location string // cids
	sync.RWMutex
}

func NewMemoryStore(uri string) Store {
	u, _ := url.Parse(uri)

	return &MemoryStore{
		images:   make(map[string]*Image),
		location: u.Path,
	}
}

func (r *MemoryStore) Save(image *Image) (string, error) {
	if image == nil || len(image.Manifests) == 0 {
		return "", ErrInvalid
	}
	cid, err := computeCID(image)
	if err != nil {
		return "", err
	}
	r.Lock()
	r.images[cid] = image

	if r.location != "" {
		if err := saveCID(r.location, cid, image); err != nil {
			return "", err
		}
	}
	r.Unlock()
	return cid, nil
}

func (r *MemoryStore) Resolve(name string, ref string) ([]string, error) {
	r.RLock()
	defer r.RUnlock()
	for cid, image := range r.images {
		if image.Name == name {
			if ref == "" {
				var sa []string
				for digest := range image.Manifests {
					sa = append(sa, ":"+digest)
				}
				return sa, nil
			}
			for digest := range image.Manifests {
				if digest == ref {
					return []string{cid}, nil
				}
			}
		}
	}
	return nil, NewNotFoundError(name)
}

func (r *MemoryStore) HasBlob(cid, digest string) bool {
	r.RLock()
	defer r.RUnlock()
	if image, ok := r.images[cid]; ok {
		_, found := image.Blobs[digest]
		return found
	}
	return false
}

func (r *MemoryStore) GetBlob(cid, digest string) ([]byte, error) {
	r.RLock()
	defer r.RUnlock()
	if image, ok := r.images[cid]; ok {
		if data, found := image.Blobs[digest]; found {
			return data, nil
		}
	}
	return nil, NewNotFoundError(cid + ":" + digest)
}

func (r *MemoryStore) GetManifest(cid, reference string) ([]byte, error) {
	r.RLock()
	defer r.RUnlock()
	if image, ok := r.images[cid]; ok {
		if data, found := image.Manifests[reference]; found {
			return data, nil
		}

	}
	return nil, NewNotFoundError(cid + ":" + reference)
}
