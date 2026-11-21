package mediasource

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"kando/packages/core"
	"kando/packages/media"
)

// memDriver is a source in memory: files by path, which it lists, stats and opens, counting its closes; it has no method that writes.
type memDriver struct {
	mu     sync.Mutex
	files  map[string]string
	closed int
	failOn string
}

func (d *memDriver) List(_ context.Context, dir string) ([]Entry, error) {
	seen := map[string]bool{}
	var out []Entry
	for p, data := range d.files {
		if path.Dir(p) != dir && !(dir == "" && !strings.Contains(p, "/")) {
			continue
		}
		name := path.Base(p)
		if !seen[name] {
			seen[name] = true
			out = append(out, Entry{Name: name, Size: int64(len(data))})
		}
	}
	return out, nil
}

func (d *memDriver) Stat(_ context.Context, p string) (Entry, error) {
	data, ok := d.files[p]
	if !ok {
		return Entry{}, ErrNotExist
	}
	return Entry{Name: path.Base(p), Size: int64(len(data))}, nil
}

func (d *memDriver) Open(_ context.Context, p string, offset, length int64) (io.ReadCloser, error) {
	if p == d.failOn {
		return nil, errors.New("connection reset")
	}
	data, ok := d.files[p]
	if !ok {
		return nil, ErrNotExist
	}
	return io.NopCloser(strings.NewReader(data[offset:])), nil
}

func (d *memDriver) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed++
	return nil
}

// testKind is a built-in kind for the tests, whose sources open as the test says.
func testKind(open func() (Driver, error)) Kind {
	return Kind{ID: "test", Title: func(string) string { return "Test" }, Icon: "server",
		Open: func(context.Context, Source, map[string]string) (Driver, error) { return open() }}
}

// A Pool connects to a source once and shares the connection, makes a new one once its source changed or after Forget, closes it when its source goes, and knows no source it was not given.
func TestPool(t *testing.T) {
	ctx := context.Background()
	var opened []*memDriver
	k := testKind(nil)
	kinds := func(id string) (Kind, bool) { return k, id == "test" }
	open := func(context.Context, Kind, Source) (Driver, error) {
		d := &memDriver{}
		opened = append(opened, d)
		return d, nil
	}
	s := Source{ID: "0123456789abcdef", Kind: "test", Name: "NAS", Config: map[string]string{"host": "nas"}}
	var p Pool
	if _, err := p.Driver(ctx, s.ID); !errors.Is(err, ErrSourceGone) {
		t.Fatalf("an empty pool: %v", err)
	}
	p.Set([]Source{s, {ID: "fedcba9876543210", Kind: "plugin:x:y", Name: "unknown"}}, kinds, open)
	a, _ := p.Driver(ctx, s.ID)
	b, _ := p.Driver(ctx, s.ID)
	if a != b || len(opened) != 1 {
		t.Fatalf("two connections: %d", len(opened))
	}
	if _, err := p.Driver(ctx, "fedcba9876543210"); !errors.Is(err, ErrSourceGone) {
		t.Fatalf("a source of a kind not registered: %v", err)
	}
	p.Set([]Source{s}, kinds, open)
	if c, _ := p.Driver(ctx, s.ID); c != a {
		t.Fatal("setting the same sources again dropped the connection")
	}
	changed := s
	changed.Config = map[string]string{"host": "nas", "share": "media"}
	p.Set([]Source{changed}, kinds, open)
	if opened[0].closed != 1 {
		t.Fatal("the connection to the source as it was is still open")
	}
	if c, _ := p.Driver(ctx, s.ID); c == a || len(opened) != 2 {
		t.Fatal("the changed source kept its old connection")
	}
	p.Forget(s.ID)
	if opened[1].closed != 1 {
		t.Fatal("Forget did not close the connection")
	}
	p.Driver(ctx, s.ID)
	p.Set(nil, kinds, open)
	if opened[2].closed != 1 {
		t.Fatal("removing the source did not close its connection")
	}
	if got, ok := p.Source(s.ID); ok {
		t.Fatalf("a removed source is still held: %+v", got)
	}
}

