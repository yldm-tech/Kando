// Package sftptest is an SFTP server on loopback for tests: the storage locations' and the media sources' (docs/media-sources.md 19) connect to it as to a NAS. It imports no package of Kando's.
package sftptest

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// The user Server lets in, with Password or with the private key UserKey.
const (
	User     = "kando"
	Password = "secret"
)

// Server is an SFTP server on loopback over the computer's own files, at Addr, with Home as the user's home folder and a new ed25519 host key, HostKey, a line of authorized_keys. It lets in User with Password or with UserKey.
type Server struct {
	Addr    string
	Home    string
	HostKey string
	UserKey ed25519.PrivateKey

	mu    sync.Mutex
	conns []net.Conn
	dials int
}

// NewKey is a new ed25519 key and its signer.
func NewKey(t testing.TB) (ed25519.PrivateKey, ssh.Signer) {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return key, signer
}

// AuthorizedKey is the public key of s as a line of authorized_keys.
func AuthorizedKey(s ssh.Signer) string {
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(s.PublicKey())))
}

// New starts a Server with a new home folder, which stops as the test ends.
func New(t testing.TB) *Server {
	t.Helper()
	_, host := NewKey(t)
	userKey, user := NewKey(t)
	f := &Server{Home: t.TempDir(), HostKey: AuthorizedKey(host), UserKey: userKey}
	config := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if c.User() == User && string(password) == Password {
				return nil, nil
			}
			return nil, errors.New("wrong password")
		},
		PublicKeyCallback: func(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if c.User() == User && string(key.Marshal()) == string(user.PublicKey().Marshal()) {
				return nil, nil
			}
			return nil, errors.New("unknown key")
		},
	}
	config.AddHostKey(host)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f.Addr = ln.Addr().String()
	var wg sync.WaitGroup
	t.Cleanup(func() {
		ln.Close()
		f.Drop()
		wg.Wait()
	})
	wg.Go(func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			f.mu.Lock()
			f.conns = append(f.conns, conn)
			f.dials++
			f.mu.Unlock()
			wg.Go(func() {
				serveSSH(conn, config, f.Home)
				f.mu.Lock()
				f.conns = slices.DeleteFunc(f.conns, func(c net.Conn) bool { return c == conn })
				f.mu.Unlock()
			})
		}
	})
	return f
}

// HostPort are the server's host and port.
func (f *Server) HostPort() (string, int) {
	host, port, _ := net.SplitHostPort(f.Addr)
	n, _ := strconv.Atoi(port)
	return host, n
}

// serveSSH answers one connection: sessions that ask for the sftp subsystem get an SFTP server.
func serveSSH(conn net.Conn, config *ssh.ServerConfig, home string) {
	defer conn.Close()
	_, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(reqs)
	for nc := range chans {
		if nc.ChannelType() != "session" {
			nc.Reject(ssh.UnknownChannelType, "only sessions")
			continue
		}
		ch, requests, err := nc.Accept()
		if err != nil {
			return
		}
		go func() {
			for req := range requests {
				ok := req.Type == "subsystem" && len(req.Payload) > 4 && string(req.Payload[4:]) == "sftp"
				req.Reply(ok, nil)
				if ok {
					go func() {
						defer ch.Close()
						server, err := sftp.NewServer(ch, sftp.WithServerWorkingDirectory(home))
						if err != nil {
							return
						}
						if err := server.Serve(); err != nil && !errors.Is(err, io.EOF) {
							return
						}
					}()
				}
			}
		}()
	}
}

// Drop cuts every connection, as a server restarting or a network going away does.
func (f *Server) Drop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.conns {
		c.Close()
	}
	f.conns = nil
}

// Open is how many of the connections the server took are still open: neither side closed them yet.
func (f *Server) Open() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.conns)
}

// Dialed is how many connections the server took.
func (f *Server) Dialed() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.dials
}
