package media

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type tarEntry struct {
	name string
	mode int64
	body string
	kind byte
}

func tarGz(t *testing.T, entries ...tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for _, e := range entries {
		kind := e.kind
		if kind == 0 {
			kind = tar.TypeReg
		}
		h := &tar.Header{Name: e.name, Mode: e.mode, Size: int64(len(e.body)), Typeflag: kind}
		if kind == tar.TypeSymlink {
			h.Size, h.Linkname = 0, e.body
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if kind == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sha(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func serve(t *testing.T, data []byte) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(data) }))
	t.Cleanup(srv.Close)
	return srv.URL + "/kit"
}

func TestInstallKitUnpacksArchive(t *testing.T) {
	data := tarGz(t,
		tarEntry{name: "./", kind: tar.TypeDir, mode: 0o755},
		tarEntry{name: "./ffmpeg", mode: 0o755, body: "#!/bin/sh\n"},
		tarEntry{name: "./ffplay", mode: 0o755, body: "#!/bin/sh\n"},
		tarEntry{name: "./LICENSE-ffmpeg.txt", mode: 0o644, body: "LGPL"},
	)
	dir := t.TempDir()
	// What an earlier version left goes once the new one is in.
	if err := os.MkdirAll(filepath.Join(dir, "ffmpeg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ffmpeg", "old"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var last int64
	path, err := InstallKit(context.Background(), Kit{ID: "ffmpeg", Program: "ffmpeg", URL: serve(t, data), SHA256: sha(data), Archive: true}, dir, func(written, total int64) { last = written })
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "ffmpeg", "ffmpeg"); path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	if last != int64(len(data)) {
		t.Fatalf("progress ended at %d of %d", last, len(data))
	}
	for _, name := range []string{"ffmpeg", "ffplay"} {
		if !isExecutable(filepath.Join(dir, "ffmpeg", name)) {
			t.Fatalf("%s is not executable", name)
		}
	}
	if info, err := os.Stat(filepath.Join(dir, "ffmpeg", "LICENSE-ffmpeg.txt")); err != nil || info.Mode()&0o111 != 0 {
		t.Fatalf("license: %v, %v", info, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ffmpeg", "old")); !os.IsNotExist(err) {
		t.Fatalf("the old version's file stayed: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("dir holds %v, want only the kit's folder", entries)
	}
}

func TestInstallKitSingleProgram(t *testing.T) {
	data := []byte("#!/bin/sh\necho 2026.10.26\n")
	dir := t.TempDir()
	path, err := InstallKit(context.Background(), Kit{ID: "yt-dlp", Program: "yt-dlp", URL: serve(t, data), SHA256: sha(data)}, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, "yt-dlp", "yt-dlp") || !isExecutable(path) {
		t.Fatalf("path %q is not the executable yt-dlp", path)
	}
	// The standalone build in a folder Kando writes updates itself with -U.
	if install := FindYTDLPInstall(path); install.Kind != "standalone" || len(install.Update) != 2 || install.Update[1] != "-U" {
		t.Fatalf("install = %+v, want the standalone build updating with -U", install)
	}
}

func TestInstallKitRejectsWrongChecksum(t *testing.T) {
	data := []byte("tampered")
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "yt-dlp"), 0o755); err != nil {
		t.Fatal(err)
	}
	kept := filepath.Join(dir, "yt-dlp", "yt-dlp")
	if err := os.WriteFile(kept, []byte("good"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := InstallKit(context.Background(), Kit{ID: "yt-dlp", Program: "yt-dlp", URL: serve(t, data), SHA256: strings.Repeat("0", 64)}, dir, nil)
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("err = %v, want a checksum error", err)
	}
	if got, _ := os.ReadFile(kept); string(got) != "good" {
		t.Fatalf("the installed program was replaced by a failed download: %q", got)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("a failed download left %v", entries)
	}
}

func TestInstallKitRejectsUnsafeArchives(t *testing.T) {
	cases := map[string]tarEntry{
		"parent":   {name: "../evil", mode: 0o755, body: "x"},
		"absolute": {name: "/tmp/evil", mode: 0o755, body: "x"},
		"symlink":  {name: "ffmpeg", kind: tar.TypeSymlink, body: "/bin/sh"},
	}
	// Windows reads a backslash as a separator too and a drive letter as another volume; elsewhere these are ordinary file names.
	if runtime.GOOS == "windows" {
		cases["backslash parent"] = tarEntry{name: `..\evil`, mode: 0o755, body: "x"}
		cases["nested backslash parent"] = tarEntry{name: `bin\..\..\evil`, mode: 0o755, body: "x"}
		cases["drive"] = tarEntry{name: `C:\evil`, mode: 0o755, body: "x"}
		cases["drive relative"] = tarEntry{name: "C:evil", mode: 0o755, body: "x"}
	}
	for name, entry := range cases {
		t.Run(name, func(t *testing.T) {
			data := tarGz(t, entry)
			_, err := InstallKit(context.Background(), Kit{ID: "ffmpeg", Program: "ffmpeg", URL: serve(t, data), SHA256: sha(data), Archive: true}, t.TempDir(), nil)
			if !errors.Is(err, errUnsafeEntry) {
				t.Fatalf("err = %v, want errUnsafeEntry", err)
			}
		})
	}
}

func TestInstallKitWithoutProgram(t *testing.T) {
	data := tarGz(t, tarEntry{name: "LICENSE", mode: 0o644, body: "MIT"})
	_, err := InstallKit(context.Background(), Kit{ID: "whisper", Program: "whisper-cli", URL: serve(t, data), SHA256: sha(data), Archive: true}, t.TempDir(), nil)
	if err == nil || !strings.Contains(err.Error(), "whisper-cli is not in the download") {
		t.Fatalf("err = %v", err)
	}
}

func TestInstallKitCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dir := t.TempDir()
	if _, err := InstallKit(ctx, Kit{ID: "yt-dlp", Program: "yt-dlp", URL: serve(t, []byte("x")), SHA256: sha([]byte("x"))}, dir, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("a canceled download left %v", entries)
	}
}

func TestInstallKitStalls(t *testing.T) {
	shortStall(t, 150*time.Millisecond)
	data := make([]byte, 64<<10)
	srv := stallingServer(t, data, 1000)
	dir := t.TempDir()
	_, err := InstallKit(context.Background(), Kit{ID: "yt-dlp", Program: "yt-dlp", URL: srv.URL, SHA256: sha(data)}, dir, nil)
	if !errors.Is(err, ErrStalled) {
		t.Fatalf("err = %v, want ErrStalled", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("a stalled download left %v", entries)
	}
}

func TestInstallKitStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "busy", http.StatusServiceUnavailable) }))
	t.Cleanup(srv.Close)
	_, err := InstallKit(context.Background(), Kit{ID: "yt-dlp", Program: "yt-dlp", URL: srv.URL, SHA256: sha(nil)}, t.TempDir(), nil)
	var status interface{ StatusCode() int }
	if !errors.As(err, &status) || status.StatusCode() != http.StatusServiceUnavailable || err.Error() != "download yt-dlp: HTTP 503" {
		t.Fatalf("err = %v, want HTTP 503", err)
	}
}

