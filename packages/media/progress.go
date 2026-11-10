package media

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"kando/packages/core"
)

// What Download asks yt-dlp to print on stdout, each line under a prefix of its own: the video's title, thumbnail and length as soon as it has been looked up, before the formats are chosen, so a download that fails at choosing them still names the video (kando-info), the formats it will download, two for a video and its sound to be merged (kando-formats), a line for each step of each format's download (kando-progress), and, last, the path of the merged file, unprefixed (--print after_move:filepath).
const (
	ytdlpInfoPrefix     = "kando-info "
	ytdlpFormatsPrefix  = "kando-formats "
	ytdlpProgressPrefix = "kando-progress "

	ytdlpInfoTemplate     = "pre_process:" + ytdlpInfoPrefix + "%(.{id,title,thumbnail,duration})j"
	ytdlpFormatsTemplate  = "before_dl:" + ytdlpFormatsPrefix + "%(requested_formats.:.format_id)j"
	ytdlpProgressTemplate = "download:" + ytdlpProgressPrefix + "%(progress.status)s %(progress.downloaded_bytes)s %(progress.total_bytes)s %(progress.total_bytes_estimate)s %(progress.speed)s %(progress.eta)s %(progress.fragment_index)s %(progress.fragment_count)s %(info.format_id)s"
)

// ytdlpReader follows yt-dlp's stdout as Download asks it to print, line by line, and reports what it reads. The fraction is the whole download's: with two formats, the video's counts for the first half and its sound's for the second. It never goes back, as yt-dlp's estimate of a fragmented download's size can.
type ytdlpReader struct {
	progress func(core.Progress)
	info     func(Info)
	// formats are the format IDs yt-dlp said it will download, in order; path is the last line that had no prefix.
	formats []string
	path    string
	best    float64
}

func (r *ytdlpReader) line(line string) {
	switch {
	case strings.HasPrefix(line, ytdlpInfoPrefix):
		var raw struct {
			Title     string   `json:"title"`
			Thumbnail string   `json:"thumbnail"`
			Duration  *float64 `json:"duration"`
		}
		if json.Unmarshal([]byte(strings.TrimPrefix(line, ytdlpInfoPrefix)), &raw) != nil {
			return
		}
		info := Info{Title: strings.TrimSpace(raw.Title), Thumbnail: raw.Thumbnail}
		if raw.Duration != nil && *raw.Duration > 0 {
			info.Duration = time.Duration(*raw.Duration * float64(time.Second))
		}
		if r.info != nil {
			r.info(info)
		}
	case strings.HasPrefix(line, ytdlpFormatsPrefix):
		var ids []string
		if json.Unmarshal([]byte(strings.TrimPrefix(line, ytdlpFormatsPrefix)), &ids) == nil {
			r.formats = ids
		}
	case strings.HasPrefix(line, ytdlpProgressPrefix):
		if p, ok := r.step(strings.Fields(strings.TrimPrefix(line, ytdlpProgressPrefix))); ok && r.progress != nil {
			r.progress(p)
		}
	case strings.TrimSpace(line) != "":
		r.path = strings.TrimSpace(line)
	}
}

// step reads one kando-progress line: status, bytes downloaded, total bytes, estimated total bytes, speed, ETA in seconds, fragment index, fragment count and format ID, NA where yt-dlp does not know.
func (r *ytdlpReader) step(fields []string) (core.Progress, bool) {
	if len(fields) != 9 {
		return core.Progress{}, false
	}
	num := func(i int) float64 {
		v, err := strconv.ParseFloat(fields[i], 64)
		if err != nil || v < 0 {
			return 0
		}
		return v
	}
	status, done, total, estimate, fragment, fragments := fields[0], num(1), num(2), num(3), num(6), num(7)
	p := core.Progress{Speed: num(4), ETA: time.Duration(num(5) * float64(time.Second))}
	part, known := 0.0, true
	switch {
	case status == "finished":
		part = 1
	case total > 0:
		part = done / total
	case estimate > 0:
		part = done / estimate
	case fragments > 0:
		part = fragment / fragments
	default:
		known = false
	}
	if !known {
		return p, true
	}
	count, index := max(len(r.formats), 1), 0
	if i := slices.Index(r.formats, fields[8]); i >= 0 {
		index = i
	}
	whole := (float64(index) + min(part, 1)) / float64(count)
	r.best = max(r.best, whole)
	p.Fraction, p.Known = min(r.best, 1), true
	return p, true
}

