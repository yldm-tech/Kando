package film

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// The stand-in for ffmpeg is this test binary under the name ffmpeg (standInFFmpeg), so it runs on Windows too, which starts no shell script. Started under that name, it writes "copy" to the file its last argument names, as ffmpeg writes its output there, and then, while a file named slow is beside it, waits five seconds, as a long copy does, until it is stopped.
func init() {
	if strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe") != "ffmpeg" {
		return
	}
	if err := os.WriteFile(os.Args[len(os.Args)-1], []byte("copy"), 0o644); err != nil {
		os.Exit(1)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(os.Args[0]), "slow")); err == nil {
		time.Sleep(5 * time.Second)
	}
	os.Exit(0)
}

// standInFFmpeg puts the stand-in for ffmpeg in dir and returns its path: a link to this test binary named ffmpeg, ffmpeg.exe on Windows, which runs a file as a program only by that name and makes symbolic links only in developer mode, so there it is a hard link, or a copy where dir is on another volume.
func standInFFmpeg(t *testing.T, dir string) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "ffmpeg")
	if runtime.GOOS != "windows" {
		if err := os.Symlink(exe, path); err != nil {
			t.Fatal(err)
		}
		return path
	}
	path += ".exe"
	if os.Link(exe, path) == nil {
		return path
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestRemuxArgsCopyThenEncode checks ffmpeg's arguments for the copies the system's player plays: the first picture and sound copied as they are into an MP4, or encoded as H.264 and AAC, without the streams an MP4 cannot hold, its index at the front.
func TestRemuxArgsCopyThenEncode(t *testing.T) {
	arg := func(args []string, name string) string {
		if i := slices.Index(args, name); i >= 0 && i+1 < len(args) {
			return args[i+1]
		}
		return ""
	}
	copied := RemuxArgs("/films/a.mkv", "/cache/a.mp4", RemuxCopy)
	if arg(copied, "-i") != "/films/a.mkv" || arg(copied, "-c") != "copy" || arg(copied, "-movflags") != "+faststart" || copied[len(copied)-1] != "/cache/a.mp4" || !slices.Contains(copied, "-sn") {
		t.Fatalf("copy: %q", copied)
	}
	encoded := RemuxArgs("/films/a.mkv", "/cache/a.mp4", RemuxEncode)
	if arg(encoded, "-c:v") != "h264" || arg(encoded, "-c:a") != "aac" || slices.Contains(encoded, "copy") {
		t.Fatalf("encode: %q", encoded)
	}
}

// TestRemuxPathFollowsTheVideo checks that a video's copy is named after it and its step, and that the video changing makes a copy of its own.
func TestRemuxPathFollowsTheVideo(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "a.mkv")
	if err := os.WriteFile(video, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	cache := filepath.FromSlash("/cache")
	first, err := RemuxPath(cache, video, RemuxCopy)
	if err != nil || filepath.Dir(first) != cache || filepath.Ext(first) != ".mp4" {
		t.Fatalf("copy at %q, %v", first, err)
	}
	if again, _ := RemuxPath(cache, video, RemuxCopy); again != first {
		t.Fatalf("the same video copies to %q and %q", first, again)
	}
	if encoded, _ := RemuxPath(cache, video, RemuxEncode); encoded == first {
		t.Fatal("the encoded copy takes the copied one's place")
	}
	if err := os.WriteFile(video, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if changed, _ := RemuxPath(cache, video, RemuxCopy); changed == first {
		t.Fatal("a changed video kept its old copy's name")
	}
	if _, err := RemuxPath("/cache", filepath.Join(dir, "gone.mkv"), RemuxCopy); err == nil {
		t.Fatal("a copy of a video that is not there")
	}
}

// TestRemuxKeepsOneFilmsCopies runs Remux with a stand-in for ffmpeg that writes its output file, and checks that the copy lands at its path, that the copies of another video are cleared, that a copy already made is kept as it is, and that without ffmpeg there is no copy.
func TestRemuxKeepsOneFilmsCopies(t *testing.T) {
	dir := t.TempDir()
	ffmpeg := standInFFmpeg(t, dir)
	cache := filepath.Join(dir, "cache")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(cache, "0123456789abcdef01234567-1.mp4")
	if err := os.WriteFile(other, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	video := filepath.Join(dir, "a.mkv")
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest, err := RemuxPath(cache, video, RemuxCopy)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := Remux(ctx, ffmpeg, video, dest, RemuxCopy); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(dest); err != nil || string(data) != "copy" {
		t.Fatalf("the copy holds %q, %v", data, err)
	}
	if _, err := os.Stat(other); err == nil {
		t.Fatal("another video's copy stayed")
	}
	if err := os.WriteFile(dest, []byte("made"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Remux(ctx, ffmpeg, video, dest, RemuxCopy); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(dest); string(data) != "made" {
		t.Fatalf("a copy already made was made again: %q", data)
	}
	if err := Remux(ctx, "", video, dest+".x", RemuxCopy); err == nil {
		t.Fatal("a copy without ffmpeg")
	}
}

// TestRemuxRunsWriteFilesOfTheirOwn starts a copy, stops it while ffmpeg writes, and checks that a second run of the same copy, started meanwhile, still lands: the stopped run removes only the file it wrote.
func TestRemuxRunsWriteFilesOfTheirOwn(t *testing.T) {
	dir := t.TempDir()
	// The stand-in writes its output, then waits while the file slow is beside it, as a long copy does.
	ffmpeg := standInFFmpeg(t, dir)
	video := filepath.Join(dir, "a.mkv")
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(dir, "cache")
	dest, err := RemuxPath(cache, video, RemuxCopy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "slow"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan error, 1)
	go func() { stopped <- Remux(ctx, ffmpeg, video, dest, RemuxCopy) }()
	// waitParts waits until n runs have written their files and wait.
	waitParts := func(n int) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			parts, _ := filepath.Glob(filepath.Join(cache, "*.part"))
			if len(parts) == n {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%d runs wrote their files, want %d", len(parts), n)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	waitParts(1)
	second, cancelSecond := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelSecond()
	done := make(chan error, 1)
	go func() { done <- Remux(second, ffmpeg, video, dest, RemuxCopy) }()
	waitParts(2)
	// The first run is stopped while both wait, and only then does the copy end, so the first cannot finish before it is stopped, as it could when slow went first.
	cancel()
	if err := <-stopped; err == nil {
		t.Fatal("the stopped run made its copy")
	}
	if err := os.Remove(filepath.Join(dir, "slow")); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("the second run: %v", err)
	}
	if data, err := os.ReadFile(dest); err != nil || string(data) != "copy" {
		t.Fatalf("the copy holds %q, %v", data, err)
	}
	if parts, _ := filepath.Glob(filepath.Join(cache, "*.part")); len(parts) != 0 {
		t.Fatalf("files left behind: %q", parts)
	}
}

// TestMissingPlugins checks which of the system player's errors say GStreamer lacks what the video needs, as GTK and NSGo word them on Linux, and that the others, as a file that is not there, leave the copies to try.
func TestMissingPlugins(t *testing.T) {
	for _, msg := range []string{
		"Your GStreamer installation is missing a plug-in. No installed GStreamer plug-in can play this format (H.264 / AAC need gst-libav or gst-plugins-ugly / -bad).",
		"GTK could not find a media module. Check your installation.",
		"No suitable plugins found",
	} {
		if !MissingPlugins(errors.New(msg)) {
			t.Errorf("MissingPlugins(%q) = false", msg)
		}
	}
	for _, err := range []error{nil, errors.New("Resource not found."), errors.New("The operation couldn’t be completed. (AVFoundationErrorDomain error -11828.)")} {
		if MissingPlugins(err) {
			t.Errorf("MissingPlugins(%v) = true", err)
		}
	}
}