// Every platform Kando ships for gets the three programs, each pinned to a checksum.
func TestKitsArePinned(t *testing.T) {
	for _, platform := range []string{"darwin-arm64", "darwin-amd64", "linux-amd64", "linux-arm64"} {
		for _, program := range []string{"yt-dlp", "ffmpeg", "whisper-cli"} {
			kit, ok := kitOn(platform, program)
			if !ok {
				t.Errorf("%s has no kit for %s", platform, program)
				continue
			}
			if len(kit.SHA256) != 64 || kit.Size <= 0 || !strings.HasPrefix(kit.URL, "https://github.com/") {
				t.Errorf("%s %s is not pinned: %+v", platform, program, kit)
			}
		}
	}
}

func TestLookToolPrefersOwnKit(t *testing.T) {
	kit, ok := KitFor("ffmpeg")
	if !ok {
		t.Skip("no kits on this platform")
	}
	own := t.TempDir()
	onPath := t.TempDir()
	for _, path := range []string{filepath.Join(own, kit.ID, "ffmpeg"), filepath.Join(onPath, "ffmpeg")} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", onPath)
	saved := OwnToolsDir
	t.Cleanup(func() { OwnToolsDir = saved })

	OwnToolsDir = ""
	if got := lookTool("ffmpeg"); got != filepath.Join(onPath, "ffmpeg") {
		t.Fatalf("without a folder of its own, lookTool = %q", got)
	}
	OwnToolsDir = own
	if got := lookTool("ffmpeg"); got != filepath.Join(own, kit.ID, "ffmpeg") {
		t.Fatalf("lookTool = %q, want Kando's own", got)
	}
	// Programs no kit provides are looked up as before.
	if got := lookTool("whisper-cli"); got != "" && strings.HasPrefix(got, own) {
		t.Fatalf("lookTool found whisper-cli in an empty kit folder: %q", got)
	}
}
