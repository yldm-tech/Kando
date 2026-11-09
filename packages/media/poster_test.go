package media

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGrabPoster(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		skipWithoutFFmpeg(t, "ffmpeg is not installed")
	}
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	if _, err := run(context.Background(), ffmpeg, "-nostdin", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=0x3a2c1e:s=160x90:d=1", "-frames:v", "1", video); err != nil {
		skipWithoutFFmpeg(t, "ffmpeg could not make a clip: %v", err)
	}
	dest := filepath.Join(dir, "poster.jpg")
	if err := GrabPoster(context.Background(), ffmpeg, video, dest); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dest)
	if err != nil || info.Size() == 0 {
		t.Fatalf("poster %v size %d", err, info.Size())
	}
	if _, err := os.Stat(strings.TrimSuffix(dest, filepath.Ext(dest)) + ".grab.jpg"); !os.IsNotExist(err) {
		t.Fatal("partial poster was left behind")
	}
}

// A video's cover art comes back as the file holds it; a video without one has none.
func TestEmbeddedCover(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		skipWithoutFFmpeg(t, "ffmpeg is not installed")
	}
	dir := t.TempDir()
	cover := filepath.Join(dir, "cover.png")
	with := filepath.Join(dir, "with.mp4")
	without := filepath.Join(dir, "without.mp4")
	ctx := context.Background()
	if _, err := run(ctx, ffmpeg, "-nostdin", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=red:s=64x64:d=1", "-frames:v", "1", cover); err != nil {
		skipWithoutFFmpeg(t, "ffmpeg could not make a picture: %v", err)
	}
	if _, err := run(ctx, ffmpeg, "-nostdin", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=blue:s=160x90:d=1", "-i", cover,
		"-map", "0", "-map", "1", "-c:v:0", "mpeg4", "-c:v:1", "png", "-disposition:v:1", "attached_pic", with); err != nil {
		skipWithoutFFmpeg(t, "ffmpeg could not make a clip with cover art: %v", err)
	}
	if _, err := run(ctx, ffmpeg, "-nostdin", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=blue:s=160x90:d=1", "-c:v", "mpeg4", without); err != nil {
		skipWithoutFFmpeg(t, "ffmpeg could not make a clip: %v", err)
	}
	got, err := EmbeddedCover(ctx, ffmpeg, with)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(cover)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("cover is %d bytes, the file's own %d", len(got), len(want))
	}
	if data, err := EmbeddedCover(ctx, ffmpeg, without); err == nil {
		t.Fatalf("a video without cover art gave %d bytes", len(data))
	}
}

func TestGrabPosterSkipsABlackIntro(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		skipWithoutFFmpeg(t, "ffmpeg is not installed")
	}
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	if _, err := run(context.Background(), ffmpeg, "-nostdin", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=black:s=160x90:d=1:r=10",
		"-f", "lavfi", "-i", "color=c=white:s=160x90:d=1:r=10",
		"-filter_complex", "[0:v][1:v]concat=n=2:v=1:a=0",
		"-c:v", "mpeg4", "-pix_fmt", "yuv420p", video,
	); err != nil {
		skipWithoutFFmpeg(t, "ffmpeg could not make a clip: %v", err)
	}
	dest := filepath.Join(dir, "poster.jpg")
	if err := GrabPoster(context.Background(), ffmpeg, video, dest); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	luma, ok := jpegLuma(data)
	if !ok || luma < minPosterLuma {
		t.Fatalf("poster luma %v ok %v", luma, ok)
	}
}
