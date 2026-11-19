package storage

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// fakeSSH is an SFTP server on loopback over the computer's own files, with home as the user's home folder and a new ed25519 host key, that lets in kando with the password secret or with userKey.
type fakeSSH struct {
	addr    string
	home    string
	hostKey string
	userKey ed25519.PrivateKey

	mu    sync.Mutex
	conns []net.Conn
	dials int
}

func newKey(t *testing.T) (ed25519.PrivateKey, ssh.Signer) {
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

func authorizedKey(s ssh.Signer) string {
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(s.PublicKey())))
}

func newFakeSSH(t *testing.T) *fakeSSH {
	t.Helper()
	_, host := newKey(t)
	userKey, user := newKey(t)
	f := &fakeSSH{home: t.TempDir(), hostKey: authorizedKey(host), userKey: userKey}
	config := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if c.User() == "kando" && string(password) == "secret" {
				return nil, nil
			}
			return nil, errors.New("wrong password")
		},
		PublicKeyCallback: func(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if c.User() == "kando" && string(key.Marshal()) == string(user.PublicKey().Marshal()) {
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
	f.addr = ln.Addr().String()
	var wg sync.WaitGroup
	t.Cleanup(func() {
		ln.Close()
		f.drop()
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
			wg.Go(func() { serveSSH(conn, config, f.home) })
		}
	})
	return f
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

// drop cuts every connection, as a server restarting or a network going away does.
func (f *fakeSSH) drop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.conns {
		c.Close()
	}
	f.conns = nil
}

func (f *fakeSSH) dialed() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.dials
}

func (f *fakeSSH) remote(root string) Remote {
	host, port, _ := net.SplitHostPort(f.addr)
	n, _ := strconv.Atoi(port)
	return Remote{Kind: KindSFTP, Host: host, Port: n, Root: root, User: "kando", Password: "secret", HostKey: f.hostKey}
}

// sftpProvider is a provider over the home folder of a new fakeSSH. The root is relative, as a Windows path is not one an SFTP server takes as absolute.
func sftpProvider(t *testing.T) (Provider, *fakeSSH) {
	t.Helper()
	f := newFakeSSH(t)
	p, closer, err := OpenRemote(context.Background(), f.remote(""))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closer.Close() })
	return p, f
}

// Without a trusted host key no connection is made, and the error carries the key the server offered, which once trusted lets the provider in: trust on first use.
func TestSFTPTrustsAHostKeyOnlyOnceItIsGiven(t *testing.T) {
	ctx := context.Background()
	f := newFakeSSH(t)
	cfg := f.remote("")
	cfg.HostKey = ""
	_, _, err := OpenRemote(ctx, cfg)
	var hk *HostKeyError
	if !errors.Is(err, ErrUnknownHostKey) || !errors.As(err, &hk) {
		t.Fatalf("no host key = %v", err)
	}
	if hk.Key != f.hostKey || hk.Trusted != "" || hk.Host != "127.0.0.1" || !strings.HasPrefix(hk.Fingerprint, "SHA256:") {
		t.Fatalf("offered %+v, want %s", hk, f.hostKey)
	}
	cfg.HostKey = hk.Key
	p, closer, err := OpenRemote(ctx, cfg)
	if err != nil {
		t.Fatalf("with the offered key trusted: %v", err)
	}
	defer closer.Close()
	write(t, p, "a.txt", "a")

	_, other := newKey(t)
	cfg.HostKey = authorizedKey(other)
	_, _, err = OpenRemote(ctx, cfg)
	if !errors.Is(err, ErrHostKeyMismatch) || !errors.As(err, &hk) || hk.Key != f.hostKey || hk.Trusted != cfg.HostKey {
		t.Fatalf("another key = %v (%+v)", err, hk)
	}
	cfg.HostKey = "not a key"
	if _, _, err := OpenRemote(ctx, cfg); err == nil {
		t.Fatal("a host key that is not one was taken")
	}
}

