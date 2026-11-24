package storage

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"kando/apps/kando/storage/davtest"
)

// webdavProvider is a provider over the folder films of a new davtest.Server.
func webdavProvider(t *testing.T) (Provider, *davtest.Server) {
	t.Helper()
	f := davtest.New(t)
	if err := os.Mkdir(f.Dir+"/films", 0o755); err != nil {
		t.Fatal(err)
	}
	p, closer, err := OpenRemote(context.Background(), Remote{Kind: KindWebDAV, URL: f.URL, Root: "/films/", User: "kando", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closer.Close() })
	return p, f
}

func TestWebDAVKeepsFilesUnderItsRootAndReadsByRange(t *testing.T) {
	p, f := webdavProvider(t)
	write(t, p, "Heat (1995)/video.mp4", "0123456789")
	if body, err := os.ReadFile(f.Dir + "/films/Heat (1995)/video.mp4"); err != nil || string(body) != "0123456789" {
		t.Fatalf("on the server: %q, %v", body, err)
	}
	f.ForgetRanges()
	if got := read(t, p, "Heat (1995)/video.mp4", 2, 3); got != "234" {
		t.Fatalf("range = %q", got)
	}
	if got := read(t, p, "Heat (1995)/video.mp4", 7, -1); got != "789" {
		t.Fatalf("tail = %q", got)
	}
	if ranges := f.Ranges(); !slices.Equal(ranges, []string{"bytes=2-4", "bytes=7-9"}) {
		t.Fatalf("ranges asked %q", ranges)
	}
	info, err := p.Stat(context.Background(), "Heat (1995)/video.mp4")
	if err != nil || info.ETag == "" {
		t.Fatalf("stat = %+v, %v", info, err)
	}
}

func TestWebDAVRefusesAWrongPasswordAndABadAddress(t *testing.T) {
	ctx := context.Background()
	f := davtest.New(t)
	if _, _, err := OpenRemote(ctx, Remote{Kind: KindWebDAV, URL: f.URL, User: "kando", Password: "wrong"}); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("wrong password = %v", err)
	}
	if _, _, err := OpenRemote(ctx, Remote{Kind: KindWebDAV, URL: f.URL, Root: "missing", User: "kando", Password: "secret"}); err == nil {
		t.Fatal("a missing root opened")
	}
	for _, bad := range []string{"ftp://nas/films", "nas/films", "https://user:pw@nas/films", "https://nas/../x"} {
		if _, _, err := OpenRemote(ctx, Remote{Kind: KindWebDAV, URL: bad}); err == nil {
			t.Errorf("%s opened", bad)
		}
	}
	if _, _, err := OpenRemote(ctx, Remote{Kind: KindWebDAV, URL: f.URL, Root: "../x"}); err == nil {
		t.Error("a root above the address opened")
	}
}
