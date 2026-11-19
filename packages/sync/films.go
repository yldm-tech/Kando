package sync

import "context"

// What a paired phone does to a film of the computer's library (yldm-tech/Kando#221), as the options of a film in its Library offer: translate it again (OpFilmRetranslate), take its subtitles as a file to share (OpFilmExport), delete it from the library (OpFilmDelete), and download the video of a film that keeps none (OpFilmVideo, Film.SubtitlesOnly). The film is named by its ID as the computer lists it (Film.ID); a film of a locked library, or one the computer does not list, is not found. Retranslating, deleting and downloading the video change the computer (Mutates), so a read-only device (Caller.ReadOnly) is refused them; exporting only reads the subtitles, which such a device reads anyway (PathSubtitles), so it may. Like every v2 call they go over any transport.
const (
	// OpFilmRetranslate takes FilmRef and answers the Job that translates every line of the film again into the language the computer translates into, once queued. The film stays as it is until the new lines are written; a film the computer did not make the subtitles of, or whose job is waiting or running, answers CodeConflict, and one the computer cannot translate now, without an AI service or a language to translate into, CodeNotReady with why.
	OpFilmRetranslate = "films.retranslate"
	// OpFilmExport takes FilmRef and answers SubtitleFile: the film's lines with the user's corrections, as the computer's own Export Subtitles writes them, in the format and with the lines it was last told to.
	OpFilmExport = "films.export"
	// OpFilmDelete takes FilmRef and answers nothing once the film is off the library: its folder, or its video and subtitles, go to the computer's Trash, as the computer's own Remove from Library does. A film whose files could not go answers CodeNotReady with why, and stays.
	OpFilmDelete = "films.delete"
	// OpFilmVideo takes FilmRef and answers the Job that downloads the video of a film that keeps none (Film.SubtitlesOnly) beside its subtitles, once queued; the film is listed with its video once the job is done. A film with its video, or whose job is waiting or running, answers CodeConflict.
	OpFilmVideo = "films.download_video"
)

// FilmRef names a film by its ID (Film.ID).
type FilmRef struct {
	ID string `json:"id"`
}

// SubtitleFile is a film's subtitles as a file: the name the computer gives it and what it holds.
type SubtitleFile struct {
	Name string `json:"name"`
	Data []byte `json:"data"`
}

// MaxSubtitleFile is the largest SubtitleFile a desktop answers; a longer one answers CodeInvalid, as a reply cannot be longer than a phone reads.
const MaxSubtitleFile = 8 << 20

// FilmService is what a desktop does for the films' calls, whichever transport carries them. Its errors are as Service's.
type FilmService interface {
	RetranslateFilm(ctx context.Context, caller Caller, ref FilmRef) (Job, error)
	ExportFilm(ctx context.Context, caller Caller, ref FilmRef) (SubtitleFile, error)
	DeleteFilm(ctx context.Context, caller Caller, ref FilmRef) error
	DownloadFilmVideo(ctx context.Context, caller Caller, ref FilmRef) (Job, error)
}

// answerFilms runs a call of the films.
func (d *Desk) answerFilms(ctx context.Context, caller Caller, call Call) (any, error) {
	if d.Films == nil {
		return nil, Errorf(CodeUnsupported, "this desktop does not act on its films for phones")
	}
	var ref FilmRef
	if err := args(call, &ref); err != nil {
		return nil, err
	}
	if ref.ID == "" {
		return nil, Errorf(CodeInvalid, "%s names no film", call.Op)
	}
	switch call.Op {
	case OpFilmRetranslate:
		return d.Films.RetranslateFilm(ctx, caller, ref)
	case OpFilmExport:
		return d.Films.ExportFilm(ctx, caller, ref)
	case OpFilmVideo:
		return d.Films.DownloadFilmVideo(ctx, caller, ref)
	}
	return struct{}{}, d.Films.DeleteFilm(ctx, caller, ref)
}

// RetranslateFilm has the computer translate film id again, and answers the job that does.
func (c *Client) RetranslateFilm(ctx context.Context, id string) (Job, error) {
	var out Job
	err := c.Call(ctx, OpFilmRetranslate, FilmRef{ID: id}, &out)
	return out, err
}

// ExportFilm is film id's subtitles as a file.
func (c *Client) ExportFilm(ctx context.Context, id string) (SubtitleFile, error) {
	var out SubtitleFile
	err := c.Call(ctx, OpFilmExport, FilmRef{ID: id}, &out)
	return out, err
}

// DeleteFilm deletes film id from the computer's library.
func (c *Client) DeleteFilm(ctx context.Context, id string) error {
	return c.Call(ctx, OpFilmDelete, FilmRef{ID: id}, nil)
}

// DownloadFilmVideo has the computer download the video of film id, which keeps none, and answers the job that does.
func (c *Client) DownloadFilmVideo(ctx context.Context, id string) (Job, error) {
	var out Job
	err := c.Call(ctx, OpFilmVideo, FilmRef{ID: id}, &out)
	return out, err
}
