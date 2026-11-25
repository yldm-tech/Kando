package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"testing"

	"kando/packages/remotefs"
	"kando/packages/remotefs/davtest"
	"kando/packages/remotefs/providertest"
	"kando/packages/remotefs/sftptest"
)

// providers are a Local over a new folder and an ObjectStore over a fake bucket, rooted below its top so the root's prefix is part of every key: this package's own providers.
func providers(t *testing.T) map[string]remotefs.Provider {
	store, _ := fakeStore(t, "kando/films")
	return map[string]remotefs.Provider{"local": NewLocal(t.TempDir()), "objects": store}
}

// copyProviders are providers and, as a storage location can be on a NAS, a WebDAV and an SFTP provider over servers in the test, which Put and Get copy to and from.
func copyProviders(t *testing.T) map[string]remotefs.Provider {
	out := providers(t)
	dav := davtest.New(t)
	if err := os.Mkdir(dav.Dir+"/films", 0o755); err != nil {
		t.Fatal(err)
	}
	ssh := sftptest.New(t)
	host, port := ssh.HostPort()
	for kind, cfg := range map[string]remotefs.Remote{
		"webdav": {Kind: remotefs.KindWebDAV, URL: dav.URL, Root: "/films/", User: "kando", Password: "secret"},
		"sftp":   {Kind: remotefs.KindSFTP, Host: host, Port: port, User: sftptest.User, Password: sftptest.Password, HostKey: ssh.HostKey},
	} {
		p, closer, err := remotefs.OpenRemote(context.Background(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { closer.Close() })
		out[kind] = p
	}
	return out
}

// TestProviders checks the rules every provider keeps against a Local and an ObjectStore.
func TestProviders(t *testing.T) {
	providertest.Run(t, providers)
}

func TestLocalWritesAtomicallyAndHidesItsTempFile(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	l := NewLocal(dir)
	providertest.Write(t, l, "a/video.mp4", "old")
	// While a write is going, the file is still the old one and the temp file is not listed.
	r, w := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- l.Write(ctx, "a/video.mp4", r, 6) }()
	if _, err := w.Write([]byte("new")); err != nil {
		t.Fatal(err)
	}
	if got := providertest.Read(t, l, "a/video.mp4", 0, -1); got != "old" {
		t.Fatalf("during the write = %q", got)
	}
	entries, err := os.ReadDir(dir + "/a")
	if err != nil || len(entries) != 2 {
		t.Fatalf("the folder holds %v, %v", entries, err)
	}
	if got := providertest.Names(t, l, "a"); !slices.Equal(got, []string{"video.mp4"}) {
		t.Fatalf("lists %q", got)
	}
	w.Write([]byte("abc"))
	w.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := providertest.Read(t, l, "a/video.mp4", 0, -1); got != "newabc" {
		t.Fatalf("after the write = %q", got)
	}
	if entries, _ := os.ReadDir(dir + "/a"); len(entries) != 1 {
		t.Fatalf("the temp file stayed: %v", entries)
	}
}

