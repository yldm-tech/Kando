package media

import (
	"context"
	"errors"
	"path"
	"strings"

	"kando/packages/core"
)

// SourceReader is how a job reaches a video on one of the user's media sources, a NAS share or a server the app reads (a kando-source:// address, core.PlatformSource): Copy copies the video at source into dir, the library's folder, reporting how far it got to the context's Trace (ReportProgress), and returns the file's path. The app gives Tools one (Tools.Sources); the copy is then a film of the library like any other.
type SourceReader interface {
	Copy(ctx context.Context, source, dir string) (string, error)
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
