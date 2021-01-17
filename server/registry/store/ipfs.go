package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"strings"

	api "github.com/ipfs/go-ipfs-api"
	files "github.com/ipfs/go-ipfs-files"
)

type IpfsStore struct {
	client   *Client
	location string // local cids
}

func NewIpfsStore(uri string) Store {
	u, _ := url.Parse(uri)

	client := NewClient(u.Host)
	return &IpfsStore{
		client:   client,
		location: u.Path,
	}
}

func (r *IpfsStore) Save(image *Image) (string, error) {
	if image == nil || len(image.Manifests) == 0 {
		return "", ErrInvalid
	}

	cid, err := r.client.AddImage(image.Manifests, image.Blobs)

	// TODO ipfs add
	if r.location != "" {
		saveCID(r.location, cid, image)
	}
	return cid, err
}

func (r *IpfsStore) Resolve(name string, ref string) ([]string, error) {
	return nil, ErrNotSupported
}

func (r *IpfsStore) HasBlob(cid, digest string) bool {
	// if cid is known, the content must exist on IPFS.
	// we check any way
	p := r.blobPath(cid, digest)
	_, err := r.client.List(p)
	return err == nil
}

func (r *IpfsStore) GetBlob(cid, digest string) ([]byte, error) {
	p := r.blobPath(cid, digest)
	return r.getContent(p)
}

func (r *IpfsStore) GetManifest(cid, reference string) ([]byte, error) {
	p := r.manifestPath(cid, reference)
	return r.getContent(p)
}

func (r *IpfsStore) getContent(path string) ([]byte, error) {
	rd, err := r.client.Cat(path)
	if err != nil {
		return nil, err
	}
	defer rd.Close()
	return ioutil.ReadAll(rd)
}

func (r *IpfsStore) blobPath(cid, digest string) string {
	return r.ipfsURL(cid, []string{"blobs", digest})
}

func (r *IpfsStore) manifestPath(cid, reference string) string {
	return r.ipfsURL(cid, []string{"manifests", reference})
}

func (r *IpfsStore) ipfsURL(cid string, s []string) string {
	return fmt.Sprintf("%s/%s", cid, strings.Join(s, "/"))
}

// Client is the IPFS client
type Client struct {
	client *api.Shell
}

type object struct {
	Hash string
}

// AddImage adds components of an image recursively
func (client *Client) AddImage(manifest map[string][]byte, layers map[string][]byte) (string, error) {
	mf := make(map[string]files.Node)
	for k, v := range manifest {
		mf[k] = files.NewBytesFile(v)
	}

	bf := make(map[string]files.Node)
	for k, v := range layers {
		bf[k] = files.NewBytesFile(v)
	}

	sf := files.NewMapDirectory(map[string]files.Node{
		"blobs":     files.NewMapDirectory(bf),
		"manifests": files.NewMapDirectory(mf),
	})
	slf := files.NewSliceDirectory([]files.DirEntry{files.FileEntry("image", sf)})

	reader := files.NewMultiFileReader(slf, true)
	resp, err := client.client.Request("add").
		Option("recursive", true).
		Option("cid-version", 1).
		Body(reader).
		Send(context.Background())
	if err != nil {
		return "", err
	}

	defer resp.Close()

	if resp.Error != nil {
		return "", resp.Error
	}

	dec := json.NewDecoder(resp.Output)
	var final string
	for {
		var out object
		err = dec.Decode(&out)
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}
		final = out.Hash
	}

	if final == "" {
		return "", errors.New("no results received")
	}

	return final, nil
}

// Cat the content at the given path. Callers need to drain and close the returned reader after usage.
func (client *Client) Cat(path string) (io.ReadCloser, error) {
	return client.client.Cat(path)
}

// List entries at the given path
func (client *Client) List(path string) ([]*api.LsLink, error) {
	return client.client.List(path)
}

func NewClient(host string) *Client {
	client := api.NewShell(host)
	return &Client{
		client: client,
	}
}