func TestLocalRefusesASymlinkOutOfTheRoot(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(outside+"/secret", []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Symlink(outside, dir+"/out"); err != nil {
		t.Skip("no symlinks here:", err)
	}
	l := NewLocal(dir)
	if r, err := l.Open(context.Background(), "out/secret", 0, -1); err == nil {
		r.Close()
		t.Fatal("read through a symlink out of the root")
	}
	if err := l.Write(context.Background(), "out/planted", strings.NewReader("x"), 1); err == nil {
		t.Fatal("wrote through a symlink out of the root")
	}
	if _, err := os.Stat(outside + "/planted"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("planted: %v", err)
	}
}

func TestLocalMkdirAllAndOpenAFolder(t *testing.T) {
	ctx := context.Background()
	l := NewLocal(t.TempDir())
	if err := l.MkdirAll(ctx, "a/b/c"); err != nil {
		t.Fatal(err)
	}
	if info, err := l.Stat(ctx, "a/b/c"); err != nil || !info.Dir {
		t.Fatalf("stat = %+v, %v", info, err)
	}
	if _, err := l.Open(ctx, "a/b", 0, -1); !errors.Is(err, remotefs.ErrIsDir) {
		t.Fatalf("open a folder = %v", err)
	}
}

func TestObjectStoreKeysLiveUnderItsRoot(t *testing.T) {
	ctx := context.Background()
	store, fake := fakeStore(t, "kando/films/")
	providertest.Write(t, store, "Heat/video.mp4", "video")
	if _, ok := fake.object("kando/films/Heat/video.mp4"); !ok {
		t.Fatalf("keys %v", fake.objects)
	}
	// An object outside the root is not seen.
	fake.mu.Lock()
	fake.store("kando/other.mp4", []byte("x"))
	fake.mu.Unlock()
	if got := providertest.Names(t, store, ""); !slices.Equal(got, []string{"Heat/"}) {
		t.Fatalf("root lists %q", got)
	}
	if _, err := NewObjectStore(nil, "../x"); !errors.Is(err, remotefs.ErrInvalidPath) {
		t.Fatalf("root outside = %v", err)
	}
	// The root of an empty bucket lists nothing rather than failing.
	empty, _ := fakeStore(t, "")
	if infos, err := empty.List(ctx, ""); err != nil || len(infos) != 0 {
		t.Fatalf("empty root = %v, %v", infos, err)
	}
}

// Listing a folder of many objects goes page after page, and a folder holding only folders is there.
func TestObjectStoreListsEveryPage(t *testing.T) {
	store, _ := fakeStore(t, "")
	var want []string
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		providertest.Write(t, store, "films/"+name+".mp4", name)
		want = append(want, name+".mp4")
		providertest.Write(t, store, "films/"+name+"/video.mp4", name)
		want = append(want, name+"/")
	}
	slices.Sort(want)
	if got := providertest.Names(t, store, "films"); !slices.Equal(got, want) {
		t.Fatalf("lists %q, want %q", got, want)
	}
}

func TestObjectStoreLinksAndReadsByRange(t *testing.T) {
	ctx := context.Background()
	store, fake := fakeStore(t, "kando")
	providertest.Write(t, store, "Heat/video.mp4", "0123456789")
	if got := providertest.Read(t, store, "Heat/video.mp4", 2, 3); got != "234" {
		t.Fatalf("range = %q", got)
	}
	if got := fake.ranges; !slices.Equal(got, []string{"bytes=2-4"}) {
		t.Fatalf("ranges asked %q", got)
	}
	var linker Linker = store
	link, err := linker.Link(ctx, "Heat/video.mp4", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(link, "/films/kando/Heat/video.mp4?") || !strings.Contains(link, "X-Amz-Signature=") {
		t.Fatalf("link %s", link)
	}
}

// A body larger than one PUT is not read into memory whole: it is sent as an upload in parts by the cloud package, which its own tests cover with short parts; here a body over the threshold goes in parts and the object holds all of it.
func TestObjectStoreWritesALargeBodyInParts(t *testing.T) {
	if testing.Short() {
		t.Skip("writes 40 MiB")
	}
	store, fake := fakeStore(t, "")
	body := bytes.Repeat([]byte("kando-"), 40<<20/6)
	// A pipe cannot go back to its start, so only parts of it are kept in memory.
	r, w := io.Pipe()
	go func() {
		w.Write(body)
		w.Close()
	}()
	if err := store.Write(context.Background(), "big.mp4", r, int64(len(body))); err != nil {
		t.Fatal(err)
	}
	if got, _ := fake.object("big.mp4"); !bytes.Equal(got, body) {
		t.Fatalf("stored %d bytes of %d", len(got), len(body))
	}
	if fake.count("part") != 2 || fake.count("complete") != 1 || fake.count("put") != 0 {
		t.Fatalf("calls %v", fake.calls)
	}
}
