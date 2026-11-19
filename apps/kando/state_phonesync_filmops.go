package main

import (
	"context"
	"errors"
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/phonesync"
	"kando/apps/kando/remote"
	"kando/apps/kando/settings"
	"kando/apps/kando/subexport"
	"kando/packages/core"
	psync "kando/packages/sync"
)

// phoneFilmOps is what a paired phone does to a film over protocol v2 (psync.FilmService), as the options of a film in its Library offer, through what the window does to a film: 重新翻译为 (queueRetranslatable), 导出字幕 (the export dialog's choice, subexport), 从资料库移除 (libraryTrashIn) and a film's 下载视频 (queueVideo). A film is found as Phone Sync lists it, in any library the app knows that is not locked (phoneFilmFind), and acted on on the engine's goroutine, where a library locked meanwhile is not found either. The page's list of what the phones did says what each phone changed.
type phoneFilmOps struct{ a *app }

// phoneFilmFound is a film a phone named, as the app files it, with its library and its files as Phone Sync serves them.
type phoneFilmFound struct {
	item films.Film
	lib  phonesync.Library
	film phonesync.Film
}

// phoneFilmFind is the film id names among those Phone Sync serves (phoneSource.Films): read off the engine's goroutine as the films' list is. A film of a locked library, or none the libraries list, is not found.
func (a *app) phoneFilmFind(ctx context.Context, id string) (phoneFilmFound, error) {
	src := phoneSource{a}
	libs, err := src.Libraries(ctx)
	if err != nil {
		return phoneFilmFound{}, err
	}
	for _, lib := range libs {
		items, shelf, err := src.items(ctx, lib)
		if err != nil {
			continue
		}
		for _, item := range items {
			if phoneFilmID(item) == id {
				return phoneFilmFound{item: item, lib: lib, film: phoneFilmOf(item, shelf)}, nil
			}
		}
	}
	return phoneFilmFound{}, psync.Errorf(psync.CodeNotFound, "no film %q", id)
}

// phoneFilmJob is the done film f's job on the shelf, on the engine's goroutine: not found once its library is locked or the film left the shelf, and a conflict for a film Kando did not make the subtitles of, or whose job is waiting or running again. what names what the phone asked for, in the error.
func (a *app) phoneFilmJob(f phoneFilmFound, what string) (core.Job, error) {
	if a.libraryLocked(f.lib.Dir) {
		return core.Job{}, psync.Errorf(psync.CodeNotFound, "no film %q", f.film.ID)
	}
	job, ok := a.library.Find(f.item.JobID)
	switch {
	case f.item.JobID == "" || !ok:
		return core.Job{}, psync.Errorf(psync.CodeConflict, "Kando did not make the subtitles of %q, so it cannot %s", f.film.Title, what)
	case job.ID == a.queue.active || job.Stage != core.StageComplete:
		return core.Job{}, psync.Errorf(psync.CodeConflict, "%q is waiting or running in the queue; wait until it ends", f.film.Title)
	}
	return job, nil
}

// RetranslateFilm translates every line of the film again into the language the settings name now, as 重新翻译为 does with it, behind the jobs waiting, and answers the job, which the phone follows in the queue. Without an AI service to translate with, or a language to translate into, it answers not_ready with what is missing.
func (p phoneFilmOps) RetranslateFilm(ctx context.Context, caller psync.Caller, ref psync.FilmRef) (psync.Job, error) {
	a := p.a
	f, err := a.phoneFilmFind(ctx, ref.ID)
	if err != nil {
		return psync.Job{}, err
	}
	return remote.OnUI(ctx, a.update, func() (psync.Job, error) {
		job, err := a.phoneFilmJob(f, "translate it again")
		if err != nil {
			return psync.Job{}, err
		}
		m := remoteMessages()
		if msg := a.remoteNoTranslator(m); msg != "" {
			return psync.Job{}, psync.Errorf(psync.CodeNotReady, "%s", msg)
		}
		ready, err := a.queueRetranslatable(job, core.RedoTranslation, "")
		if err != nil {
			return psync.Job{}, psync.Errorf(psync.CodeNotReady, "%s", m.QueueNoLanguage())
		}
		// As 重新翻译为 does, the hero forgets the first line it read from the subtitles the job will write again.
		delete(a.lib.quotes, job.SubtitlePath)
		a.queueAdd(ready)
		a.phoneLog(caller.Name, settings.ActivityRetranslated, a.libraryJobName(job), 0, 0)
		queued, _ := a.library.Find(job.ID)
		return a.phoneJobOf(queued), nil
	})
}

