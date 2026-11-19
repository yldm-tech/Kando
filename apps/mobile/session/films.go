package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"kando/apps/mobile/catalog"
	"kando/apps/mobile/store"
	"kando/packages/sync"
)

// What the phone has a computer do to a film of its library over protocol v2 (kando/packages/sync, films.go), as a film's options offer: translate it again, take its subtitles as a file to share, delete it from the library, and download the video of a film that keeps none. Each runs through the film's computer while the film is busy (FilmBusy); a job the computer answers joins its queue as the phone follows it, the action that worked is told through QueueEvents.Acted, and one that failed through Failed. A film the computer no longer has is read again with its shelf.

// filmKey stands for an action on film id among the busy jobs, apart from a job of the same ID, as a film's job has.
func filmKey(desktop, id string) string { return store.Key(desktop, "\x00film:"+id) }

// FilmBusy reports whether an action on film id of desktop is under way.
func (s *Session) FilmBusy(desktop, id string) bool { return s.remote.busy.Get()[filmKey(desktop, id)] }

// Listed is the film kept under key as its computer last listed it, false for one it does not list or has not yet.
func (s *Session) Listed(key string) (catalog.Entry, bool) {
	for _, c := range s.Computers() {
		for _, lib := range c.Listing.Libraries {
			for _, f := range lib.Films {
				if e := (catalog.Entry{Desktop: c.ID, Computer: c.Name, Library: lib.Library, Film: f}); e.Key() == key {
					return e, true
				}
			}
		}
	}
	return catalog.Entry{}, false
}

// VideoComing reports whether desktop is downloading the video of film id, one that keeps none: the job that downloads it waits, runs or is paused in its queue. Read it in a binding.
func (s *Session) VideoComing(desktop, id string) bool {
	for _, j := range s.Queue(desktop).Jobs {
		if j.ID != id || (j.State != sync.JobWaiting && j.State != sync.JobRunning && j.State != sync.JobPaused) {
			continue
		}
		for _, st := range j.Steps {
			if st.Name == sync.StepTranslate && st.State == sync.StepSkipped {
				return true
			}
		}
	}
	return false
}

// actOnFilm runs call on desktop for film id while it is busy, off the engine's goroutine: a job it answers joins desktop's queue, after is run on the engine's goroutine once it worked, and Acted is told op when it changed the film; an error is told through Failed, after which a computer that answered lists its films again.
func (s *Session) actOnFilm(desktop, id, op, title string, call func(ctx context.Context, c *sync.Client) (*sync.Job, error), after func()) {
	c := s.Client(desktop)
	key := filmKey(desktop, id)
	if c == nil || s.remote.busy.GetUntracked()[key] {
		return
	}
	s.setBusy(key, true)
	s.work.Go(func() {
		ctx, cancel := context.WithTimeout(s.remote.ctx, actFor)
		defer cancel()
		job, err := call(ctx, c)
		s.cx.Post(func() {
			s.setBusy(key, false)
			if err != nil {
				s.queueFailed(desktop, err)
				var e *sync.Error
				if errors.As(err, &e) && e.Code != sync.CodeReadOnly {
					s.RefreshDesktop(desktop)
				}
				return
			}
			if job != nil && job.ID != "" {
				s.upsertJobs(desktop, []sync.Job{*job})
			}
			if after != nil {
				after()
			}
			if s.remote.events.Acted != nil && sync.Mutates(op) {
				s.remote.events.Acted(desktop, op, title)
			}
		})
	})
}

// RetranslateFilm has desktop translate film f of its library again, into the language it translates into; the phone follows the job in its queue.
func (s *Session) RetranslateFilm(desktop string, f sync.Film) {
	s.actOnFilm(desktop, f.ID, sync.OpFilmRetranslate, f.Title, func(ctx context.Context, c *sync.Client) (*sync.Job, error) {
		job, err := c.RetranslateFilm(ctx, f.ID)
		return &job, err
	}, nil)
}

// DownloadVideoOnComputer has desktop download the video of film f, which keeps none there; the phone follows the job in its queue, and downloads the video in turn once the computer lists the film with it.
func (s *Session) DownloadVideoOnComputer(desktop string, f sync.Film) {
	s.actOnFilm(desktop, f.ID, sync.OpFilmVideo, f.Title, func(ctx context.Context, c *sync.Client) (*sync.Job, error) {
		job, err := c.DownloadFilmVideo(ctx, f.ID)
		return &job, err
	}, nil)
}

// DeleteFromComputer deletes film f from desktop's library, whose files go to the computer's Trash; the computer lists its films again after. What the phone keeps of the film stays, to be deleted on its own.
func (s *Session) DeleteFromComputer(desktop string, f sync.Film) {
	s.actOnFilm(desktop, f.ID, sync.OpFilmDelete, f.Title, func(ctx context.Context, c *sync.Client) (*sync.Job, error) {
		return nil, c.DeleteFilm(ctx, f.ID)
	}, func() { s.RefreshDesktop(desktop) })
}

// ExportSubtitles takes film f's subtitles from desktop as the file its export writes, and keeps it in the app's cache under the name the computer gives it, for the share sheet: done gets its path on the engine's goroutine, or why it could not, which Failed is told too.
func (s *Session) ExportSubtitles(desktop string, f sync.Film, done func(path string, err error)) {
	dir := filepath.Join(s.cx.CacheDir(), "subtitles")
	var path string
	s.actOnFilm(desktop, f.ID, sync.OpFilmExport, f.Title, func(ctx context.Context, c *sync.Client) (*sync.Job, error) {
		file, err := c.ExportFilm(ctx, f.ID)
		if err == nil {
			path, err = keepExport(dir, file)
		}
		if err != nil {
			s.cx.Post(func() { done("", err) })
		}
		return nil, err
	}, func() { done(path, nil) })
}

// keepExport writes file in dir, in place of what an export before left there, under its name made safe for a file name.
func keepExport(dir string, file sync.SubtitleFile) (string, error) {
	_ = os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r < ' ' {
			return '_'
		}
		return r
	}, filepath.Base(file.Name))
	if name == "" || name == "." || name == ".." {
		name = "subtitles.srt"
	}
	path := filepath.Join(dir, name)
	return path, os.WriteFile(path, file.Data, 0o600)
}
