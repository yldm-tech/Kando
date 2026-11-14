package film

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"
)

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
	first, err := RemuxPath("/cache", video, RemuxCopy)
	if err != nil || filepath.Dir(first) != "/cache" || filepath.Ext(first) != ".mp4" {
		t.Fatalf("copy at %q, %v", first, err)
	}
	if again, _ := RemuxPath("/cache", video, RemuxCopy); again != first {
		t.Fatalf("the same video copies to %q and %q", first, again)
	}
	if encoded, _ := RemuxPath("/cache", video, RemuxEncode); encoded == first {
		t.Fatal("the encoded copy takes the copied one's place")
	}
	if err := os.WriteFile(video, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if changed, _ := RemuxPath("/cache", video, RemuxCopy); changed == first {
		t.Fatal("a changed video kept its old copy's name")
	}
	if _, err := RemuxPath("/cache", filepath.Join(dir, "gone.mkv"), RemuxCopy); err == nil {
		t.Fatal("a copy of a video that is not there")
	}
}

// TestRemuxKeepsOneFilmsCopies runs Remux with a stand-in for ffmpeg that writes its output file, and checks that the copy lands at its path, that the copies of another video are cleared, that a copy already made is kept as it is, and that without ffmpeg there is no copy.
func TestRemuxKeepsOneFilmsCopies(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in for ffmpeg is a shell script")
	}
	dir := t.TempDir()
	ffmpeg := filepath.Join(dir, "ffmpeg")
	// The stand-in writes "copy" to the file named by its last argument, as ffmpeg writes its output there.
	if err := os.WriteFile(ffmpeg, []byte("#!/bin/sh\nfor last; do :; done\nprintf copy > \"$last\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
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
