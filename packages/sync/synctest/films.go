package synctest

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"kando/packages/sync"
)

// filmOps are what phones asked of the stand-in desktop's films (sync.FilmService): the films translated again, exported and deleted, and those whose video was asked for, by ID in the order asked; and whether it acts on its films at all (SetNoFilmOps).
type filmOps struct {
	retranslated, exported, deleted, videos []string
	none                                    bool
}

// SetNoFilmOps makes the desktop answer the films' calls unsupported while none is set, as a Kando from before them.
func (d *Desktop) SetNoFilmOps(none bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ops.none = none
}

// Retranslated are the films phones had the desktop translate again, by ID, in the order asked.
func (d *Desktop) Retranslated() []string { return d.opsOf(&d.ops.retranslated) }

// Exported are the films whose subtitles phones took as a file.
func (d *Desktop) Exported() []string { return d.opsOf(&d.ops.exported) }

// Deleted are the films phones deleted from the desktop's libraries.
func (d *Desktop) Deleted() []string { return d.opsOf(&d.ops.deleted) }

// VideosAsked are the films that keep no video whose video phones had the desktop download.
func (d *Desktop) VideosAsked() []string { return d.opsOf(&d.ops.videos) }

func (d *Desktop) opsOf(list *[]string) []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(*list)
}

// filmService is the stand-in's films as v2's calls act on them: translating one again and downloading the video of one that keeps none queue a waiting job under the film's ID, as the desktop's redo of a film does, exporting writes its lines as SubRip, and deleting takes it off its library.
type filmService struct{ d *Desktop }

func (s filmService) RetranslateFilm(_ context.Context, caller sync.Caller, ref sync.FilmRef) (sync.Job, error) {
	d := s.d
	d.mu.Lock()
	defer d.mu.Unlock()
	f, err := d.servedLocked(ref.ID)
	if err != nil {
		return sync.Job{}, err
	}
	d.ops.retranslated = append(d.ops.retranslated, f.ID)
	return d.redoLocked(f, caller, sync.StepTranslate), nil
}

func (s filmService) ExportFilm(_ context.Context, _ sync.Caller, ref sync.FilmRef) (sync.SubtitleFile, error) {
	d := s.d
	d.mu.Lock()
	defer d.mu.Unlock()
	f, err := d.servedLocked(ref.ID)
	if err != nil {
		return sync.SubtitleFile{}, err
	}
	d.ops.exported = append(d.ops.exported, f.ID)
	var b strings.Builder
	for i, c := range f.Cues {
		fmt.Fprintf(&b, "%d\n%s --> %s\n", i+1, srtTime(c.StartMS), srtTime(c.EndMS))
		if c.Translation != "" {
			b.WriteString(c.Translation + "\n")
		}
		b.WriteString(c.Original + "\n\n")
	}
	return sync.SubtitleFile{Name: f.Title + ".dual.srt", Data: []byte(b.String())}, nil
}

func (s filmService) DeleteFilm(_ context.Context, _ sync.Caller, ref sync.FilmRef) error {
	d := s.d
	d.mu.Lock()
	defer d.mu.Unlock()
	f, err := d.servedLocked(ref.ID)
	if err != nil {
		return err
	}
	d.ops.deleted = append(d.ops.deleted, f.ID)
	d.films = slices.DeleteFunc(d.films, func(x Film) bool { return x.ID == f.ID })
	for i := range d.libs {
		if d.libs[i].ID == f.Library && !f.SubtitlesOnly {
			d.libs[i].Films--
		}
	}
	return nil
}

func (s filmService) DownloadFilmVideo(_ context.Context, caller sync.Caller, ref sync.FilmRef) (sync.Job, error) {
	d := s.d
	d.mu.Lock()
	defer d.mu.Unlock()
	f, err := d.servedLocked(ref.ID)
	if err != nil {
		return sync.Job{}, err
	}
	if !f.SubtitlesOnly {
		return sync.Job{}, sync.Errorf(sync.CodeConflict, "film %s keeps its video", f.ID)
	}
	d.ops.videos = append(d.ops.videos, f.ID)
	return d.redoLocked(f, caller, sync.StepDownload), nil
}

// servedLocked is film id of a library the desktop lists. d.mu is held.
func (d *Desktop) servedLocked(id string) (Film, error) {
	if d.ops.none {
		return Film{}, sync.Errorf(sync.CodeUnsupported, "no film operations")
	}
	i := slices.IndexFunc(d.films, func(f Film) bool { return f.ID == id })
	if i < 0 || !slices.ContainsFunc(d.libs, func(l sync.Library) bool { return l.ID == d.films[i].Library }) {
		return Film{}, sync.Errorf(sync.CodeNotFound, "no film %q", id)
	}
	return d.films[i], nil
}

// redoLocked queues a waiting job under f's ID that runs only step, as the desktop's redo of a done film does, in place of one under that ID, and tells the phones waiting for events. d.mu is held.
func (d *Desktop) redoLocked(f Film, caller sync.Caller, step string) sync.Job {
	var steps []sync.Step
	for _, name := range []string{sync.StepDownload, sync.StepTranscribe, sync.StepTranslate} {
		st := sync.Step{Name: name, State: sync.StepSkipped}
		if name == step {
			st.State = sync.StepWaiting
		}
		steps = append(steps, st)
	}
	job := sync.Job{ID: f.ID, Title: f.Title, Library: f.Library, Device: caller.Name, State: sync.JobWaiting, Steps: steps}
	if i := slices.IndexFunc(d.queue.jobs, func(j sync.Job) bool { return j.ID == job.ID }); i >= 0 {
		d.queue.jobs[i] = job
	} else {
		d.queue.jobs = append(d.queue.jobs, job)
	}
	d.desk.Journal.PublishJobs(d.queue.jobs)
	return job
}

// srtTime writes ms as SubRip writes a time.
func srtTime(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	return fmt.Sprintf("%02d:%02d:%02d,%03d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60, ms%1000)
}
