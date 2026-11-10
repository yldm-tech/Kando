package media

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kando/packages/core"
)

// readCapture feeds a captured stdout of the real yt-dlp, run with Download's arguments, through the reader line by line.
func readCapture(t *testing.T, name string) (*ytdlpReader, []core.Progress, []Info) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "ytdlp", name))
	if err != nil {
		t.Fatal(err)
	}
	var steps []core.Progress
	var infos []Info
	r := &ytdlpReader{progress: func(p core.Progress) { steps = append(steps, p) }, info: func(i Info) { infos = append(infos, i) }}
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		r.line(line)
	}
	return r, steps, infos
}

func TestYTDLPReaderFollowsAVideoAndItsSound(t *testing.T) {
	r, steps, infos := readCapture(t, "download-youtube.stdout")
	if len(infos) != 1 || infos[0].Title != "Me at the zoo" || !strings.HasPrefix(infos[0].Thumbnail, "https://i.ytimg.com/") || infos[0].Duration != 19*time.Second {
		t.Fatalf("info = %+v", infos)
	}
	if r.path != "/library/Me at the zoo [jNQXAC9IVRw]/video.mp4" {
		t.Fatalf("path = %q", r.path)
	}
	if len(steps) == 0 {
		t.Fatal("no progress")
	}
	last := -1.0
	half := false
	for _, p := range steps {
		if !p.Known {
			continue
		}
		if p.Fraction < last {
			t.Fatalf("progress went back from %v to %v", last, p.Fraction)
		}
		last = p.Fraction
		// The video is the first of two formats: when it has finished the whole download is half done.
		if p.Fraction == 0.5 {
			half = true
		}
	}
	if !half || last != 1 {
		t.Fatalf("progress did not pass half way and end at 1: half %v, last %v", half, last)
	}
	var speed bool
	for _, p := range steps {
		speed = speed || p.Speed > 0
	}
	if !speed {
		t.Fatal("no step said how fast it downloads")
	}
}

func TestYTDLPReaderFollowsFragments(t *testing.T) {
	_, steps, infos := readCapture(t, "download-hls.stdout")
	if len(infos) != 1 || infos[0].Title != "x36xhzz" {
		t.Fatalf("info = %+v", infos)
	}
	first, final := steps[0], steps[len(steps)-1]
	if !first.Known || first.Fraction > 0.01 {
		t.Fatalf("first step = %+v", first)
	}
	if !final.Known || final.Fraction != 1 {
		t.Fatalf("last step = %+v", final)
	}
	// Midway through, yt-dlp knows how long the rest takes.
	var eta bool
	for _, p := range steps {
		eta = eta || p.ETA > 0
	}
	if !eta {
		t.Fatal("no step said how long is left")
	}
}

func TestYTDLPReaderSkipsAStepItCannotRead(t *testing.T) {
	r := &ytdlpReader{}
	if _, ok := r.step(strings.Fields("downloading 1 2")); ok {
		t.Fatal("a short line was read")
	}
	p, ok := r.step(strings.Fields("downloading 1024 NA NA NA NA NA NA 18"))
	if !ok || p.Known {
		t.Fatalf("a step with no total = %+v, %v", p, ok)
	}
}

func TestWhisperFractionReadsTheRealProgressLines(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "whisper", "progress.stderr"))
	if err != nil {
		t.Fatal(err)
	}
	var got []float64
	for _, line := range strings.Split(string(data), "\n") {
		if f, ok := whisperFraction(line); ok {
			got = append(got, f)
		}
	}
	want := []float64{0.23, 0.50, 0.73, 0.98, 1}
	if len(got) != len(want) {
		t.Fatalf("fractions = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("fractions = %v, want %v", got, want)
		}
	}
}

