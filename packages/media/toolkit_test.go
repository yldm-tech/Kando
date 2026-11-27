package media

import (
	"archive/tar"
	"archive/zip"
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

func zipOf(t *testing.T, entries ...tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		h.SetMode(os.FileMode(e.mode))
		if e.kind == tar.TypeDir {
			h.SetMode(os.ModeDir | os.FileMode(e.mode))
		}
		if e.kind == tar.TypeSymlink {
			h.SetMode(os.ModeSymlink | 0o777)
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// Deno publishes its builds as a .zip of the one program, which may run once it is in.
func TestInstallKitUnpacksZip(t *testing.T) {
	data := zipOf(t, tarEntry{name: "deno", mode: 0o755, body: "#!/bin/sh\n"})
	dir := t.TempDir()
	path, err := InstallKit(context.Background(), Kit{ID: "deno", Program: "deno", Version: "2.9.8", URL: serve(t, data), SHA256: sha(data), Zip: true}, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "deno", "deno"); path != want || !isExecutable(path) {
		t.Fatalf("path %q is not the executable %q", path, want)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "deno", kitVersionFile)); strings.TrimSpace(string(got)) != "2.9.8" {
		t.Fatalf("version file %q", got)
	}
}

func TestInstallKitRejectsUnsafeZips(t *testing.T) {
	cases := map[string]tarEntry{
		"parent":   {name: "../evil", mode: 0o755, body: "x"},
		"absolute": {name: "/tmp/evil", mode: 0o755, body: "x"},
		"symlink":  {name: "deno", kind: tar.TypeSymlink, body: "/bin/sh"},
	}
	for name, entry := range cases {
		t.Run(name, func(t *testing.T) {
			data := zipOf(t, entry)
			_, err := InstallKit(context.Background(), Kit{ID: "deno", Program: "deno", URL: serve(t, data), SHA256: sha(data), Zip: true}, t.TempDir(), nil)
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

// Every platform Kando ships for gets the three programs and Deno, each pinned to a checksum.
func TestKitsArePinned(t *testing.T) {
	for _, platform := range []string{"darwin-arm64", "darwin-amd64", "linux-amd64", "linux-arm64"} {
		for _, program := range []string{"yt-dlp", "ffmpeg", "whisper-cli", "deno"} {
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
	noPackageManagerDirs(t)
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
	// Kando's own comes first, unless its kit is a fallback, which comes after every copy the computer has.
	first, last := filepath.Join(own, kit.ID, "ffmpeg"), filepath.Join(onPath, "ffmpeg")
	if kit.Fallback {
		first, last = last, first
	}
	if got := Candidates("ffmpeg"); len(got) != 2 || got[0] != first || got[1] != last {
		t.Fatalf("Candidates = %q, want [%s %s]", got, first, last)
	}
	if got := lookTool("ffmpeg"); got != first {
		t.Fatalf("lookTool = %q, want %s", got, first)
	}
	// Programs no kit provides are looked up as before.
	if got := lookTool("whisper-cli"); got != "" && strings.HasPrefix(got, own) {
		t.Fatalf("lookTool found whisper-cli in an empty kit folder: %q", got)
	}
}

// TestKitsFallBackOnlyOnLinuxForFFmpegAndWhisper checks which kits are fallbacks: on Linux ffmpeg and whisper-cli, which the distribution keeps working and may build with more than the kit (Kit.Fallback); everywhere else Kando runs its own copy first.
// TestKitStale checks that a kit Kando downloaded is stale once this Kando pins a later version of it, that one downloaded before Kando wrote its version is taken for the pinned one, and that yt-dlp, which updates itself, never is.
func TestKitStale(t *testing.T) {
	saved := OwnToolsDir
	OwnToolsDir = t.TempDir()
	t.Cleanup(func() { OwnToolsDir = saved })
	data := tarGz(t, tarEntry{name: "./ffmpeg", mode: 0o755, body: "#!/bin/sh\n"})
	kit := Kit{ID: "ffmpeg", Program: "ffmpeg", Version: "9.0.2", URL: serve(t, data), SHA256: sha(data), Archive: true}
	if _, err := InstallKit(context.Background(), kit, OwnToolsDir, nil); err != nil {
		t.Fatal(err)
	}
	if KitStale(kit) {
		t.Fatal("the kit just downloaded is stale")
	}
	newer := kit
	newer.Version = "9.1"
	if !KitStale(newer) {
		t.Fatal("a kit of an earlier version is not stale")
	}
	if newer.Updates = true; KitStale(newer) {
		t.Fatal("a kit that updates itself is stale")
	}
	if err := os.Remove(filepath.Join(OwnToolsDir, "ffmpeg", kitVersionFile)); err != nil {
		t.Fatal(err)
	}
	if newer.Updates = false; KitStale(newer) {
		t.Fatal("a kit downloaded before Kando wrote its version is stale")
	}
}

// Deno is a fallback everywhere: the computer's, as Homebrew's, runs when there is one.
func TestKitsFallBackOnlyOnLinuxForFFmpegAndWhisper(t *testing.T) {
	for platform, list := range kits {
		for _, kit := range list {
			want := kit.Program == "deno" || strings.HasPrefix(platform, "linux-") && kit.Program != "yt-dlp"
			if kit.Fallback != want {
				t.Errorf("%s %s: Fallback = %v, want %v", platform, kit.Program, kit.Fallback, want)
			}
			if kit.Updates != (kit.Program == "yt-dlp") {
				t.Errorf("%s %s: Updates = %v, want it only for yt-dlp, which updates itself with -U", platform, kit.Program, kit.Updates)
			}
		}
	}
}

// TestSourceOnWindows checks that every program Kando runs has a page to get it from and a Scoop package on Windows, where Kando downloads none of them, and a page elsewhere.
func TestSourceOnWindows(t *testing.T) {
	for _, program := range []string{"yt-dlp", "ffmpeg", "whisper-cli"} {
		if _, ok := kitOn("windows-amd64", program); ok {
			t.Errorf("a Windows kit of %s: its row would offer the download, not the source", program)
		}
		src, ok := sourceOn("windows", program)
		if !ok || !strings.HasPrefix(src.Page, "https://") || src.Scoop == "" {
			t.Errorf("sourceOn(windows, %s) = %+v, %v", program, src, ok)
		}
		if src, ok := sourceOn("linux", program); !ok || !strings.HasPrefix(src.Page, "https://") || src.Scoop != "" {
			t.Errorf("sourceOn(linux, %s) = %+v, %v", program, src, ok)
		}
	}
	if _, ok := sourceOn("windows", "everyapi"); ok {
		t.Error("a source of everyapi, which the AI settings get")
	}
}
