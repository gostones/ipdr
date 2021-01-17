package net

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	scp "github.com/hnakamur/go-scp"
	"golang.org/x/crypto/ssh"
)

// the following binaries are required on the remote system.
const (
	mkdirBin = "mkdir -p"
	lsBin    = "ls -1"
	scpBin   = "scp"
)

// ScpShell for transferring files via ssh.
type ScpShell struct {
	addr   string
	path   string
	config *ssh.ClientConfig
}

type Transfer struct {
	conn *ssh.Client
	cli  *scp.SCP
	path string
}

type FileInfo = scp.FileInfo

func NewFileInfo(name string, size int64, mode os.FileMode, modTime, accessTime time.Time) *FileInfo {
	return scp.NewFileInfo(name, size, mode, modTime, accessTime)
}

func (r *Transfer) Send(info *FileInfo, rd io.Reader, filename string) error {
	p := filepath.Join(r.path, filename)
	return r.cli.Send(info, ioutil.NopCloser(rd), p)
}

func (r *Transfer) Receive(filename string, w io.Writer) (*FileInfo, error) {
	p := filepath.Join(r.path, filename)
	return r.cli.Receive(p, w)
}

func (r *Transfer) Mkdir(dirs ...string) error {
	for _, dir := range dirs {
		cmd := fmt.Sprintf("%s %s", mkdirBin, filepath.Join(r.path, dir))
		if _, err := runCommand(cmd, r.conn); err != nil {
			return err
		}
	}
	return nil
}

// NewScpShell parses URL and reads env for authentication.
// scp://user[:pass]@host:port/path
// IPDR_PRIVATE_KEY=
// IPDR_ID_FILE=$HOME/.ssh/id_rsa
func NewScpShell(uri string) *ScpShell {
	u, _ := url.Parse(uri)

	var auth []ssh.AuthMethod
	if pass, ok := u.User.Password(); ok {
		auth = []ssh.AuthMethod{
			ssh.Password(pass),
		}
	} else {
		auth = []ssh.AuthMethod{
			publicKeys(),
		}
	}

	config := &ssh.ClientConfig{
		User:            u.User.Username(),
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         60 * time.Second,
	}
	return &ScpShell{
		addr:   u.Host,
		path:   u.Path,
		config: config,
	}
}

func (r *ScpShell) String() string {
	return fmt.Sprintf("scp://%s@%s%s", r.config.User, r.addr, r.path)
}

// Dial and recover from panic if any
func (r *ScpShell) Dial() (cli *ssh.Client, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("invalid ssh config. %v", r)
		}
	}()
	cli, err = ssh.Dial("tcp", r.addr, r.config)
	return
}

func (r *ScpShell) Connect(cb func(conn *ssh.Client) ([]byte, error)) ([]byte, error) {
	conn, err := r.Dial()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	b, err := cb(conn)
	return b, err
}

func (r *ScpShell) Do(cb func(*Transfer) ([]byte, error)) ([]byte, error) {
	b, err := r.Connect(func(conn *ssh.Client) ([]byte, error) {
		cli := scp.NewSCP(conn)
		cli.SCPCommand = scpBin
		return cb(&Transfer{
			conn: conn,
			cli:  cli,
			path: r.path,
		})
	})
	return b, err
}

func (r *ScpShell) FileExist(filename string) bool {
	_, err := r.Connect(func(conn *ssh.Client) ([]byte, error) {
		cmd := fmt.Sprintf("%s %s", lsBin, filepath.Join(r.path, filename))
		return runCommand(cmd, conn)
	})
	return err == nil
}

func (r *ScpShell) List(filename string) ([]string, error) {
	split := func(s string) []string {
		var sa []string
		sc := bufio.NewScanner(strings.NewReader(s))
		for sc.Scan() {
			sa = append(sa, sc.Text())
		}
		return sa
	}
	b, err := r.Connect(func(conn *ssh.Client) ([]byte, error) {
		cmd := fmt.Sprintf("%s %s", lsBin, filepath.Join(r.path, filename))
		return runCommand(cmd, conn)
	})
	if err != nil {
		return nil, err
	}
	return split(string(b)), nil
}

func (r *ScpShell) ReadFile(filename string) ([]byte, error) {
	b, err := r.Do(func(t *Transfer) ([]byte, error) {
		var buf bytes.Buffer
		_, err := t.Receive(filename, &buf)
		if err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	})
	return b, err
}

// CopyDirs transfers local directories to the remote directory recursively.
func (r *ScpShell) CopyDirs(remote string, local ...string) error {
	_, err := r.Do(func(t *Transfer) ([]byte, error) {
		for _, l := range local {
			if err := t.cli.SendDir(l, filepath.Join(r.path, remote), nil); err != nil {
				return nil, err
			}
		}
		return nil, nil
	})
	return err
}

func publicKeys() ssh.AuthMethod {
	// $HOME/.ssh/id_rsa
	defaultFile := func() string {
		usr, err := user.Current()
		if err != nil {
			return ""
		}
		return filepath.Join(usr.HomeDir, ".ssh", "id_rsa")
	}
	readKey := func() []byte {
		file := os.Getenv("IPDR_ID_FILE")
		if file == "" {
			file = defaultFile()
		}
		key, err := ioutil.ReadFile(file)
		if err != nil {
			return nil
		}
		return key
	}
	key := []byte(os.Getenv("IPDR_PRIVATE_KEY"))
	if len(key) == 0 {
		key = readKey()
	}

	if key == nil {
		return nil
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil
	}
	return ssh.PublicKeys(signer)
}

func runCommand(cmd string, conn *ssh.Client) ([]byte, error) {
	sess, err := conn.NewSession()
	if err != nil {
		return nil, err
	}
	defer sess.Close()

	var stdout bytes.Buffer
	sess.Stdout = &stdout
	if err := sess.Run(cmd); err != nil {
		return nil, err
	}

	return stdout.Bytes(), nil
}
