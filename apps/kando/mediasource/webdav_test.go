package mediasource

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"kando/packages/remotefs"
	"kando/packages/remotefs/davtest"
)

// The WebDAV kind is one the Registry takes beside SMB and SFTP, a phone can reach it, and Builtin finds it by its ID alone; its password is its one secret.
func TestWebDAVKindIsValid(t *testing.T) {
	var r Registry
	for _, k := range []Kind{SMB(), SFTP(), WebDAV()} {
		if err := r.Register(k); err != nil {
			t.Fatal(err)
		}
	}
	k, ok := Builtin(KindWebDAV)
	if !ok || k.ID != KindWebDAV || !k.Origin.Builtin() || !k.Phone || !slices.Equal(k.SecretKeys(), []string{WebDAVPassword}) {
		t.Fatalf("Builtin(webdav) = %+v, %v", k, ok)
	}
	for _, f := range k.Fields {
		if f.Title("en") == "" || f.Title("zh-Hans") == "" {
			t.Errorf("field %q has no title", f.Key)
		}
	}
}

// A WebDAV source's fields become remotefs's Remote, its server is its address's host, and an http:// address is one that sends the password unencrypted.
func TestWebDAVRemote(t *testing.T) {
	s := Source{ID: "0123456789abcdef", Kind: KindWebDAV, Config: map[string]string{WebDAVURL: " https://nas.local:5006/dav ", WebDAVRoot: "films", WebDAVUser: " kando "}}
	want := remotefs.Remote{Kind: remotefs.KindWebDAV, URL: "https://nas.local:5006/dav", Root: "films", User: "kando", Password: "pa55"}
	if got := WebDAVRemote(s, "pa55"); got != want {
		t.Fatalf("WebDAVRemote = %+v, want %+v", got, want)
	}
	if got, ok := Remote(s, map[string]string{WebDAVPassword: "pa55"}); !ok || got != want {
		t.Fatalf("Remote = %+v, %v", got, ok)
	}
	if h := Host(s); h != "nas.local" {
		t.Fatalf("Host = %q", h)
	}
	if PlainHTTP(s) {
		t.Fatal("https:// is plain HTTP")
	}
	s.Config[WebDAVURL] = "HTTP://192.168.1.20/dav"
	if !PlainHTTP(s) || Host(s) != "192.168.1.20" {
		t.Fatalf("http:// = %v, host %q", PlainHTTP(s), Host(s))
	}
}

// The WebDAV kind reads a folder of the server, signed in with the password, by ranges; a wrong password is a failure to sign in.
func TestWebDAVOpens(t *testing.T) {
	ctx := context.Background()
	f := davtest.New(t)
	if err := os.MkdirAll(filepath.Join(f.Dir, "films", "Heat"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.Dir, "films", "Heat", "video.mp4"), []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := Source{ID: "0123456789abcdef", Kind: KindWebDAV, Config: map[string]string{WebDAVURL: f.URL, WebDAVRoot: "films", WebDAVUser: davtest.User}}
	d, err := WebDAV().Open(ctx, s, map[string]string{WebDAVPassword: davtest.Password})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	entries, err := d.List(ctx, "Heat")
	if err != nil || len(entries) != 1 || entries[0].Name != "video.mp4" || entries[0].Size != 10 || entries[0].Version == "" {
		t.Fatalf("List = %+v, %v", entries, err)
	}
	f.ForgetRanges()
	rc, err := d.Open(ctx, "Heat/video.mp4", 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(rc)
	rc.Close()
	if err != nil || string(got) != "234" || !slices.Equal(f.Ranges(), []string{"bytes=2-4"}) {
		t.Fatalf("read %q, %v, ranges %q", got, err, f.Ranges())
	}
	if _, err := d.Stat(ctx, "../.."); err == nil {
		t.Fatal("a path out of the root was read")
	}
	if _, err := WebDAV().Open(ctx, s, map[string]string{WebDAVPassword: "wrong"}); remotefs.Diagnose(err) != remotefs.FailSignIn {
		t.Fatalf("a wrong password = %v", err)
	}
}