// whisperProgress matches the line whisper-cli prints on stderr with -pp, such as "whisper_print_progress_callback: progress =  23%".
var whisperProgress = regexp.MustCompile(`progress =\s*(\d+)%`)

// whisperFraction reads a whisper-cli progress line as a fraction of the transcription.
func whisperFraction(line string) (float64, bool) {
	m := whisperProgress.FindStringSubmatch(line)
	if m == nil {
		return 0, false
	}
	percent, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return float64(percent) / 100, true
}

// runFollowing runs a tool and hands each line it prints on one stream, stdout or stderr as fromStderr says, to onLine on the calling goroutine as it comes, a carriage return ending a line as a newline does. It returns what the tool printed on each stream, the followed one's last 64 KiB, and, on failure, an error with what stderr says went wrong (toolError).
func runFollowing(ctx context.Context, fromStderr bool, onLine func(string), name string, args ...string) (stdout, stderr string, err error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var otherBuf, followed bytes.Buffer
	var pipe io.ReadCloser
	if fromStderr {
		cmd.Stdout = &otherBuf
		pipe, err = cmd.StderrPipe()
	} else {
		cmd.Stderr = &otherBuf
		pipe, err = cmd.StdoutPipe()
	}
	if err != nil {
		return "", "", err
	}
	if err := cmd.Start(); err != nil {
		return "", "", fmt.Errorf("%s: %w", filepath.Base(name), err)
	}
	scanner := bufio.NewScanner(io.TeeReader(pipe, &limitedBuffer{buf: &followed, max: 64 << 10}))
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	scanner.Split(scanLinesOrReturns)
	for scanner.Scan() {
		onLine(scanner.Text())
	}
	// Whatever the tool still prints after a line too long to scan is read and dropped, so it does not block writing it.
	_, _ = io.Copy(io.Discard, pipe)
	waitErr := cmd.Wait()
	stdout, stderr = followed.String(), otherBuf.String()
	if fromStderr {
		stdout, stderr = otherBuf.String(), followed.String()
	}
	if waitErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return stdout, stderr, ctxErr
		}
		logToolFailure(name, waitErr, stderr)
		return stdout, stderr, fmt.Errorf("%s: %w: %s", filepath.Base(name), waitErr, toolError(stderr))
	}
	return stdout, stderr, nil
}

// limitedBuffer keeps the last max bytes written to it at most, dropping the oldest, so a tool's long output does not grow without end while its last lines, where the error is, are kept.
type limitedBuffer struct {
	buf *bytes.Buffer
	max int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.buf.Write(p)
	if over := b.buf.Len() - b.max; over > 0 {
		b.buf.Next(over)
	}
	return len(p), nil
}

// scanLinesOrReturns is bufio.ScanLines that also ends a line at a carriage return.
func scanLinesOrReturns(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// toolError is what a failed tool's stderr says: the lines yt-dlp starts with "ERROR:", whole, when there are any, since the reason is in them and a long one would lose its start to a plain tail; otherwise the end of stderr.
func toolError(stderr string) string {
	var errs []string
	for _, line := range strings.Split(stderr, "\n") {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, "ERROR:") {
			errs = append(errs, line)
		}
	}
	if len(errs) > 0 {
		return tail(strings.Join(errs, "\n"), 1200)
	}
	return tail(stderr, 400)
}
