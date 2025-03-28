// Copyright 2018 Google LLC All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package registry

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"path"
	"strings"
	"sync"

	"github.com/miguelmota/ipdr/netutil"
)

// Returns whether this url should be handled by the blob handler
// This is complicated because blob is indicated by the trailing path, not the leading path.
// https://github.com/opencontainers/distribution-spec/blob/master/spec.md#pulling-a-layer
// https://github.com/opencontainers/distribution-spec/blob/master/spec.md#pushing-a-layer
func isBlob(req *http.Request) bool {
	elem := strings.Split(req.URL.Path, "/")
	elem = elem[1:]
	if elem[len(elem)-1] == "" {
		elem = elem[:len(elem)-1]
	}
	if len(elem) < 3 {
		return false
	}
	return elem[len(elem)-2] == "blobs" || (elem[len(elem)-3] == "blobs" &&
		elem[len(elem)-2] == "uploads")
}

// blobs
type blobs struct {
	// Blobs are content addresses. we store them globally underneath their sha and make no distinctions per image.
	contents map[string][]byte
	// Each upload gets a unique id that writes occur to until finalized.
	uploads map[string][]byte
	lock    sync.Mutex

	layers map[string][]string

	registry *registry
}

func (b *blobs) handle(resp http.ResponseWriter, req *http.Request) *regError {
	elem := strings.Split(req.URL.Path, "/")
	elem = elem[1:]
	if elem[len(elem)-1] == "" {
		elem = elem[:len(elem)-1]
	}
	// Must have a path of form /v2/{name}/blobs/{upload,sha256:}
	if len(elem) < 4 {
		return &regError{
			Status:  http.StatusBadRequest,
			Code:    "NAME_INVALID",
			Message: "blobs must be attached to a repo",
		}
	}
	target := elem[len(elem)-1]
	service := elem[len(elem)-2]
	digest := req.URL.Query().Get("digest")
	contentRange := req.Header.Get("Content-Range")
	repo := strings.Join(elem[1:len(elem)-2], "/")
	if service == "uploads" {
		repo = strings.Join(elem[1:len(elem)-3], "/")
	}

	if req.Method == "HEAD" {
		b.lock.Lock()
		defer b.lock.Unlock()

		// content is available if image is locally pushed
		if c, ok := b.contents[target]; ok {
			resp.Header().Set("Content-Length", fmt.Sprint(len(c)))
			resp.Header().Set("Docker-Content-Digest", target)
			resp.WriteHeader(http.StatusOK)
			return nil
		}

		// get it if available on IPFS
		cid, err := b.registry.resolveCID(repo, target)
		if err != nil {
			return &regError{
				Status:  http.StatusNotFound,
				Code:    "BLOB_UNKNOWN",
				Message: err.Error(),
			}
		}
		uri := b.registry.ipfsURL([]string{cid, "blobs", target})
		ipfsResp, err := http.Head(uri)
		if err != nil {
			return &regError{
				Status:  http.StatusNotFound,
				Code:    "BLOB_UNKNOWN",
				Message: err.Error(),
			}
		}
		defer ipfsResp.Body.Close()
		if ipfsResp.StatusCode != http.StatusOK {
			return &regError{
				Status:  http.StatusNotFound,
				Code:    "BLOB_UNKNOWN",
				Message: ipfsResp.Status,
			}
		}

		body, err := ioutil.ReadAll(ipfsResp.Body)
		size := len(body)
		if err != nil {
			return &regError{
				Status:  http.StatusNotFound,
				Code:    "BLOB_UNKNOWN",
				Message: err.Error(),
			}
		}

		resp.Header().Set("Content-Length", fmt.Sprint(size))
		resp.Header().Set("Docker-Content-Digest", target)
		resp.WriteHeader(ipfsResp.StatusCode)
		io.CopyN(resp, bytes.NewReader(body), int64(size))

		return nil
	}

	if req.Method == "GET" {
		cid, err := b.registry.resolveCID(repo, target)
		if err != nil {
			return &regError{
				Status:  http.StatusNotFound,
				Code:    "BLOB_UNKNOWN",
				Message: err.Error(),
			}
		}
		uri := b.registry.ipfsURL([]string{cid, "blobs", target})
		ipfsResp, err := netutil.Get(uri)
		if err != nil {
			return &regError{
				Status:  http.StatusNotFound,
				Code:    "BLOB_UNKNOWN",
				Message: err.Error(),
			}
		}
		defer ipfsResp.Body.Close()
		if ipfsResp.StatusCode != http.StatusOK {
			return &regError{
				Status:  http.StatusNotFound,
				Code:    "BLOB_UNKNOWN",
				Message: ipfsResp.Status,
			}
		}

		// TODO ipfsResp.ContentLength could be -1
		// copy the whole body content - not desirable
		body, err := ioutil.ReadAll(ipfsResp.Body)
		size := len(body)
		resp.Header().Set("Content-Length", fmt.Sprint(size))
		resp.Header().Set("Docker-Content-Digest", target)
		resp.WriteHeader(ipfsResp.StatusCode)
		io.CopyN(resp, bytes.NewReader(body), int64(size))

		return nil
	}

	if req.Method == "POST" && target == "uploads" && digest != "" {
		l := &bytes.Buffer{}
		io.Copy(l, req.Body)
		rd := sha256.Sum256(l.Bytes())
		d := "sha256:" + hex.EncodeToString(rd[:])
		if d != digest {
			return &regError{
				Status:  http.StatusBadRequest,
				Code:    "DIGEST_INVALID",
				Message: "digest does not match contents",
			}
		}

		b.lock.Lock()
		defer b.lock.Unlock()
		b.contents[d] = l.Bytes()
		digests := b.layers[repo]
		b.layers[repo] = append(digests, d)
		resp.Header().Set("Docker-Content-Digest", d)
		resp.WriteHeader(http.StatusCreated)
		return nil
	}

	if req.Method == "POST" && target == "uploads" && digest == "" {
		id := fmt.Sprint(rand.Int63())
		resp.Header().Set("Location", "/"+path.Join("v2", path.Join(elem[1:len(elem)-2]...), "blobs/uploads", id))
		resp.Header().Set("Range", "0-0")
		resp.WriteHeader(http.StatusAccepted)
		return nil
	}

	if req.Method == "PATCH" && service == "uploads" && contentRange != "" {
		start, end := 0, 0
		if _, err := fmt.Sscanf(contentRange, "%d-%d", &start, &end); err != nil {
			return &regError{
				Status:  http.StatusRequestedRangeNotSatisfiable,
				Code:    "BLOB_UPLOAD_UNKNOWN",
				Message: "We don't understand your Content-Range",
			}
		}
		b.lock.Lock()
		defer b.lock.Unlock()
		if start != len(b.uploads[target]) {
			return &regError{
				Status:  http.StatusRequestedRangeNotSatisfiable,
				Code:    "BLOB_UPLOAD_UNKNOWN",
				Message: "Your content range doesn't match what we have",
			}
		}
		l := bytes.NewBuffer(b.uploads[target])
		io.Copy(l, req.Body)
		b.uploads[target] = l.Bytes()
		resp.Header().Set("Location", "/"+path.Join("v2", path.Join(elem[1:len(elem)-3]...), "blobs/uploads", target))
		resp.Header().Set("Range", fmt.Sprintf("0-%d", len(l.Bytes())-1))
		resp.WriteHeader(http.StatusNoContent)
		return nil
	}

	if req.Method == "PATCH" && service == "uploads" && contentRange == "" {
		b.lock.Lock()
		defer b.lock.Unlock()
		if _, ok := b.uploads[target]; ok {
			return &regError{
				Status:  http.StatusBadRequest,
				Code:    "BLOB_UPLOAD_INVALID",
				Message: "Stream uploads after first write are not allowed",
			}
		}

		l := &bytes.Buffer{}
		io.Copy(l, req.Body)

		b.uploads[target] = l.Bytes()
		resp.Header().Set("Location", "/"+path.Join("v2", path.Join(elem[1:len(elem)-3]...), "blobs/uploads", target))
		resp.Header().Set("Range", fmt.Sprintf("0-%d", len(l.Bytes())-1))
		resp.WriteHeader(http.StatusNoContent)
		return nil
	}

	if req.Method == "PUT" && service == "uploads" && digest == "" {
		return &regError{
			Status:  http.StatusBadRequest,
			Code:    "DIGEST_INVALID",
			Message: "digest not specified",
		}
	}

	if req.Method == "PUT" && service == "uploads" && digest != "" {
		b.lock.Lock()
		defer b.lock.Unlock()
		l := bytes.NewBuffer(b.uploads[target])
		io.Copy(l, req.Body)
		rd := sha256.Sum256(l.Bytes())
		d := "sha256:" + hex.EncodeToString(rd[:])
		if d != digest {
			return &regError{
				Status:  http.StatusBadRequest,
				Code:    "DIGEST_INVALID",
				Message: "digest does not match contents",
			}
		}

		b.contents[d] = l.Bytes()
		digests := b.layers[repo]
		b.layers[repo] = append(digests, d)
		delete(b.uploads, target)
		resp.Header().Set("Docker-Content-Digest", d)
		resp.WriteHeader(http.StatusCreated)
		return nil
	}

	return &regError{
		Status:  http.StatusBadRequest,
		Code:    "METHOD_UNKNOWN",
		Message: "We don't understand your method + url",
	}
}

func (b *blobs) get(repo string) (map[string][]byte, bool) {
	b.lock.Lock()

	digests, ok := b.layers[repo]
	if !ok {
		return nil, false
	}

	layers := make(map[string][]byte)
	for _, d := range digests {
		blob, ok := b.contents[d]
		if !ok {
			return nil, false
		}
		layers[d] = blob
	}

	b.lock.Unlock()
	return layers, true
}

func (b *blobs) remove(repo string) {
	b.lock.Lock()

	digests, ok := b.layers[repo]
	if !ok {
		return
	}
	delete(b.layers, repo)

	for _, d := range digests {
		delete(b.contents, d)
	}

	b.lock.Unlock()
}