// Copier copies a source's video into the library where its films go, with the subtitles beside it renamed after the copy, reporting how far it got; a second copy of the same video is numbered rather than written over the first.
func TestCopierCopiesAVideoWithItsSubtitles(t *testing.T) {
	d := &memDriver{files: map[string]string{
		"Movies/Heat (1995).mkv":    strings.Repeat("h", 4096),
		"Movies/Heat (1995).srt":    "1\n00:00:01,000 --> 00:00:02,000\nhi\n",
		"Movies/Heat (1995).en.ass": "[Script Info]",
		"Movies/Heat (1995) 2.srt":  "another film's",
		"Movies/Ronin.mkv":          "r",
	}}
	var p Pool
	s := Source{ID: "0123456789abcdef", Kind: "test", Name: "Living Room NAS"}
	p.Set([]Source{s}, func(string) (Kind, bool) { return testKind(nil), true }, func(context.Context, Kind, Source) (Driver, error) { return d, nil })
	dir := t.TempDir()
	var progress []core.Progress
	ctx := media.WithTrace(context.Background(), &media.Trace{Progress: func(pr core.Progress) { progress = append(progress, pr) }})
	ref := Ref{Source: s.ID, Path: "Movies/Heat (1995).mkv"}.String()
	got, err := Copier{Pool: &p}.Copy(ctx, ref, dir)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "Living Room NAS", "Movies", "Heat (1995).mkv")
	if got != want {
		t.Fatalf("copied to %q, want %q", got, want)
	}
	if b, _ := os.ReadFile(got); len(b) != 4096 {
		t.Fatalf("copied %d bytes", len(b))
	}
	for _, name := range []string{"Heat (1995).srt", "Heat (1995).en.ass"} {
		if _, err := os.Stat(filepath.Join(dir, "Living Room NAS", "Movies", name)); err != nil {
			t.Errorf("subtitles %s not copied: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "Living Room NAS", "Movies", "Heat (1995) 2.srt")); err == nil {
		t.Error("another film's subtitles were copied")
	}
	if len(progress) == 0 || progress[len(progress)-1].Fraction != 1 || !progress[len(progress)-1].Known {
		t.Fatalf("progress %+v", progress)
	}
	again, err := Copier{Pool: &p}.Copy(ctx, ref, dir)
	if err != nil || filepath.Base(again) != "Heat (1995) (2).mkv" {
		t.Fatalf("again %q, %v", again, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Living Room NAS", "Movies", "Heat (1995) (2).srt")); err != nil {
		t.Errorf("the second copy's subtitles: %v", err)
	}
}

// A copy that fails leaves nothing in the library and drops the connection, so the next copy makes a new one; a source that went is said so.
func TestCopierFails(t *testing.T) {
	d := &memDriver{files: map[string]string{"a.mkv": "aaaa"}, failOn: "a.mkv"}
	var p Pool
	s := Source{ID: "0123456789abcdef", Kind: "test", Name: "NAS"}
	p.Set([]Source{s}, func(string) (Kind, bool) { return testKind(nil), true }, func(context.Context, Kind, Source) (Driver, error) { return d, nil })
	dir := t.TempDir()
	if _, err := (Copier{Pool: &p}).Copy(context.Background(), Ref{Source: s.ID, Path: "a.mkv"}.String(), dir); err == nil {
		t.Fatal("a broken read copied")
	}
	if d.closed != 1 {
		t.Fatal("the connection that failed was kept")
	}
	var left []string
	filepath.WalkDir(dir, func(p string, e os.DirEntry, _ error) error {
		if !e.IsDir() {
			left = append(left, p)
		}
		return nil
	})
	if len(left) != 0 {
		t.Fatalf("left %q", left)
	}
	if _, err := (Copier{Pool: &p}).Copy(context.Background(), Ref{Source: "fedcba9876543210", Path: "a.mkv"}.String(), dir); !errors.Is(err, ErrSourceGone) {
		t.Fatalf("a source that went: %v", err)
	}
	if _, err := (Copier{Pool: &p}).Copy(context.Background(), "https://example.com/a.mkv", dir); err == nil {
		t.Fatal("copied a link that is no source's")
	}
}

// Offline mode reads a source on the local network and refuses one on the internet, by where its host resolves to.
func TestCopierOffline(t *testing.T) {
	d := &memDriver{files: map[string]string{"a.mkv": "aaaa"}}
	var p Pool
	near := Source{ID: "0123456789abcdef", Kind: "test", Name: "NAS", Config: map[string]string{SMBHost: "nas.local"}}
	far := Source{ID: "fedcba9876543210", Kind: "test", Name: "Cloud", Config: map[string]string{SMBHost: "files.example.com"}}
	p.Set([]Source{near, far}, func(string) (Kind, bool) { return testKind(nil), true }, func(context.Context, Kind, Source) (Driver, error) { return d, nil })
	local := func(_ context.Context, host string) (bool, error) { return host == "nas.local", nil }
	c := Copier{Pool: &p, Offline: func() bool { return true }, LocalHost: local}
	if _, err := c.Copy(context.Background(), Ref{Source: near.ID, Path: "a.mkv"}.String(), t.TempDir()); err != nil {
		t.Fatalf("a NAS on the local network offline: %v", err)
	}
	if _, err := c.Copy(context.Background(), Ref{Source: far.ID, Path: "a.mkv"}.String(), t.TempDir()); !errors.Is(err, media.ErrOffline) {
		t.Fatalf("a source on the internet offline: %v", err)
	}
}

// A video's subtitles are the subtitle files named as it is, with a language or not; the host of a source is its server's.
func TestSubtitlesOfAndHost(t *testing.T) {
	names := []string{"Heat.mkv", "Heat.srt", "Heat.en.vtt", "Heat.SRT", "Heat.part1.en.srt", "Heater.srt", "Heat.txt"}
	if got := SubtitlesOf("Heat.mkv", names); !slices.Equal(got, []string{"Heat.srt", "Heat.en.vtt", "Heat.SRT"}) {
		t.Fatalf("SubtitlesOf = %q", got)
	}
	if h := Host(Source{Config: map[string]string{SMBHost: " nas.local "}}); h != "nas.local" {
		t.Fatalf("SMB host %q", h)
	}
	if h := Host(Source{Config: map[string]string{EndpointKey: "https://dav.example.com:8443/remote"}}); h != "dav.example.com" {
		t.Fatalf("endpoint host %q", h)
	}
}
