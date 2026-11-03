package media

import (
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
		t.Skip("ffmpeg is not installed")
	}
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	if _, err := run(context.Background(), ffmpeg, "-nostdin", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=0x3a2c1e:s=160x90:d=1", "-frames:v", "1", video); err != nil {
		t.Skipf("ffmpeg could not make a clip: %v", err)
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
