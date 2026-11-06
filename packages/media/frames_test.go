package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// skipWithoutFFmpeg skips a test that needs ffmpeg when ffmpeg is missing or cannot make its clip. CI sets KANDO_REQUIRE_FFMPEG where it installs ffmpeg, and there the test fails instead, so that it cannot pass by skipping.
func skipWithoutFFmpeg(t *testing.T, format string, args ...any) {
	t.Helper()
	if os.Getenv("KANDO_REQUIRE_FFMPEG") != "" {
		t.Fatalf(format, args...)
	}
	t.Skipf(format, args...)
}

func TestOpenFramesReadsPictures(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		skipWithoutFFmpeg(t, "ffmpeg is not installed")
	}
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	if _, err := run(context.Background(), ffmpeg, "-nostdin", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=black:s=160x90:d=1:r=10", "-c:v", "mpeg4", "-pix_fmt", "yuv420p", video); err != nil {
		skipWithoutFFmpeg(t, "ffmpeg could not make a clip: %v", err)
	}
	reader, err := OpenFrames(ffmpeg, video)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	img, dur, err := reader.Next()
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 160 || img.Bounds().Dy() != 90 || dur <= 0 {
		t.Fatalf("frame %v for %s", img.Bounds(), dur)
	}
	if reader.Duration() < 900*time.Millisecond || reader.Duration() > 1500*time.Millisecond {
		t.Fatalf("duration %s", reader.Duration())
	}
	if _, _, err := reader.Next(); err != nil {
		t.Fatal(err)
	}
}
