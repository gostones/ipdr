package net

import (
	"fmt"
	"github.com/miguelmota/ipdr/ipfs"
	"io/ioutil"
	"net"
	"path/filepath"
	"sort"
	"strings"
)

// CIDResolver is the interface that maps container image repo[:reference] to content ID.
type CIDResolver interface {
	Resolve(repo string, reference string) ([]string, error)
}

// lookup resolves dnslink similar to the following
// https://github.com/ipfs/go-dnslink
func lookup(domain string) (string, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if !strings.HasPrefix(domain, "_dnslink.") {
		domain = "_dnslink." + domain
	}
	txts, err := net.LookupTXT(domain)
	if err != nil {
		return "", err
	}
	for _, txt := range txts {
		if txt != "" {
			if strings.HasPrefix(txt, "dnslink=") {
				txt = string(txt[8:])
			}
			return txt, nil
		}
	}
	return "", fmt.Errorf("invalid TXT record")
}

// File resolver
type fileResolver struct {
	root string
}

func NewFileResolver(uri string) (CIDResolver, error) {
	p := filepath.Clean(strings.TrimPrefix(uri, "file:"))
	return &fileResolver{
		root: p,
	}, nil
}

func (r *fileResolver) Resolve(repo, reference string) ([]string, error) {
	if reference == "" {
		files, err := ioutil.ReadDir(fmt.Sprintf("%s/%s", r.root, repo))
		if err != nil {
			return nil, err
		}
		var sa []string
		for _, f := range files {
			// if f.Mode().IsRegular() {
			// 	sa = append(sa, f.Name())
			// }
			sa = append(sa, f.Name())
		}
		return sa, nil
	}

	b, err := ioutil.ReadFile(fmt.Sprintf("%s/%s/:%s", r.root, repo, reference))
	if err != nil {
		return nil, err
	}
	return []string{strings.TrimSpace(string(b))}, nil
}

// DNSLink resolver
// https://docs.ipfs.io/concepts/dnslink/
type dnslinkResolver struct {
	resolver CIDResolver
}

func NewDNSLinkResolver(client *ipfs.Client, domain string) (CIDResolver, error) {
	var r CIDResolver
	txt, err := lookup(domain)
	if err != nil {
		return nil, err
	}
	switch {
	case strings.HasPrefix(txt, "file:"):
		r, err = NewFileResolver(txt)
		if err != nil {
			return nil, err
		}
	case strings.HasPrefix(txt, "/ipfs/"):
		r, err = NewIPFSResolver(client, txt)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("not supported: %s", txt)
	}

	return &dnslinkResolver{
		resolver: r,
	}, nil
}

func (r *dnslinkResolver) Resolve(repo, reference string) ([]string, error) {
	return r.resolver.Resolve(repo, reference)
}

// IPFS resolver
type ipfsResolver struct {
	client *ipfs.Client
	cid    string
}

func NewIPFSResolver(client *ipfs.Client, root string) (CIDResolver, error) {
	return &ipfsResolver{
		client: client,
		cid:    strings.TrimRight(strings.TrimPrefix(root, "/ipfs/"), "/"), // /ipfs/<cid>
	}, nil
}

func (r *ipfsResolver) Resolve(repo string, reference string) ([]string, error) {
	if reference == "" {
		links, err := r.client.List(fmt.Sprintf("%s/%s", r.cid, repo))
		if err != nil {
			return nil, err
		}
		var sa []string
		for _, l := range links {
			sa = append(sa, l.Name)
		}
		return sa, nil
	}

	b, err := r.readCID(repo, reference)
	if err != nil {
		return nil, err
	}
	return []string{strings.TrimSpace(string(b))}, nil
}

func (r *ipfsResolver) readCID(repo, reference string) ([]byte, error) {
	return r.getContent(fmt.Sprintf("%s/%s/:%s", r.cid, repo, reference))
}

func (r *ipfsResolver) getContent(path string) ([]byte, error) {
	rd, err := r.client.Cat(path)
	if err != nil {
		return nil, err
	}
	defer rd.Close()
	return ioutil.ReadAll(rd)
}

// SCP resolver
type scpResolver struct {
	shell *ScpShell
}

func NewScpResolver(uri string) (CIDResolver, error) {
	return &scpResolver{
		shell: NewScpShell(uri),
	}, nil
}

func (r *scpResolver) Resolve(repo string, reference string) ([]string, error) {
	if reference == "" {
		sa, err := r.shell.List(repo)
		if err != nil {
			return nil, err
		}
		return sa, nil
	}

	b, err := r.shell.ReadFile(filepath.Join(repo, ":"+reference))
	if err != nil {
		return nil, err
	}
	return []string{strings.TrimSpace(string(b))}, nil
}

type resolver struct {
	resolvers []CIDResolver
}

func NewResolver(client *ipfs.Client, list []string) CIDResolver {
	var resolvers []CIDResolver
	for _, l := range list {
		switch {
		case strings.HasPrefix(l, "file:"):
			if r, err := NewFileResolver(l); err == nil {
				resolvers = append(resolvers, r)
			}
		case strings.HasPrefix(l, "scp:"):
			if r, err := NewScpResolver(l); err == nil {
				resolvers = append(resolvers, r)
			}
		case strings.HasPrefix(l, "/ipfs/"):
			if r, err := NewIPFSResolver(client, l); err == nil {
				resolvers = append(resolvers, r)
			}
		default:
			// assume dnslink
			if r, err := NewDNSLinkResolver(client, l); err == nil {
				resolvers = append(resolvers, r)
			}
		}
	}

	return &resolver{
		resolvers: resolvers,
	}
}

// collect all results if reference is empty for listing
func (r *resolver) Resolve(repo string, reference string) ([]string, error) {
	var list []string
	for _, re := range r.resolvers {
		result, err := re.Resolve(repo, reference)
		if err != nil {
			return nil, err
		}
		if result != nil {
			// return early
			if reference != "" {
				return result, nil
			}
			list = append(list, result...)
		}
	}
	list = uniq(list)
	sort.Strings(list)
	return list, nil
}

func uniq(sa []string) []string {
	keys := make(map[string]bool)
	list := []string{}
	for _, s := range sa {
		if _, ok := keys[s]; !ok {
			keys[s] = true
			list = append(list, s)
		}
	}
	return list
}
