package registry

import (
	// "io/ioutil"
	// "os"
	// "path/filepath"
	// "strings"
	"sync"
)

// cidCache contains known cid entries.
type cidCache struct {
	// maps repo:tag -> cid
	cids     map[string]string
	location string

	sync.RWMutex
}

func key(repo, ref string) string {
	return repo + ":" + ref
}

// func ( r *cidCache) persist() bool {
// 	return r.location != ""
// }

func (r *cidCache) Add(repo, reference string, cid string) {
	r.Lock()

	k := key(repo, reference)

	r.cids[k] = cid

	// // only store name:tag reference
	// if repo != cid && !strings.HasPrefix(reference, "sha256:") && r.persist() {
	// 	r.writeCID(k, cid)
	// }

	r.Unlock()
}

func (r *cidCache) Get(repo, reference string) (string, bool) {
	r.RLock()

	k := key(repo, reference)

	val, ok := r.cids[k]
	// if !ok  && r.persist() {
	// 	if v, err := r.readCID(k); err == nil {
	// 		val = v
	// 		ok = true
	// 	}
	// }

	r.RUnlock()
	return val, ok
}

// func (r *cidCache) readCID(key string) (string, error) {
// 	p := r.cidPath(key)
// 	content, err := ioutil.ReadFile(p)
// 	if err != nil {
// 		return "", err
// 	}
// 	return string(content), nil
// }

// func (r *cidCache) writeCID(key string, val string) error {
// 	p := r.cidPath(key)
// 	if err := os.MkdirAll(filepath.Dir(p), os.ModePerm); err != nil {
// 		return err
// 	}
// 	return ioutil.WriteFile(p, []byte(val), 0644)
// }

// func (r *cidCache) cidPath(key string) string {
// 	pc := strings.SplitN(key, ":", 2)
// 	p := filepath.Join(r.location, pc[0], ":"+pc[1])
// 	return p
// }

func newCidCache() *cidCache {
	return &cidCache{
		cids: make(map[string]string),
		// location: location,
	}
}
