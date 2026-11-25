package mediasource

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/storage"
	"kando/packages/remotefs"
)

// The SMB kind is one the Registry takes, and Builtin finds it by its ID alone.
func TestSMBKindIsValid(t *testing.T) {
	var r Registry
	if err := r.Register(SMB()); err != nil {
		t.Fatal(err)
	}
	k, ok := Builtin(KindSMB)
	if !ok || k.ID != KindSMB || !k.Origin.Builtin() || !slices.Equal(k.SecretKeys(), []string{SMBPassword}) {
		t.Fatalf("Builtin(smb) = %+v, %v", k, ok)
	}
	if _, ok := Builtin("nfs"); ok {
		t.Fatal("Builtin knows nfs, which this version does not register")
	}
	for _, f := range k.Fields {
		if f.Title("en") == "" || f.Title("zh-Hans") == "" {
			t.Errorf("field %q has no title", f.Key)
		}
	}
}

// An SMB source's fields become remotefs's Remote: signing is required unless the source says its NAS does not support it, and a port that is not one is SMB's own.
func TestSMBRemote(t *testing.T) {
	s := Source{ID: "0123456789abcdef", Kind: KindSMB, Config: map[string]string{
		SMBHost: " nas.local ", SMBShare: "/media/", SMBRoot: "films", SMBUser: "kando", SMBDomain: "WORKGROUP", SMBPort: "4450",
	}}
	got := SMBRemote(s, "pa55")
	want := remotefs.Remote{Kind: remotefs.KindSMB, Host: "nas.local", Port: 4450, Share: "media", Root: "films", User: "kando", Domain: "WORKGROUP", Password: "pa55", RequireSigning: true}
	if got != want {
		t.Fatalf("SMBRemote = %+v, want %+v", got, want)
	}
	s.Config[SMBNoSigning] = "true"
	s.Config[SMBPort] = "seventy"
	if got := SMBRemote(s, ""); got.RequireSigning || got.Port != 0 {
		t.Fatalf("without signing and a port %+v", got)
	}
}

// fakeShare is one share of a fake SMB server: a storage.Local over a folder of the test's, which counts its closes.
type fakeShare struct {
	remotefs.Provider
	closed *int
}

func (f fakeShare) Close() error {
	*f.closed++
	return nil
}

// A source without a share reads the whole server: its root lists the shares, a path's first name is the share it is in, mounted once as a path first reaches it, and Close unmounts them all. Nothing is written.
func TestSMBWithoutAShareReadsEveryShare(t *testing.T) {
	dirs := map[string]string{"media": t.TempDir(), "backup": t.TempDir()}
	ctx := context.Background()
	if err := storage.NewLocal(dirs["media"]).Write(ctx, "films/Heat.mkv", strings.NewReader("heat"), 4); err != nil {
		t.Fatal(err)
	}
	var mounts []string
	closed := 0
	open := func(_ context.Context, cfg remotefs.Remote) (remotefs.Provider, io.Closer, error) {
		dir, ok := dirs[cfg.Share]
		if !ok {
			return nil, nil, remotefs.ErrNotExist
		}
		if !cfg.RequireSigning || cfg.Password != "pa55" {
			t.Errorf("mounted with %+v", cfg)
		}
		mounts = append(mounts, cfg.Share)
		share := fakeShare{Provider: storage.NewLocal(dir), closed: &closed}
		return share, share, nil
	}
	shares := func(context.Context, remotefs.Remote) ([]string, error) { return []string{"backup", "media"}, nil }
	d, err := openSMB(ctx, remotefs.Remote{Host: "nas.local", User: "kando", Password: "pa55", RequireSigning: true}, open, shares)
	if err != nil {
		t.Fatal(err)
	}
	root, err := d.List(ctx, "")
	if err != nil || len(root) != 2 || root[0] != (Entry{Name: "backup", Dir: true}) || root[1].Name != "media" {
		t.Fatalf("root %+v, %v", root, err)
	}
	if len(mounts) != 0 {
		t.Fatalf("listing the shares mounted %q", mounts)
	}
	films, err := d.List(ctx, "media/films")
	if err != nil || len(films) != 1 || films[0].Name != "Heat.mkv" || films[0].Size != 4 || films[0].Version == "" {
		t.Fatalf("media/films %+v, %v", films, err)
	}
	if e, err := d.Stat(ctx, "media"); err != nil || !e.Dir || e.Name != "media" {
		t.Fatalf("Stat(media) %+v, %v", e, err)
	}
	r, err := d.Open(ctx, "media/films/Heat.mkv", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(r)
	r.Close()
	if string(b) != "ea" {
		t.Fatalf("read %q", b)
	}
	if !slices.Equal(mounts, []string{"media"}) {
		t.Fatalf("mounted %q, want media once", mounts)
	}
	if _, err := d.Open(ctx, "media", 0, -1); !errors.Is(err, remotefs.ErrIsDir) {
		t.Fatalf("opening a share = %v", err)
	}
	if _, err := d.List(ctx, "../etc"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("climbing out = %v", err)
	}
	if _, err := d.List(ctx, "photos"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("a share that is not there = %v", err)
	}
	if err := d.Close(); err != nil || closed != 1 {
		t.Fatalf("Close = %v, closed %d", err, closed)
	}
	if _, err := d.List(ctx, "media"); err == nil {
		t.Fatal("a closed source still reads")
	}
}

// A source with a share reads that share alone, and a folder without a share is no source; a server that refuses the sign-in fails as it opens.
func TestSMBOpens(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	closed := 0
	open := func(_ context.Context, cfg remotefs.Remote) (remotefs.Provider, io.Closer, error) {
		share := fakeShare{Provider: storage.NewLocal(dir), closed: &closed}
		return share, share, nil
	}
	refused := errors.New("logon failure")
	shares := func(context.Context, remotefs.Remote) ([]string, error) { return nil, refused }
	d, err := openSMB(ctx, remotefs.Remote{Host: "nas", Share: "media", User: "kando"}, open, shares)
	if err != nil {
		t.Fatal(err)
	}
	if e, err := d.Stat(ctx, ""); err != nil || !e.Dir {
		t.Fatalf("root %+v, %v", e, err)
	}
	d.Close()
	if closed != 1 {
		t.Fatalf("closed %d", closed)
	}
	if _, err := openSMB(ctx, remotefs.Remote{Host: "nas", Root: "films", User: "kando"}, open, shares); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("a folder without a share = %v", err)
	}
	if _, err := openSMB(ctx, remotefs.Remote{Host: "nas", User: "kando"}, open, shares); !errors.Is(err, refused) {
		t.Fatalf("a refused sign-in = %v", err)
	}
}

// A file's version is its ETag when the server gives one, else its size and time, so a file that changed reads as another version.
func TestEntryVersion(t *testing.T) {
	at := time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC)
	a := entryOf(remotefs.Info{Name: "a.mkv", Size: 10, Modified: at})
	b := entryOf(remotefs.Info{Name: "a.mkv", Size: 11, Modified: at})
	c := entryOf(remotefs.Info{Name: "a.mkv", Size: 10, Modified: at.Add(time.Second)})
	if a.Version == "" || a.Version == b.Version || a.Version == c.Version {
		t.Fatalf("versions %q %q %q", a.Version, b.Version, c.Version)
	}
	if e := entryOf(remotefs.Info{Name: "a.mkv", ETag: `"x"`}); e.Version != `"x"` {
		t.Fatalf("ETag version %q", e.Version)
	}
	if e := entryOf(remotefs.Info{Name: "dir", Dir: true}); e.Version != "" {
		t.Fatalf("folder version %q", e.Version)
	}
}
