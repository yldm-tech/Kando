package media

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// GrabPoster writes one JPEG frame of video to dest. A partial file is removed when the grab fails.
func GrabPoster(ctx context.Context, ffmpeg, video, dest string) error {
	if ffmpeg == "" || video == "" || dest == "" {
		return errors.New("poster source is missing")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := strings.TrimSuffix(dest, filepath.Ext(dest)) + ".grab.jpg"
	defer os.Remove(tmp)
	if _, err := run(ctx, ffmpeg, "-nostdin", "-loglevel", "error", "-y", "-ss", "0", "-i", video, "-frames:v", "1", "-vf", "scale=640:-2", tmp); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}