func TestSFTPSignsInWithAPasswordOrAKey(t *testing.T) {
	ctx := context.Background()
	f := newFakeSSH(t)
	dir := f.home
	plain, err := ssh.MarshalPrivateKey(f.userKey, "")
	if err != nil {
		t.Fatal(err)
	}
	locked, err := ssh.MarshalPrivateKeyWithPassphrase(f.userKey, "", []byte("phrase"))
	if err != nil {
		t.Fatal(err)
	}
	keyOnly := f.remote("")
	keyOnly.Password = ""
	keyOnly.PrivateKey = string(pem.EncodeToMemory(plain))
	lockedKey := keyOnly
	lockedKey.PrivateKey, lockedKey.Passphrase = string(pem.EncodeToMemory(locked)), "phrase"
	for name, cfg := range map[string]Remote{"password": f.remote(""), "key": keyOnly, "key with a passphrase": lockedKey} {
		p, closer, err := OpenRemote(ctx, cfg)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		write(t, p, name+".txt", name)
		closer.Close()
		if body, err := os.ReadFile(dir + "/" + name + ".txt"); err != nil || string(body) != name {
			t.Fatalf("%s: wrote %q, %v", name, body, err)
		}
	}

	wrong := f.remote("")
	wrong.Password = "wrong"
	if _, _, err := OpenRemote(ctx, wrong); err == nil {
		t.Fatal("a wrong password got in")
	}
	noPhrase := lockedKey
	noPhrase.Passphrase = ""
	if _, _, err := OpenRemote(ctx, noPhrase); err == nil {
		t.Fatal("a locked key was read without its passphrase")
	}
	nothing := f.remote("")
	nothing.Password = ""
	if _, _, err := OpenRemote(ctx, nothing); err == nil {
		t.Fatal("connected with neither a password nor a key")
	}
	missing := f.remote("missing")
	if _, _, err := OpenRemote(ctx, missing); !errors.Is(err, ErrNotExist) {
		t.Fatalf("a missing root = %v", err)
	}
}

// A connection the server cut is made again for the next operation.
func TestSFTPConnectsAgainAfterTheConnectionBroke(t *testing.T) {
	ctx := context.Background()
	p, f := sftpProvider(t)
	write(t, p, "Heat/video.mp4", "video")
	if n := f.dialed(); n != 1 {
		t.Fatalf("dialed %d times", n)
	}
	f.drop()
	if got := read(t, p, "Heat/video.mp4", 0, -1); got != "video" {
		t.Fatalf("after the cut = %q", got)
	}
	f.drop()
	write(t, p, "Heat/video.srt", "subs")
	if got := names(t, p, "Heat"); strings.Join(got, ",") != "video.mp4,video.srt" {
		t.Fatalf("lists %q", got)
	}
	if n := f.dialed(); n != 3 {
		t.Fatalf("dialed %d times, want 3", n)
	}
	p.(io.Closer).Close()
	if _, err := p.Stat(ctx, "Heat"); !errors.Is(err, errClosed) {
		t.Fatalf("after Close = %v", err)
	}
}

// The root may be relative to the user's home folder, or absolute.
func TestSFTPRootRelativeToHomeOrAbsolute(t *testing.T) {
	f := newFakeSSH(t)
	if err := os.Mkdir(f.home+"/films", 0o755); err != nil {
		t.Fatal(err)
	}
	roots := map[string]string{"films": f.home + "/films"}
	if runtime.GOOS != "windows" {
		// An absolute path of this server on Windows reads "/C:/...", which is the server's way and not one to test here.
		abs := t.TempDir()
		roots[abs+"/"] = abs
	}
	for root, dir := range roots {
		p, closer, err := OpenRemote(context.Background(), f.remote(root))
		if err != nil {
			t.Fatalf("%s: %v", root, err)
		}
		write(t, p, "a.txt", root)
		closer.Close()
		if body, err := os.ReadFile(dir + "/a.txt"); err != nil || string(body) != root {
			t.Fatalf("%s: wrote %q, %v", root, body, err)
		}
	}
}