// DownloadFilmVideo downloads the video of the film, one that keeps none, as its 下载视频 does (queueVideo), and answers the job, which the phone follows in the queue. While offline mode is on the job waits until it is off, as it does when asked on the desktop.
func (p phoneFilmOps) DownloadFilmVideo(ctx context.Context, caller psync.Caller, ref psync.FilmRef) (psync.Job, error) {
	a := p.a
	f, err := a.phoneFilmFind(ctx, ref.ID)
	if err != nil {
		return psync.Job{}, err
	}
	return remote.OnUI(ctx, a.update, func() (psync.Job, error) {
		job, err := a.phoneFilmJob(f, "download its video")
		if err != nil {
			return psync.Job{}, err
		}
		if !job.SubtitlesOnly {
			return psync.Job{}, psync.Errorf(psync.CodeConflict, "%q keeps its video already", f.film.Title)
		}
		a.queueVideo(job.ID)
		a.phoneLog(caller.Name, settings.ActivityVideo, a.libraryJobName(job), 0, 0)
		queued, _ := a.library.Find(job.ID)
		return a.phoneJobOf(queued), nil
	})
}

// DeleteFilm takes the film off its library as 从资料库移除 does (libraryTrashIn): its folder, or its video and subtitles, go to the Trash, and the message bar says so. A film whose files could not go stays, and the phone is told why.
func (p phoneFilmOps) DeleteFilm(ctx context.Context, caller psync.Caller, ref psync.FilmRef) error {
	a := p.a
	f, err := a.phoneFilmFind(ctx, ref.ID)
	if err != nil {
		return err
	}
	_, err = remote.OnUI(ctx, a.update, func() (struct{}, error) {
		if a.libraryLocked(f.lib.Dir) {
			return struct{}{}, psync.Errorf(psync.CodeNotFound, "no film %q", f.film.ID)
		}
		name := films.Name(f.item)
		gone, _, reason := a.libraryTrashIn(f.lib.Dir, []films.Film{f.item}, remoteMessages())
		if len(gone) == 0 {
			return struct{}{}, psync.Errorf(psync.CodeNotReady, "%s", reason)
		}
		a.libraryForget(gone)
		a.tell(a.messages().LibraryTrashed(name))
		a.phoneLog(caller.Name, settings.ActivityDeleted, name, 0, 0)
		return struct{}{}, nil
	})
	return err
}

// phoneExport is what the export of a film takes from the app: what the export dialog last chose, the film's subtitle delay, and what stands in for a missing translation.
type phoneExport struct {
	choice      subexport.Choice
	delay       time.Duration
	placeholder string
}

// ExportFilm is the film's subtitles as the export dialog writes them (subexport), in the format and with the lines it last chose, the user's corrections laid over them and the lines marked as no speech left out, named as 导出到影片文件夹 names the file. The lines are read off the engine's goroutine, as a phone reads them (phoneSource.Cues).
func (p phoneFilmOps) ExportFilm(ctx context.Context, _ psync.Caller, ref psync.FilmRef) (psync.SubtitleFile, error) {
	a := p.a
	f, err := a.phoneFilmFind(ctx, ref.ID)
	if err != nil {
		return psync.SubtitleFile{}, err
	}
	how, err := remote.OnUI(ctx, a.update, func() (phoneExport, error) {
		if a.libraryLocked(f.lib.Dir) {
			return phoneExport{}, psync.Errorf(psync.CodeNotFound, "no film %q", f.film.ID)
		}
		return phoneExport{choice: a.export.choice, delay: a.player.delays[f.item.VideoPath], placeholder: a.messages().SubexportPlaceholder()}, nil
	})
	if err != nil {
		return psync.SubtitleFile{}, err
	}
	cues, err := phoneSource{a}.Cues(ctx, f.film)
	if errors.Is(err, phonesync.ErrNotFound) {
		return psync.SubtitleFile{}, psync.Errorf(psync.CodeNotFound, "film %q has no subtitles", f.film.ID)
	}
	if err != nil {
		return psync.SubtitleFile{}, err
	}
	lines := make([]core.Line, len(cues))
	for i, c := range cues {
		lines[i] = core.Line{Start: time.Duration(c.StartMS) * time.Millisecond, End: time.Duration(c.EndMS) * time.Millisecond, Original: c.Original, Translation: c.Translation}
	}
	data := how.choice.Export(how.delay, how.placeholder).Write(core.Correct(lines, nil))
	if len(data) > psync.MaxSubtitleFile {
		return psync.SubtitleFile{}, psync.Errorf(psync.CodeInvalid, "the subtitles of %q are too long to send", f.film.Title)
	}
	return psync.SubtitleFile{Name: how.choice.FileName(f.item.VideoPath), Data: data}, nil
}
