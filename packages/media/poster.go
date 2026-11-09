package media

import (
	"bytes"
	"context"
	"errors"
	"image/jpeg"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// minPosterLuma is how bright a grabbed frame must be before it counts as a cover. Below this, the frame is a black intro.
const minPosterLuma = 0.08

// GrabPoster writes one JPEG frame of video to dest, skipping a black opening when a later frame is brighter. A partial file is removed when the grab fails.
func GrabPoster(ctx context.Context, ffmpeg, video, dest string) error {
	if ffmpeg == "" || video == "" || dest == "" {
		return errors.New("poster source is missing")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	offsets := posterOffsets(videoDuration(ffmpeg, video))
	var best []byte
	bestLuma := -1.0
	var lastErr error
	for _, at := range offsets {
		data, err := grabPosterFrame(ctx, ffmpeg, video, at)
		if err != nil {
			lastErr = err
			continue
		}
		luma, ok := jpegLuma(data)
		if !ok {
			lastErr = errors.New("poster frame is not a jpeg")
			continue
		}
		if luma > bestLuma {
			best = data
			bestLuma = luma
		}
		if luma >= minPosterLuma {
			break
		}
	}
	if best == nil {
		if lastErr != nil {
			return lastErr
		}
		return errors.New("poster frame is missing")
	}
	tmp := strings.TrimSuffix(dest, filepath.Ext(dest)) + ".grab.jpg"
	defer os.Remove(tmp)
	if err := os.WriteFile(tmp, best, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}

// EmbeddedCover returns the cover picture a video file carries, as an MP4's or MKV's cover art (an attached picture), byte for byte as the file holds it, PNG or JPEG. It fails for a file that carries none.
func EmbeddedCover(ctx context.Context, ffmpeg, video string) ([]byte, error) {
	if ffmpeg == "" || video == "" {
		return nil, errors.New("cover source is missing")
	}
	// 0:v is every picture stream and 0:V those that are not attached pictures, so the two maps leave only the attached ones; with none, ffmpeg has no stream to write and fails.
	out, err := run(ctx, ffmpeg, "-nostdin", "-loglevel", "error", "-i", video, "-map", "0:v", "-map", "-0:V", "-frames:v", "1", "-c", "copy", "-f", "image2pipe", "pipe:1")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, errors.New("embedded cover is empty")
	}
	return []byte(out), nil
}

// GrabFrame returns one JPEG frame at at. It is used for lightweight video previews where a permanent poster file is unnecessary.
func GrabFrame(ctx context.Context, ffmpeg, video string, at time.Duration) ([]byte, error) {
	if ffmpeg == "" || video == "" {
		return nil, errors.New("frame source is missing")
	}
	return grabPosterFrame(ctx, ffmpeg, video, at)
}

// PosterTooDark reports whether path is a cover grabbed from a nearly black frame.
func PosterTooDark(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return false
	}
	luma, ok := jpegLuma(data)
	return ok && luma < minPosterLuma
}

func posterOffsets(d time.Duration) []time.Duration {
	if d <= 0 {
		return []time.Duration{time.Second, 3 * time.Second, 10 * time.Second, 0}
	}
	limit := d - 200*time.Millisecond
	if limit < 0 {
		limit = 0
	}
	raw := []time.Duration{d * 3 / 4, d / 2, d / 4, d / 10, time.Second, 0}
	out := make([]time.Duration, 0, len(raw))
	seen := map[time.Duration]bool{}
	for _, off := range raw {
		if off < 0 {
			off = 0
		}
		if off > limit {
			off = limit
		}
		off = off.Round(100 * time.Millisecond)
		if seen[off] {
			continue
		}
		seen[off] = true
		out = append(out, off)
	}
	return out
}

// VideoDuration is how long the video at path plays, read with ffmpeg, or 0 when ffmpeg cannot tell.
func VideoDuration(ffmpeg, path string) time.Duration { return videoDuration(ffmpeg, path) }

func videoDuration(ffmpeg, path string) time.Duration {
	_, _, _, duration, err := probePicture(ffmpeg, path)
	if err != nil {
		return 0
	}
	return duration
}

func grabPosterFrame(ctx context.Context, ffmpeg, video string, at time.Duration) ([]byte, error) {
	args := []string{"-nostdin", "-loglevel", "error", "-y"}
	if at > 0 {
		args = append(args, "-ss", strconv.FormatFloat(at.Seconds(), 'f', 3, 64))
	}
	args = append(args, "-i", video, "-frames:v", "1", "-vf", "scale=640:-2", "-f", "image2", "-c:v", "mjpeg", "pipe:1")
	out, err := run(ctx, ffmpeg, args...)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, errors.New("poster frame is empty")
	}
	return []byte(out), nil
}

func jpegLuma(data []byte) (float64, bool) {
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return 0, false
	}
	bounds := img.Bounds()
	var sum float64
	var n int
	for y := bounds.Min.Y; y < bounds.Max.Y; y += 4 {
		for x := bounds.Min.X; x < bounds.Max.X; x += 4 {
			r, g, b, _ := img.At(x, y).RGBA()
			sum += 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
			n++
		}
	}
	if n == 0 {
		return 0, false
	}
	return sum / float64(n) / 65535, true
}
