package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	cid "github.com/ipfs/go-cid"
	mbase "github.com/multiformats/go-multibase"
	mh "github.com/multiformats/go-multihash"
)

func digest(image *Image) string {
	keys := func(m map[string][]byte) []string {
		ks := make([]string, 0, len(m))
		for k := range image.Blobs {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		return ks
	}

	h := sha256.New()
	for _, k := range keys(image.Blobs) {
		h.Write(image.Blobs[k])
	}
	for _, k := range keys(image.Manifests) {
		h.Write(image.Manifests[k])
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// toCIDv1 encodes hash into CIDv1 (different from ipfs add -r)
func toCIDv1(hash string) (string, error) {
	pref := cid.Prefix{
		Version:  1,
		Codec:    cid.Raw,
		MhType:   mh.SHA2_256,
		MhLength: -1, // default length
	}
	c, err := pref.Sum([]byte(hash))
	if err != nil {
		return "", err
	}
	return c.StringOfBase(mbase.Base32)
}

func computeCID(image *Image) (string, error) {
	hash := digest(image)
	return toCIDv1(hash)
}

func splitNameRef(name string) (string, string) {
	sa := strings.SplitN(name, ":", 2)
	if len(sa) == 2 {
		return sa[0], sa[1]
	}
	return sa[0], ""
}

func saveCID(location string, cid string, image *Image) error {
	cidPath := func(name, ref string) string {
		return filepath.Join(location, "cids", name, ":"+ref)
	}

	if image.Name == "" {
		return fmt.Errorf("image has no name")
	}

	if err := os.MkdirAll(filepath.Dir(cidPath(image.Name, "/")), os.ModePerm); err != nil {
		return err
	}
	v := []byte(cid)
	for k := range image.Manifests {
		if strings.HasPrefix(k, "sha256:") {
			continue
		}
		p := cidPath(image.Name, k)
		if err := ioutil.WriteFile(p, v, 0644); err != nil {
			return err
		}
	}
	return nil
}
