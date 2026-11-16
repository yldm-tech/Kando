package film

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"kando/packages/media"
)

// The system's player plays what the platform's media framework reads: on macOS AVFoundation, which takes MP4 and QuickTime files of the codecs it knows but not an MKV or a WebM, as many downloaded and recorded films are. Such a film plays from a copy in an MP4 the player can open, which ffmpeg, the one the pipeline uses, makes: first by copying its streams into the new container, which takes seconds for a whole film, and when the player cannot play that either, by encoding its picture as H.264 and its sound as AAC, which every platform's player plays.

// RemuxStep is how a copy for the system's player is made.
type RemuxStep int

const (
	// RemuxCopy copies the video's picture and sound into an MP4 as they are.
	RemuxCopy RemuxStep = iota + 1
	// RemuxEncode encodes them as H.264 and AAC in an MP4.
	RemuxEncode
)

// RemuxPath is where the copy of the video at path made by step goes, in dir: named after the video's path, size and time of change, so a video changed since makes a copy of its own.
func RemuxPath(dir, path string, step RemuxStep) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(path + "\x00" + strconv.FormatInt(info.Size(), 10) + "\x00" + strconv.FormatInt(info.ModTime().UnixNano(), 10)))
	return filepath.Join(dir, hex.EncodeToString(sum[:12])+"-"+strconv.Itoa(int(step))+".mp4"), nil
}

// RemuxArgs are ffmpeg's arguments to make the copy of src at dest by step: the first picture and the first sound track, without the subtitles and data streams an MP4 cannot hold, moving the index to the front so the player starts at once.
func RemuxArgs(src, dest string, step RemuxStep) []string {
	args := []string{"-nostdin", "-loglevel", "error", "-y", "-i", src, "-map", "0:v:0?", "-map", "0:a:0?", "-sn", "-dn"}
	if step == RemuxEncode {
		args = append(args, "-c:v", "h264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "192k")
	} else {
		args = append(args, "-c", "copy")
	}
	return append(args, "-movflags", "+faststart", "-f", "mp4", dest)
}

// Remux makes the copy of the video at src for the system's player at dest by step with ffmpeg, unless one is there already, and clears dir of the copies of other videos, as one film plays at a time. A copy cut short, by ctx or a failure, is removed.
func Remux(ctx context.Context, ffmpeg, src, dest string, step RemuxStep) error {
	if ffmpeg == "" {
		return errors.New("no ffmpeg to copy the video with")
	}
	if info, err := os.Stat(dest); err == nil && info.Size() > 0 {
		return nil
	}
	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	keep := strings.TrimSuffix(filepath.Base(dest), filepath.Ext(dest))
	keep = keep[:strings.LastIndexByte(keep, '-')+1]
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), keep) {
				_ = os.Remove(filepath.Join(dir, e.Name()))
			}
		}
	}
	// Each run writes a file of its own until the copy is whole, so a run stopped as the same film opens again does not remove what the next one writes.
	part, err := os.CreateTemp(dir, filepath.Base(dest)+"-*.part")
	if err != nil {
		return err
	}
	tmp := part.Name()
	_ = part.Close()
	cmd := media.Command(ctx, ffmpeg, RemuxArgs(src, tmp, step)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.Remove(tmp)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
