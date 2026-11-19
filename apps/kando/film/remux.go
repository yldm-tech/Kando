package film

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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

// ErrMissingPlugins is PlayErr's ID for a video the system's player cannot play because GStreamer, which plays videos on Linux, lacks the plug-ins for it (MissingPlugins); the page says which to install and offers GStreamerInstall to copy.
const ErrMissingPlugins = "video-plugins"

// The PlayErr IDs of a video moved to a storage location: the location is no longer in the settings, could not be reached, no longer has the video, or neither the system's player nor the copy made of it from there plays it (Remux reads it from the location). The page offers the settings for the first three, rather than a file manager on a video that is not on this computer.
const (
	ErrStoredGone        = "stored-gone"
	ErrStoredUnreachable = "stored-unreachable"
	ErrStoredMissing     = "stored-missing"
	ErrStoredUnplayable  = "stored-unplayable"
)

// GStreamerInstall is the command that installs GTK 4's GStreamer media backend and the GStreamer plug-ins that play the videos Kando downloads, on Debian and Ubuntu, as the README gives it.
const GStreamerInstall = "sudo apt install libgtk-4-media-gstreamer gstreamer1.0-plugins-good gstreamer1.0-plugins-bad gstreamer1.0-libav"

// MissingPlugins reports whether err, the system player's, says it has no media backend or no plug-in for the video, as GTK and GStreamer say on Linux (NSGo passes GStreamer's message on, with a hint of its own once it says a plug-in is missing). No copy ffmpeg makes would play then either, as its H.264 needs gst-libav too. GTK and GStreamer translate their messages, so in another language than English it is not recognized, and the copies are tried.
func MissingPlugins(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, says := range []string{"could not find a media module", "missing a plug-in", "No suitable plugins", "No installed GStreamer plug-in"} {
		if strings.Contains(msg, says) {
			return true
		}
	}
	return false
}

// RemuxPath is where the copy of the video at path made by step goes, in dir: named after the video's path, size and time of change, so a video changed since makes a copy of its own.
func RemuxPath(dir, path string, step RemuxStep) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	return RemuxPathOf(dir, path+"\x00"+strconv.FormatInt(info.Size(), 10)+"\x00"+strconv.FormatInt(info.ModTime().UnixNano(), 10), step), nil
}

// RemuxPathOf is where the copy made by step of the video that id names goes, in dir, for a video that is not a file on this computer, as one moved to a storage location: id says which video it is and which version of it, as RemuxPath's path, size and time of change do.
func RemuxPathOf(dir, id string, step RemuxStep) string {
	sum := sha256.Sum256([]byte(id))
	return filepath.Join(dir, hex.EncodeToString(sum[:12])+"-"+strconv.Itoa(int(step))+".mp4")
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

// Remux makes the copy of the video at src, a file or an http address ffmpeg reads by ranges, for the system's player at dest by step with ffmpeg, unless one is there already, and clears dir of the copies of other videos, as one film plays at a time. A copy cut short, by ctx or a failure, is removed. progress, when it is not nil, hears how far into the video the copy has got as ffmpeg reports it (-progress), from the goroutine Remux runs on.
func Remux(ctx context.Context, ffmpeg, src, dest string, step RemuxStep, progress func(time.Duration)) error {
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
	// ffmpeg writes key=value lines of how far it got to standard output, out_time_us among them, and its errors to standard error.
	cmd := media.Command(ctx, ffmpeg, append([]string{"-progress", "pipe:1", "-nostats"}, RemuxArgs(src, tmp, step)...)...)
	var errs bytes.Buffer
	cmd.Stderr = &errs
	err = remuxRun(cmd, progress)
	if err != nil {
		_ = os.Remove(tmp)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(errs.String()))
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// remuxRun runs cmd, an ffmpeg writing -progress to its standard output, to its end, telling progress each time it reports how far into the video it got.
func remuxRun(cmd *exec.Cmd, progress func(time.Duration)) error {
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	lines := bufio.NewScanner(out)
	for lines.Scan() {
		value, ok := strings.CutPrefix(lines.Text(), "out_time_us=")
		if !ok || progress == nil {
			continue
		}
		if us, err := strconv.ParseInt(value, 10, 64); err == nil && us >= 0 {
			progress(time.Duration(us) * time.Microsecond)
		}
	}
	// A line too long to scan stops the scan; the rest is read, so ffmpeg does not wait on a full pipe.
	_, _ = io.Copy(io.Discard, out)
	return cmd.Wait()
}
