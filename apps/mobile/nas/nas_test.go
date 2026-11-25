package nas

import (
	"context"
	"errors"
	"io"
	"slices"
	"testing"
	"time"

	"kando/apps/mobile/store"
	"kando/packages/remotefs"
)

func TestCheckSaysWhatKeepsASourceFromBeingTried(t *testing.T) {
	src := func(kind string, c map[string]string) store.Source { return store.Source{Kind: kind, Config: c} }
	for _, c := range []struct {
		name  string
		src   store.Source
		typed map[string]string
		saved bool
		want  Problem
	}{
		{"smb without a server", src(KindSMB, nil), nil, false, NeedsHost},
		{"smb with a bad port", src(KindSMB, map[string]string{Host: "nas", Port: "70000", User: "me"}), nil, false, NeedsPort},
		{"smb with a path for a share", src(KindSMB, map[string]string{Host: "nas", Share: "a/b", User: "me"}), nil, false, NeedsShare},
		{"smb with a folder and no share", src(KindSMB, map[string]string{Host: "nas", Root: "films", User: "me"}), nil, false, NeedsShareForRoot},
		{"smb without a user", src(KindSMB, map[string]string{Host: "nas", Share: "media"}), nil, false, NeedsUser},
		{"smb whole server", src(KindSMB, map[string]string{Host: "nas", User: "guest"}), nil, false, NoProblem},
		{"new sftp without a secret", src(KindSFTP, map[string]string{Host: "nas", User: "me"}), nil, false, NeedsSecret},
		{"saved sftp keeps its secret", src(KindSFTP, map[string]string{Host: "nas", User: "me"}), nil, true, NoProblem},
		{"sftp with a key", src(KindSFTP, map[string]string{Host: "nas", User: "me"}), map[string]string{PrivateKey: "pem"}, false, NoProblem},
		{"webdav with a query", src(KindWebDAV, map[string]string{URL: "https://nas/dav?x=1"}), nil, false, NeedsURL},
		{"webdav with ftp", src(KindWebDAV, map[string]string{URL: "ftp://nas"}), nil, false, NeedsURL},
		{"webdav", src(KindWebDAV, map[string]string{URL: "http://nas:5005/dav"}), nil, false, NoProblem},
	} {
		if got := Check(c.src, c.typed, c.saved); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
}

// Config keeps what is not secret, a switch only when on.
func TestConfigKeepsNoSecret(t *testing.T) {
	got := Config(KindSMB, map[string]string{Host: " nas ", Password: "pw", NoSigning: "false", Share: "media", "other": "x"})
	if len(got) != 2 || got[Host] != "nas" || got[Share] != "media" {
		t.Errorf("Config = %v", got)
	}
	r := Remote(store.Source{Kind: KindSMB, Config: map[string]string{Host: "nas", NoSigning: "true"}}, map[string]string{Password: "pw"})
	if r.RequireSigning || r.Password != "pw" || r.Kind != remotefs.KindSMB {
		t.Errorf("Remote = %+v", r)
	}
	if r := Remote(store.Source{Kind: KindSMB, Config: map[string]string{Host: "nas"}}, nil); !r.RequireSigning {
		t.Error("an SMB source does without signing by default")
	}
}

func TestListingShowsFoldersThenVideos(t *testing.T) {
	got := Listing([]remotefs.Info{{Name: "b.mkv"}, {Name: "notes.txt"}, {Name: "Zoo", Dir: true}, {Name: ".hidden", Dir: true}, {Name: "A.mp4"}, {Name: "apple", Dir: true}, {Name: "bad\xff.mp4"}})
	var names []string
	for _, i := range got {
		names = append(names, i.Name)
	}
	if want := []string{"apple", "Zoo", "A.mp4", "b.mkv"}; !slices.Equal(names, want) {
		t.Errorf("Listing = %q, want %q", names, want)
	}
}

func TestLocalTellsTheLocalNetwork(t *testing.T) {
	for host, want := range map[string]bool{"192.168.1.10": true, "10.0.0.2": true, "fd00::1": true, "[fe80::1]": true, "127.0.0.1": true, "nas": true, "nas.local": true, "box.lan": true,
		"8.8.8.8": false, "100.64.0.1": false, "dav.example.com": false, "": false} {
		if got := Local(host); got != want {
			t.Errorf("Local(%q) = %v", host, got)
		}
	}
}

// fakeProvider is a provider that lists one name, records the shares it was opened for, and refuses to write, which the Reader never asks it.
type fakeProvider struct {
	remotefs.Provider
	share string
}

func (f fakeProvider) List(context.Context, string) ([]remotefs.Info, error) {
	return []remotefs.Info{{Name: f.share + ".mp4", Size: 1}}, nil
}

func (f fakeProvider) Close() error { return nil }

// An SMB source without a share lists the server's shares at its root, and mounts a share as a path reaches into it.
func TestAWholeSMBServerListsItsShares(t *testing.T) {
	var mounted []string
	d := Dialer{
		Connect: func(_ context.Context, cfg remotefs.Remote) (remotefs.Provider, io.Closer, error) {
			mounted = append(mounted, cfg.Share)
			p := fakeProvider{share: cfg.Share}
			return p, p, nil
		},
		Shares: func(context.Context, remotefs.Remote) ([]string, error) { return []string{"media", "backup"}, nil },
	}
	r, err := d.Open(t.Context(), remotefs.Remote{Kind: remotefs.KindSMB, Host: "nas"})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	top, err := r.List(t.Context(), "")
	if err != nil || len(top) != 2 || !top[0].Dir || top[0].Name != "media" {
		t.Fatalf("root = %+v, %v", top, err)
	}
	in, err := r.List(t.Context(), "media")
	if err != nil || len(in) != 1 || in[0].Name != "media.mp4" || !slices.Equal(mounted, []string{"media"}) {
		t.Fatalf("media = %+v, %v, mounted %q", in, err, mounted)
	}
	if _, err := r.Open(t.Context(), "media", 0, -1); !errors.Is(err, remotefs.ErrIsDir) {
		t.Errorf("opening a share = %v", err)
	}
	if _, err := d.Open(t.Context(), remotefs.Remote{Kind: remotefs.KindSMB, Host: "nas", Root: "films"}); !errors.Is(err, remotefs.ErrInvalidPath) {
		t.Errorf("a folder without a share = %v", err)
	}
}

func TestVersionIsTheETagElseSizeAndTime(t *testing.T) {
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	if got := Version(remotefs.Info{ETag: `"abc"`, Size: 3}); got != `"abc"` {
		t.Errorf("with an ETag: %q", got)
	}
	if a, b := Version(remotefs.Info{Size: 3, Modified: at}), Version(remotefs.Info{Size: 3, Modified: at.Add(time.Second)}); a == b {
		t.Errorf("a file changed keeps its version %q", a)
	}
}
