package download

import (
	"context"
	"net/http"

	"kando/apps/mobile/nas"
	"kando/packages/sync"
)

// FromSource is the file at Path of a NAS or a server the phone reaches itself (package nas), as a Source of a film: its video read from where the part kept ends, by the connection Reader gives, which stays the caller's; it has no subtitles and no poster, as a file on a NAS is a video alone. Its version (nas.Version) stands for the ETag, so a part is gone on with only while the file did not change.
type FromSource struct {
	Reader func(ctx context.Context) (nas.Reader, error)
	Path   string
}

// Subtitles are none: the film plays without, as the phone makes none itself.
func (s FromSource) Subtitles(context.Context, string) ([]sync.Cue, error) { return []sync.Cue{}, nil }

// Poster is none, which Film asks for only of a film its record says has one.
func (s FromSource) Poster(context.Context, string) ([]byte, string, error) {
	return nil, "", &sync.Error{Status: http.StatusNotFound}
}

// Video reads the file from byte from to its end, from the start when the file is not the one ifRange names or is no longer than from; to is not used, as Film asks for the rest of the file.
func (s FromSource) Video(ctx context.Context, _ string, from, _ int64, ifRange string) (*sync.Part, error) {
	r, err := s.Reader(ctx)
	if err != nil {
		return nil, err
	}
	info, err := r.Stat(ctx, s.Path)
	if err != nil {
		return nil, err
	}
	version := nas.Version(info)
	if ifRange != version || from >= info.Size {
		from = 0
	}
	body, err := r.Open(ctx, s.Path, from, -1)
	if err != nil {
		return nil, err
	}
	return &sync.Part{Body: body, Start: from, End: info.Size - 1, Total: info.Size, ETag: version, ContentType: sync.MediaType(s.Path)}, nil
}
