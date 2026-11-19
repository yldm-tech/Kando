package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// fakeFilms is a desktop's FilmService with one film, f1, which counts the changes it was asked for.
type fakeFilms struct {
	changes int
	gone    bool
}

func (f *fakeFilms) find(id string) error {
	if f.gone || id != "f1" {
		return Errorf(CodeNotFound, "no film %q", id)
	}
	return nil
}

func (f *fakeFilms) RetranslateFilm(_ context.Context, caller Caller, ref FilmRef) (Job, error) {
	if err := f.find(ref.ID); err != nil {
		return Job{}, err
	}
	f.changes++
	return Job{ID: ref.ID, Device: caller.Name, State: JobWaiting}, nil
}

func (f *fakeFilms) ExportFilm(_ context.Context, _ Caller, ref FilmRef) (SubtitleFile, error) {
	if err := f.find(ref.ID); err != nil {
		return SubtitleFile{}, err
	}
	return SubtitleFile{Name: "talk.dual.srt", Data: []byte("1\n00:00:01,000 --> 00:00:02,000\n你好\nHello\n\n")}, nil
}

func (f *fakeFilms) DeleteFilm(_ context.Context, _ Caller, ref FilmRef) error {
	if err := f.find(ref.ID); err != nil {
		return err
	}
	f.changes++
	f.gone = true
	return nil
}

func (f *fakeFilms) DownloadFilmVideo(_ context.Context, _ Caller, ref FilmRef) (Job, error) {
	if err := f.find(ref.ID); err != nil {
		return Job{}, err
	}
	f.changes++
	return Job{ID: ref.ID, State: JobWaiting}, nil
}

// A phone has the computer translate a film again and download its video, each answered with the job that does it, takes its subtitles as a file, and deletes it; a film gone since is not found, and a call naming no film is refused before the desktop's work.
func TestFilmsOverV2(t *testing.T) {
	ctx := context.Background()
	f := &fakeFilms{}
	desk := newDesk(&fakeService{receive: ReceiveQueue})
	desk.Films = f
	c := v2Desktop(t, desk)
	job, err := c.RetranslateFilm(ctx, "f1")
	if err != nil || job.ID != "f1" || job.State != JobWaiting {
		t.Fatalf("retranslate %+v %v", job, err)
	}
	if job, err = c.DownloadFilmVideo(ctx, "f1"); err != nil || job.ID != "f1" {
		t.Fatalf("video %+v %v", job, err)
	}
	file, err := c.ExportFilm(ctx, "f1")
	if err != nil || file.Name != "talk.dual.srt" || string(file.Data) != "1\n00:00:01,000 --> 00:00:02,000\n你好\nHello\n\n" {
		t.Fatalf("export %+v %v", file, err)
	}
	if err := c.DeleteFilm(ctx, "f1"); err != nil {
		t.Fatal(err)
	}
	_, err = c.ExportFilm(ctx, "f1")
	wantError(t, err, http.StatusNotFound, CodeNotFound)
	_, err = c.RetranslateFilm(ctx, "")
	wantError(t, err, http.StatusBadRequest, CodeInvalid)
	if f.changes != 3 {
		t.Fatalf("%d changes reached the desktop, want 3", f.changes)
	}
}

// A desktop that does not act on its films answers their calls unsupported, and a read-only device is refused every one that changes a film before the desktop's work, though it may take a film's subtitles as a file, which change nothing.
func TestFilmsUnsupportedAndReadOnly(t *testing.T) {
	ctx := context.Background()
	desk := newDesk(&fakeService{receive: ReceiveQueue})
	_, err := v2Desktop(t, desk).ExportFilm(ctx, "f1")
	wantError(t, err, http.StatusNotImplemented, CodeUnsupported)

	f := &fakeFilms{}
	desk.Films = f
	reader := Caller{DeviceID: "d1", Name: "Pixel", ReadOnly: true}
	args, _ := json.Marshal(FilmRef{ID: "f1"})
	for _, op := range []string{OpFilmRetranslate, OpFilmDelete, OpFilmVideo} {
		if !Mutates(op) {
			t.Fatalf("%s does not count as a change", op)
		}
		r := desk.Answer(ctx, reader, Call{ID: "w-" + op, V: CallVersion, Op: op, Args: args})
		if r.Error == nil || r.Error.Code != CodeReadOnly {
			t.Fatalf("%s of a read-only device answered %s %+v", op, r.Result, r.Error)
		}
	}
	if f.changes != 0 {
		t.Fatalf("%d changes of a read-only device reached the desktop", f.changes)
	}
	r := desk.Answer(ctx, reader, Call{ID: "r", V: CallVersion, Op: OpFilmExport, Args: args})
	var file SubtitleFile
	if r.Error != nil || Mutates(OpFilmExport) || json.Unmarshal(r.Result, &file) != nil || file.Name == "" {
		t.Fatalf("a read-only device's export: %s %+v", r.Result, r.Error)
	}
}
