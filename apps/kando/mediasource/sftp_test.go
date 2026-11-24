package mediasource

import (
	"context"
	"encoding/pem"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"kando/apps/kando/storage"
	"kando/apps/kando/storage/sftptest"

	"golang.org/x/crypto/ssh"
)

// The SFTP kind is one the Registry takes, a phone can reach it, and Builtin finds it by its ID alone. Its private key is a secret of several lines, and the server's key it trusts is no secret, so settings.json keeps it.
func TestSFTPKindIsValid(t *testing.T) {
	var r Registry
	if err := r.Register(SFTP()); err != nil {
		t.Fatal(err)
	}
	k, ok := Builtin(KindSFTP)
	if !ok || k.ID != KindSFTP || !k.Origin.Builtin() || !k.Phone || !slices.Equal(k.SecretKeys(), []string{SFTPPassword, SFTPPrivateKey, SFTPPassphrase}) {
		t.Fatalf("Builtin(sftp) = %+v, %v", k, ok)
	}
	for _, f := range k.Fields {
		if f.Title("en") == "" || f.Title("zh-Hans") == "" {
			t.Errorf("field %q has no title", f.Key)
		}
		switch f.Key {
		case SFTPPrivateKey:
			if f.Type != FieldSecret || !f.Multiline {
				t.Errorf("the private key is %+v", f)
			}
		case SFTPHostKey:
			if f.Type != FieldHostKey {
				t.Errorf("the host key is %+v", f)
			}
		}
	}
}

// An SFTP source's fields and secrets become storage's Remote: a port that is not one is SFTP's own, the key is trimmed and the passphrase kept as typed.
func TestSFTPRemote(t *testing.T) {
	s := Source{ID: "0123456789abcdef", Kind: KindSFTP, Config: map[string]string{
		SFTPHost: " nas.local ", SFTPPort: "2222", SFTPRoot: "films", SFTPUser: " kando ", SFTPHostKey: " ssh-ed25519 AAAA ",
	}}
	got := SFTPRemote(s, map[string]string{SFTPPassword: "pa55", SFTPPrivateKey: "\nKEY\n", SFTPPassphrase: " phrase "})
	want := storage.Remote{Kind: storage.KindSFTP, Host: "nas.local", Port: 2222, Root: "films", User: "kando", Password: "pa55", PrivateKey: "KEY", Passphrase: " phrase ", HostKey: "ssh-ed25519 AAAA"}
	if got != want {
		t.Fatalf("SFTPRemote = %+v, want %+v", got, want)
	}
	if r, ok := Remote(s, map[string]string{SFTPPassword: "pa55"}); !ok || r.Password != "pa55" || r.Kind != storage.KindSFTP {
		t.Fatalf("Remote = %+v, %v", r, ok)
	}
	for _, port := range []string{"", "ssh", "0", "70000"} {
		s.Config[SFTPPort] = port
		if r := SFTPRemote(s, nil); r.Port != 0 {
			t.Errorf("port %q = %d", port, r.Port)
		}
	}
	if _, ok := Remote(Source{Kind: PluginKindID("kando.webdav", "webdav")}, nil); ok {
		t.Fatal("a plugin's source has a storage Remote")
	}
}

// sftpSource is a source of the folder root of f's home folder, trusting key.
func sftpSource(f *sftptest.Server, root, key string) Source {
	host, port := f.HostPort()
	return Source{ID: "0123456789abcdef", Kind: KindSFTP, Name: "server", Config: map[string]string{
		SFTPHost: host, SFTPPort: strconv.Itoa(port), SFTPRoot: root, SFTPUser: sftptest.User, SFTPHostKey: key,
	}}
}

// The SFTP kind connects only to a server whose key the source trusts: without one, or with another, it fails with the key the server offered, which trusted lets it read the folder, signed in with the password or the private key.
func TestSFTPOpens(t *testing.T) {
	ctx := context.Background()
	f := sftptest.New(t)
	if err := os.MkdirAll(filepath.Join(f.Home, "films", "Heat"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.Home, "films", "Heat", "video.mp4"), []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	password := map[string]string{SFTPPassword: sftptest.Password}

	_, err := SFTP().Open(ctx, sftpSource(f, "films", ""), password)
	var hk *storage.HostKeyError
	if !errors.Is(err, storage.ErrUnknownHostKey) || !errors.As(err, &hk) || hk.Key != f.HostKey {
		t.Fatalf("without a key trusted = %v", err)
	}
	_, other := sftptest.NewKey(t)
	if _, err := SFTP().Open(ctx, sftpSource(f, "films", sftptest.AuthorizedKey(other)), password); !errors.Is(err, storage.ErrHostKeyMismatch) {
		t.Fatalf("with another key trusted = %v", err)
	}

	plain, err := ssh.MarshalPrivateKey(f.UserKey, "")
	if err != nil {
		t.Fatal(err)
	}
	locked, err := ssh.MarshalPrivateKeyWithPassphrase(f.UserKey, "", []byte("phrase"))
	if err != nil {
		t.Fatal(err)
	}
	signIns := map[string]map[string]string{
		"password":              password,
		"key":                   {SFTPPrivateKey: string(pem.EncodeToMemory(plain))},
		"key with a passphrase": {SFTPPrivateKey: string(pem.EncodeToMemory(locked)), SFTPPassphrase: "phrase"},
	}
	for name, secrets := range signIns {
		d, err := SFTP().Open(ctx, sftpSource(f, "films", f.HostKey), secrets)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		entries, err := d.List(ctx, "Heat")
		if err != nil || len(entries) != 1 || entries[0].Name != "video.mp4" || entries[0].Size != 10 || entries[0].Version == "" {
			t.Fatalf("%s: List = %+v, %v", name, entries, err)
		}
		if e, err := d.Stat(ctx, "Heat"); err != nil || !e.Dir {
			t.Fatalf("%s: Stat = %+v, %v", name, e, err)
		}
		rc, err := d.Open(ctx, "Heat/video.mp4", 2, 3)
		if err != nil {
			t.Fatalf("%s: Open: %v", name, err)
		}
		got, err := io.ReadAll(rc)
		rc.Close()
		if err != nil || string(got) != "234" {
			t.Fatalf("%s: read %q, %v", name, got, err)
		}
		if _, err := d.Stat(ctx, "../.."); !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("%s: a path out of the root = %v", name, err)
		}
		if err := d.Close(); err != nil {
			t.Fatalf("%s: Close: %v", name, err)
		}
	}
	if _, err := SFTP().Open(ctx, sftpSource(f, "films", f.HostKey), map[string]string{SFTPPassword: "wrong"}); storage.Diagnose(err) != storage.FailSignIn {
		t.Fatalf("a wrong password = %v", err)
	}
}
