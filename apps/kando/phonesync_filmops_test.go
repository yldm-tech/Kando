package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/settings"
	"kando/apps/kando/shelf"
	"kando/packages/core"
	psync "kando/packages/sync"

	ns "github.com/nsgo-dev/nsgo/core"
)

// A phone takes a film's subtitles as a file, as the export dialog writes them; has a film translated again and the video of a film that keeps none downloaded, each answered with the job, which waits in the queue; and deletes a film, whose folder goes to the Trash. A film Kando did not make the subtitles of has none to translate again, a film with its video none to download, A read-only phone exports but changes nothing, and a locked library's films are not found. The page's list of what the phones did says what each phone changed.
func TestPhoneSyncActsOnAFilm(t *testing.T) {
	a, tt, updates, dir, c := phonePaired(t)
	// The library's queued clip would run once the jobs the phone queues are stopped as the test ends, and block the queue in its turn.
	a.library.Jobs = slices.DeleteFunc(a.library.Jobs, func(j core.Job) bool { return j.ID == "clip" })
	a.prefs.language = "简体中文"
	// A film that keeps no video: its subtitles, and nothing where its sound was.
	lines := filepath.Join(dir, "Lines", "lines.webm")
	remoteWriteFile(t, filepath.Join(dir, "Lines", "lines.bilingual.srt"), "1\n00:00:01,000 --> 00:00:02,000\n早\nMorning\n\n")
	a.library.Jobs = append(a.library.Jobs, core.Job{ID: "lines", Source: "https://www.youtube.com/watch?v=lines00", Title: "Lines", Language: "简体中文", Stage: core.StageComplete,
		VideoPath: lines, SubtitlePath: filepath.Join(dir, "Lines", "lines.bilingual.srt"), SubtitlesOnly: true, LibraryDir: dir, CompletedAt: time.Now()})
	refresh(tt, a)
	var trashed []string
	prev := trashItem
	trashItem = func(_ *ns.Scope, path string) error {
		trashed = append(trashed, path)
		return os.RemoveAll(path)
	}
	t.Cleanup(func() { trashItem = prev })

	h, err := phoneCall(t, a, tt, updates, c.Hello)
	for _, op := range []string{psync.OpFilmRetranslate, psync.OpFilmExport, psync.OpFilmDelete, psync.OpFilmVideo} {
		if err != nil || !slices.Contains(h.Ops, op) {
			t.Fatalf("hello %+v, %v; want %s among the operations", h, err, op)
		}
	}
	lib := phoneLibraryID(dir)
	list, err := phoneCall(t, a, tt, updates, func(ctx context.Context) ([]psync.Film, error) { return c.Films(ctx, lib) })
	if err != nil {
		t.Fatal(err)
	}
	var interview string
	for _, f := range list {
		if strings.Contains(strings.ToLower(f.Title), "interview") {
			interview = f.ID
		}
		if f.ID == "lines" && (!f.SubtitlesOnly || f.VideoSize != 0) {
			t.Fatalf("the film that keeps no video is listed as %+v", f)
		}
	}
	if interview == "" || !slices.ContainsFunc(list, func(f psync.Film) bool { return f.ID == "lines" }) {
		t.Fatalf("films %+v", list)
	}

	file, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.SubtitleFile, error) { return c.ExportFilm(ctx, "yt") })
	if err != nil || file.Name != a.export.choice.FileName(filepath.Join(dir, "Rust Talk", "talk.mp4")) || !strings.Contains(string(file.Data), "你好") || !strings.Contains(string(file.Data), "Hello there") {
		t.Fatalf("export %q %q, %v", file.Name, file.Data, err)
	}

	_, err = phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.Job, error) { return c.RetranslateFilm(ctx, interview) })
	phoneWantCode(t, err, http.StatusConflict, psync.CodeConflict)
	job, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.Job, error) { return c.RetranslateFilm(ctx, "yt") })
	// The job translates alone, neither downloading nor transcribing the film again.
	if err != nil || job.ID != "yt" || (job.State != psync.JobWaiting && job.State != psync.JobRunning) || len(job.Steps) != 3 || job.Steps[0].State != psync.StepSkipped || job.Steps[1].State != psync.StepSkipped {
		t.Fatalf("retranslate %+v, %v", job, err)
	}
	if act := a.phone.activity[0]; act.Kind != settings.ActivityRetranslated || act.Title != "Rust Talk" {
		t.Fatalf("activity %+v", act)
	}

	_, err = phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.Job, error) { return c.DownloadFilmVideo(ctx, interview) })
	phoneWantCode(t, err, http.StatusConflict, psync.CodeConflict)
	video, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.Job, error) { return c.DownloadFilmVideo(ctx, "lines") })
	if err != nil || video.ID != "lines" || (video.State != psync.JobWaiting && video.State != psync.JobRunning) {
		t.Fatalf("download the video %+v, %v", video, err)
	}
	if got, _ := a.library.Find("lines"); got.Redo != core.RedoVideo || got.Quality == core.QualitySubtitles {
		t.Fatalf("the film's job %+v does not download its video", got)
	}
	if act := a.phone.activity[0]; act.Kind != settings.ActivityVideo || act.Title != "Lines" {
		t.Fatalf("activity %+v", act)
	}
	// Stopped, the download puts the film back as it was, which keeps the queue from running it once the translation is stopped as the test ends.
	if _, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.Job, error) { return c.ActOnJob(ctx, psync.OpJobCancel, "lines") }); err != nil {
		t.Fatal(err)
	}

	// Read-only, the phone exports but deletes nothing.
	dev := a.phone.devices[0].ID
	if err := a.phone.server.SetReadOnly(dev, true); err != nil {
		t.Fatal(err)
	}
	_, err = phoneCall(t, a, tt, updates, func(ctx context.Context) (struct{}, error) { return struct{}{}, c.DeleteFilm(ctx, interview) })
	phoneWantCode(t, err, http.StatusForbidden, psync.CodeReadOnly)
	if _, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.SubtitleFile, error) { return c.ExportFilm(ctx, interview) }); err != nil {
		t.Fatal(err)
	}
	if err := a.phone.server.SetReadOnly(dev, false); err != nil {
		t.Fatal(err)
	}

	if _, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (struct{}, error) { return struct{}{}, c.DeleteFilm(ctx, interview) }); err != nil {
		t.Fatal(err)
	}
	if want := []string{filepath.Join(dir, "Interview")}; !slices.Equal(trashed, want) {
		t.Fatalf("moved %q to the Trash, want %q", trashed, want)
	}
	if act := a.phone.activity[0]; act.Kind != settings.ActivityDeleted || !strings.Contains(strings.ToLower(act.Title), "interview") {
		t.Fatalf("activity %+v", act)
	}
	list, err = phoneCall(t, a, tt, updates, func(ctx context.Context) ([]psync.Film, error) { return c.Films(ctx, lib) })
	if err != nil || slices.ContainsFunc(list, func(f psync.Film) bool { return f.ID == interview }) {
		t.Fatalf("films once one was deleted %+v, %v", list, err)
	}

	// Locked, the library's films are not there for a phone.
	l, err := shelf.NewLock("pw")
	if err != nil {
		t.Fatal(err)
	}
	onEngine(a, func() { a.library.SetLock(dir, l) })
	_, err = phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.SubtitleFile, error) { return c.ExportFilm(ctx, "lines") })
	phoneWantCode(t, err, http.StatusNotFound, psync.CodeNotFound)
	onEngine(a, func() { a.library.SetLock(dir, shelf.Lock{}) })
}
