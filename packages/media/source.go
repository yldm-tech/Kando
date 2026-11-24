package media

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"kando/packages/core"
)

// SourceReader is how a job reaches a video on one of the user's media sources, a NAS share or a server the app reads (a kando-source:// address, core.PlatformSource): Copy copies the video at source into dir, the library's folder, reporting how far it got to the context's Trace (ReportProgress), and returns the file's path; the copy is then a film of the library like any other. URL is where ffmpeg reads the video at source now, without copying it, for a film indexed from the source (core.Job.Linked): the app's loopback server's address, served until ctx ends. The app gives Tools one (Tools.Sources).
type SourceReader interface {
	Copy(ctx context.Context, source, dir string) (string, error)
	URL(ctx context.Context, source string) (string, error)
}

// SourceStreamer is a Pipeline that reads a media source's video where it is, for a film indexed from the source (core.Job.Linked): StreamSource is the address ffmpeg reads it at until ctx ends, which Make hears the film from in place of a file. Tools is one, through its SourceReader.
type SourceStreamer interface {
	StreamSource(ctx context.Context, source string) (string, error)
}

// StreamSource is the address ffmpeg reads the video at the media source address source at until ctx ends (SourceStreamer), from Sources. Offline mode is the SourceReader's to judge, as Copy's is.
func (t Tools) StreamSource(ctx context.Context, source string) (string, error) {
	if t.Sources == nil {
		return "", ErrNoSourceReader
	}
	reportInfo(ctx, Info{Title: sourceTitle(source)})
	return t.Sources.URL(ctx, source)
}

// ErrNoHTTP is CheckHTTP's error for an ffmpeg built without its http protocol, which cannot read an indexed film from the app's loopback server.
var ErrNoHTTP = errors.New("media: this ffmpeg cannot read network addresses (no http protocol)")

// HTTPChecker is a Pipeline that can tell whether its ffmpeg reads http addresses, as a film indexed from a media source needs: CheckHTTP is nil when it does, ErrNoHTTP when it does not.
type HTTPChecker interface {
	CheckHTTP(ctx context.Context) error
}

// CheckHTTP reports whether FFmpeg reads http addresses (HTTPChecker): its list of protocols names http among those it reads. The usual builds do; one built without networking does not.
func (t Tools) CheckHTTP(ctx context.Context) error {
	if t.FFmpeg == "" {
		return errors.New("ffmpeg is not installed")
	}
	cmd := Command(ctx, t.FFmpeg, "-hide_banner", "-protocols")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("ffmpeg -protocols: %w", err)
	}
	if !readsHTTP(string(out)) {
		return ErrNoHTTP
	}
	return nil
}

// readsHTTP reports whether out, what ffmpeg -protocols prints, names http among the protocols it reads: the names under "Input:" until "Output:".
func readsHTTP(out string) bool {
	input := false
	for _, line := range strings.Split(out, "\n") {
		switch strings.TrimSpace(line) {
		case "Input:":
			input = true
		case "Output:":
			input = false
		case "http":
			if input {
				return true
			}
		}
	}
	return false
}

// ErrNoSourceReader is Download's error for a media source's video while Tools has no SourceReader.
var ErrNoSourceReader = errors.New("media: no media sources are set up to read this video from")

// IsSourceRef reports whether source is a video's address on a media source (core.SourceScheme), which Download copies rather than downloads.
func IsSourceRef(source string) bool {
	link, ok := core.ParseLink(source)
	return ok && link.Platform == core.PlatformSource
}

// sourceTitle is what the video at the media source address source is called before the job knows better: its file's name without the extension.
func sourceTitle(source string) string {
	link, _ := core.ParseLink(source)
	name := path.Base(link.ID)
	return strings.TrimSuffix(name, path.Ext(name))
}

// copySource copies the video at the media source address source into OutputDir with Sources, naming the job after its file first. Offline mode is the SourceReader's to judge: a source on the local network is read in it, one on the internet is not.
func (t Tools) copySource(ctx context.Context, source string) (string, error) {
	if t.Sources == nil {
		return "", ErrNoSourceReader
	}
	reportInfo(ctx, Info{Title: sourceTitle(source)})
	return t.Sources.Copy(ctx, source, t.OutputDir)
}

// ReportProgress tells the Trace of ctx how far a stage has gone, as a SourceReader's copy does.
func ReportProgress(ctx context.Context, p core.Progress) { reportProgress(ctx, p) }