func TestRunFollowingHandsOverLinesAsTheyCome(t *testing.T) {
	// A carriage return ends a line as a newline does, as whisper-cli and ffmpeg redraw one line.
	var lines []string
	stdout, stderr, err := runFollowing(context.Background(), true, func(l string) { lines = append(lines, l) }, "/bin/sh", "-c", `printf 'out\n'; printf 'a\rb\nc' >&2`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(lines, "|") != "a|b|c" || stdout != "out\n" || stderr != "a\rb\nc" {
		t.Fatalf("lines %q, stdout %q, stderr %q", lines, stdout, stderr)
	}
}

func TestRunFollowingKeepsTheErrorLines(t *testing.T) {
	_, _, err := runFollowing(context.Background(), false, func(string) {}, "/bin/sh", "-c", `printf 'WARNING: slow\nERROR: [youtube] x: Private video\n' >&2; exit 1`)
	if err == nil || !strings.Contains(err.Error(), "ERROR: [youtube] x: Private video") || strings.Contains(err.Error(), "WARNING") {
		t.Fatalf("err = %v", err)
	}
}

// captureLog sends the standard logger's output to a buffer until t ends.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	out, flags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	t.Cleanup(func() {
		log.SetOutput(out)
		log.SetFlags(flags)
	})
	return &buf
}

// A failed tool's error keeps only the end of its stderr, so the log gets the whole of it: the warnings before the error say what led to it.
func TestRunFollowingLogsTheWholeStderrOfAFailedTool(t *testing.T) {
	logged := captureLog(t)
	_, _, err := runFollowing(context.Background(), false, func(string) {}, "/bin/sh", "-c", `printf 'WARNING: first of many\n' >&2; i=0; while [ $i -lt 100 ]; do printf 'noise line %d\n' $i >&2; i=$((i+1)); done; printf 'ERROR: [youtube] x: Private video\n' >&2; exit 1`)
	if err == nil || strings.Contains(err.Error(), "WARNING: first of many") {
		t.Fatalf("err = %v", err)
	}
	for _, want := range []string{"sh failed: exit status 1", "WARNING: first of many", "noise line 50", "ERROR: [youtube] x: Private video"} {
		if !strings.Contains(logged.String(), want) {
			t.Fatalf("log lacks %q:\n%s", want, logged)
		}
	}
}

// The log keeps the last 64 KiB of a tool that printed more, where its error is.
func TestRunLogsTheEndOfALongStderr(t *testing.T) {
	logged := captureLog(t)
	_, err := run(context.Background(), "/bin/sh", "-c", `printf 'START\n' >&2; head -c 200000 /dev/zero | tr '\0' 'x' >&2; printf '\nthe end\n' >&2; exit 2`)
	if err == nil {
		t.Fatal("the tool did not fail")
	}
	got := logged.String()
	if !strings.Contains(got, "sh failed: exit status 2") || !strings.Contains(got, "the end") || strings.Contains(got, "START") {
		t.Fatalf("log of %d bytes: %.200q", len(got), got)
	}
	if len(got) > maxLoggedStderr+200 {
		t.Fatalf("log is %d bytes, more than the cap", len(got))
	}
}

// A tool stopped by its context did not fail, so nothing is logged for it.
func TestRunDoesNotLogAStoppedTool(t *testing.T) {
	logged := captureLog(t)
	for _, stream := range []bool{false, true} {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		var err error
		if stream {
			_, _, err = runFollowing(ctx, true, func(string) {}, "/bin/sh", "-c", `echo boom >&2; exec sleep 5`)
		} else {
			_, err = run(ctx, "/bin/sh", "-c", `echo boom >&2; exec sleep 5`)
		}
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v", err)
		}
	}
	if logged.Len() != 0 {
		t.Fatalf("logged %q", logged)
	}
}

func TestTraceReachesTheStageThatReports(t *testing.T) {
	var got []core.Progress
	var info Info
	ctx := WithTrace(context.Background(), &Trace{Progress: func(p core.Progress) { got = append(got, p) }, Info: func(i Info) { info = i }})
	reportProgress(ctx, core.Fraction(0.3))
	reportInfo(ctx, Info{Title: "t"})
	reportProgress(context.Background(), core.Fraction(0.9))
	if len(got) != 1 || got[0].Fraction != 0.3 || info.Title != "t" {
		t.Fatalf("progress %v, info %+v", got, info)
	}
}
