package remotefs

import (
	"context"
	"encoding/pem"
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"

	"kando/packages/remotefs/sftptest"

	"golang.org/x/crypto/ssh"
)

// sftpRemote is how to reach the folder root of f's home folder as its user, with the password, its key trusted.
func sftpRemote(f *sftptest.Server, root string) Remote {
	host, port := f.HostPort()
	return Remote{Kind: KindSFTP, Host: host, Port: port, Root: root, User: sftptest.User, Password: sftptest.Password, HostKey: f.HostKey}
}

// sftpProvider is a provider over the home folder of a new fakeSSH. The root is relative, as a Windows path is not one an SFTP server takes as absolute.
func sftpProvider(t *testing.T) (Provider, *sftptest.Server) {
	t.Helper()
	f := sftptest.New(t)
	p, closer, err := OpenRemote(context.Background(), sftpRemote(f, ""))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closer.Close() })
	return p, f
}

// Without a trusted host key no connection is made, and the error carries the key the server offered, which once trusted lets the provider in: trust on first use.
func TestSFTPTrustsAHostKeyOnlyOnceItIsGiven(t *testing.T) {
	ctx := context.Background()
	f := sftptest.New(t)
	cfg := sftpRemote(f, "")
	cfg.HostKey = ""
	_, _, err := OpenRemote(ctx, cfg)
	var hk *HostKeyError
	if !errors.Is(err, ErrUnknownHostKey) || !errors.As(err, &hk) {
		t.Fatalf("no host key = %v", err)
	}
	if hk.Key != f.HostKey || hk.Trusted != "" || hk.Host != "127.0.0.1" || !strings.HasPrefix(hk.Fingerprint, "SHA256:") {
		t.Fatalf("offered %+v, want %s", hk, f.HostKey)
	}
	cfg.HostKey = hk.Key
	p, closer, err := OpenRemote(ctx, cfg)
	if err != nil {
		t.Fatalf("with the offered key trusted: %v", err)
	}
	defer closer.Close()
	write(t, p, "a.txt", "a")

	_, other := sftptest.NewKey(t)
	cfg.HostKey = sftptest.AuthorizedKey(other)
	_, _, err = OpenRemote(ctx, cfg)
	if !errors.Is(err, ErrHostKeyMismatch) || !errors.As(err, &hk) || hk.Key != f.HostKey || hk.Trusted != cfg.HostKey {
		t.Fatalf("another key = %v (%+v)", err, hk)
	}
	cfg.HostKey = "not a key"
	if _, _, err := OpenRemote(ctx, cfg); err == nil {
		t.Fatal("a host key that is not one was taken")
	}
}

func TestSFTPSignsInWithAPasswordOrAKey(t *testing.T) {
	ctx := context.Background()
	f := sftptest.New(t)
	dir := f.Home
	plain, err := ssh.MarshalPrivateKey(f.UserKey, "")
	if err != nil {
		t.Fatal(err)
	}
	locked, err := ssh.MarshalPrivateKeyWithPassphrase(f.UserKey, "", []byte("phrase"))
	if err != nil {
		t.Fatal(err)
	}
	keyOnly := sftpRemote(f, "")
	keyOnly.Password = ""
	keyOnly.PrivateKey = string(pem.EncodeToMemory(plain))
	lockedKey := keyOnly
	lockedKey.PrivateKey, lockedKey.Passphrase = string(pem.EncodeToMemory(locked)), "phrase"
	for name, cfg := range map[string]Remote{"password": sftpRemote(f, ""), "key": keyOnly, "key with a passphrase": lockedKey} {
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

	wrong := sftpRemote(f, "")
	wrong.Password = "wrong"
	if _, _, err := OpenRemote(ctx, wrong); err == nil {
		t.Fatal("a wrong password got in")
	}
	noPhrase := lockedKey
	noPhrase.Passphrase = ""
	if _, _, err := OpenRemote(ctx, noPhrase); err == nil {
		t.Fatal("a locked key was read without its passphrase")
	}
	nothing := sftpRemote(f, "")
	nothing.Password = ""
	if _, _, err := OpenRemote(ctx, nothing); err == nil {
		t.Fatal("connected with neither a password nor a key")
	}
	missing := sftpRemote(f, "missing")
	if _, _, err := OpenRemote(ctx, missing); !errors.Is(err, ErrNotExist) {
		t.Fatalf("a missing root = %v", err)
	}
}

// A connection the server cut is made again for the next operation.
func TestSFTPConnectsAgainAfterTheConnectionBroke(t *testing.T) {
	ctx := context.Background()
	p, f := sftpProvider(t)
	write(t, p, "Heat/video.mp4", "video")
	if n := f.Dialed(); n != 1 {
		t.Fatalf("dialed %d times", n)
	}
	f.Drop()
	if got := read(t, p, "Heat/video.mp4", 0, -1); got != "video" {
		t.Fatalf("after the cut = %q", got)
	}
	f.Drop()
	write(t, p, "Heat/video.srt", "subs")
	if got := names(t, p, "Heat"); strings.Join(got, ",") != "video.mp4,video.srt" {
		t.Fatalf("lists %q", got)
	}
	if n := f.Dialed(); n != 3 {
		t.Fatalf("dialed %d times, want 3", n)
	}
	p.(io.Closer).Close()
	if _, err := p.Stat(ctx, "Heat"); !errors.Is(err, errClosed) {
		t.Fatalf("after Close = %v", err)
	}
}

// The root may be relative to the user's home folder, or absolute.
func TestSFTPRootRelativeToHomeOrAbsolute(t *testing.T) {
	f := sftptest.New(t)
	if err := os.Mkdir(f.Home+"/films", 0o755); err != nil {
		t.Fatal(err)
	}
	roots := map[string]string{"films": f.Home + "/films"}
	if runtime.GOOS != "windows" {
		// An absolute path of this server on Windows reads "/C:/...", which is the server's way and not one to test here.
		abs := t.TempDir()
		roots[abs+"/"] = abs
	}
	for root, dir := range roots {
		p, closer, err := OpenRemote(context.Background(), sftpRemote(f, root))
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
