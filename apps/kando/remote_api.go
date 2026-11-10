package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/apps/kando/remote"
	"kando/apps/kando/sidebar"
	"kando/apps/kando/sites/catalog"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/egoist/mygo"
)

// The control socket's methods (package remote), as the app answers them. remoteHandle runs on the goroutine of the request's connection: it reads and changes the app's state only inside remote.OnUI, which runs on the UI thread, and reads files, as subtitles, outside it.

// remoteHandle answers one request of the control socket.
func (a *app) remoteHandle(method string, params json.RawMessage) (any, error) {
	ctx, cancel := context.WithTimeout(a.remote.ctx, remote.Timeout)
	defer cancel()
	switch method {
	case remote.MethodStatus:
		return a.remoteStatus(ctx)
	case remote.MethodFilms:
		p, err := remote.Params[remote.FilmsParams](params)
		if err != nil {
			return nil, err
		}
		return a.remoteFilms(ctx, p)
	case remote.MethodFilm:
		p, err := remote.Params[remote.IDParams](params)
		if err != nil {
			return nil, err
		}
		return a.remoteFilm(ctx, p.ID)
	case remote.MethodSubtitles:
		p, err := remote.Params[remote.IDParams](params)
		if err != nil {
			return nil, err
		}
		return a.remoteSubtitles(ctx, p.ID)
	case remote.MethodJobs:
		return a.remoteJobs(ctx)
	case remote.MethodAdd:
		p, err := remote.Params[remote.AddParams](params)
		if err != nil {
			return nil, err
		}
		return a.remoteAdd(ctx, p)
	case remote.MethodCancel:
		p, err := remote.Params[remote.IDParams](params)
		if err != nil {
			return nil, err
		}
		return a.remoteCancel(ctx, p.ID)
	case remote.MethodRetry:
		p, err := remote.Params[remote.IDParams](params)
		if err != nil {
			return nil, err
		}
		return a.remoteRetry(ctx, p.ID)
	case remote.MethodRemove:
		p, err := remote.Params[remote.IDParams](params)
		if err != nil {
			return nil, err
		}
		return nil, a.remoteRemove(ctx, p.ID)
	case remote.MethodSites:
		p, err := remote.Params[remote.SitesParams](params)
		if err != nil {
			return nil, err
		}
		return remoteSites(p), nil
	case remote.MethodShow:
		p, err := remote.Params[remote.IDParams](params)
		if err != nil {
			return nil, err
		}
		return nil, a.remoteShow(ctx, p.ID)
	}
	return nil, remote.Errorf(remote.CodeUnknownMethod, "unknown method %q", method)
}

// remoteStatus is what the app is and whether a job can start: the sidebar's status line, the tools the pipeline runs, and the queue as the open library's 处理中 shows it (remoteQueue).
func (a *app) remoteStatus(ctx context.Context) (remote.Status, error) {
	return remote.OnUI(ctx, a.update, func() (remote.Status, error) {
		ready := a.readiness()
		running, waiting, _ := a.remoteQueue()
		s := remote.Status{
			Version:    appVersion,
			LibraryDir: a.snap().LibraryDir,
			Ready:      ready.State == sidebar.Ready,
			Readiness:  ready.Text(),
			Tools:      map[string]string{"yt-dlp": "", "ffmpeg": "", "whisper-cli": ""},
			Language:   a.prefs.language,
			Queued:     len(waiting),
		}
		if len(running) > 0 {
			s.Running = running[0].ID
		}
		if tools, ok := a.pipeline.(media.Tools); ok {
			s.Tools["yt-dlp"], s.Tools["ffmpeg"], s.Tools["whisper-cli"] = tools.YTDLP, tools.FFmpeg, tools.Whisper
			s.SpeechModel = tools.WhisperModel
			s.TranslationModel = tools.Translator.Model
		}
		return s, nil
	})
}

// remoteFilms is the open library's films as the window lists them, those from p.Source whose title holds p.Query.
func (a *app) remoteFilms(ctx context.Context, p remote.FilmsParams) ([]remote.Film, error) {
	items, err := remote.OnUI(ctx, a.update, func() ([]remote.Film, error) {
		all, err := a.remoteLibraryFilms()
		if err != nil {
			return nil, err
		}
		out := []remote.Film{}
		query := strings.ToLower(strings.TrimSpace(p.Query))
		for _, item := range all {
			if !remoteFromSource(item, p.Source) {
				continue
			}
			if query != "" && !strings.Contains(strings.ToLower(remoteFilmTitle(item)), query) {
				continue
			}
			out = append(out, remoteFilmOf(item))
		}
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	for i := range items {
		remoteFilmFiles(&items[i])
	}
	return items, nil
}

// remoteFilm is the open library's film with key id.
func (a *app) remoteFilm(ctx context.Context, id string) (remote.Film, error) {
	f, err := remote.OnUI(ctx, a.update, func() (remote.Film, error) {
		item, err := a.remoteFind(id)
		if err != nil {
			return remote.Film{}, err
		}
		return remoteFilmOf(item), nil
	})
	if err != nil {
		return remote.Film{}, err
	}
	remoteFilmFiles(&f)
	return f, nil
}

// remoteSubtitles is the film's subtitles, cue by cue, read from its files off the UI thread: each line of the speech from the original .srt, with its translation from the bilingual one.
func (a *app) remoteSubtitles(ctx context.Context, id string) ([]remote.Cue, error) {
	f, err := a.remoteFilm(ctx, id)
	if err != nil {
		return nil, err
	}
	if f.Subtitles == "" && f.OriginalSubtitles == "" {
		return nil, remote.Errorf(remote.CodeNotFound, "%q has no subtitles yet", f.Title)
	}
	original, err := remoteReadSRT(f.OriginalSubtitles)
	if err != nil {
		return nil, err
	}
	bilingual, err := remoteReadSRT(f.Subtitles)
	if err != nil {
		return nil, err
	}
	if original == nil && bilingual == nil {
		return nil, remote.Errorf(remote.CodeNotFound, "the subtitles of %q are no longer on disk", f.Title)
	}
	return remoteCues(original, bilingual), nil
}

// remoteJobs is the open library's jobs and those the queue runs for other libraries: the running one, the waiting ones in the order they will run, then the open library's finished ones, newest first (remoteQueue).
func (a *app) remoteJobs(ctx context.Context) ([]remote.Job, error) {
	jobs, err := remote.OnUI(ctx, a.update, func() ([]remote.Job, error) {
		if strings.TrimSpace(a.snap().LibraryDir) == "" {
			return nil, remote.Errorf(remote.CodeNotReady, "%s", locales.PrecheckNoLibrary())
		}
		running, waiting, finished := a.remoteQueue()
		out := []remote.Job{}
		for _, job := range slices.Concat(running, waiting, finished) {
			out = append(out, remoteJobOf(job))
		}
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	for i := range jobs {
		remoteJobFiles(&jobs[i])
	}
	return jobs, nil
}

// remoteAdd queues a job for each of p's sources, as 开始处理 and dropping videos on the window do: after the precheck the library's message bar would show, each source is added on its own, and one that cannot be is skipped with the reason. Local files become the file:// URL dropping them makes; a file that is missing or not a video is skipped before the app is asked.
func (a *app) remoteAdd(ctx context.Context, p remote.AddParams) (remote.AddResult, error) {
	if len(p.Sources) == 0 {
		return remote.AddResult{}, remote.Errorf(remote.CodeInvalid, "no sources")
	}
	type candidate struct{ given, source string }
	var candidates []candidate
	result := remote.AddResult{Jobs: []remote.Job{}}
	for _, given := range p.Sources {
		source, reason := remoteSource(given, p.Dir)
		if reason != "" {
			result.Skipped = append(result.Skipped, remote.AddFailed{Source: given, Reason: reason})
			continue
		}
		candidates = append(candidates, candidate{given, source})
	}
	if len(candidates) == 0 {
		return result, nil
	}
	added, err := remote.OnUI(ctx, a.update, func() (remote.AddResult, error) {
		res := remote.AddResult{Jobs: []remote.Job{}, Skipped: slices.Clone(result.Skipped)}
		sources := make([]string, len(candidates))
		for i, c := range candidates {
			sources[i] = c.source
		}
		if msg, _ := a.precheck(sources); msg != "" {
			return remote.AddResult{}, remote.Errorf(remote.CodeNotReady, "%s", msg)
		}
		var ids []string
		// A video the library already has waiting, running or done is skipped with why, as is a source naming the same video as one before it; a failed or cancelled job of the video is queued again and answered like a new one (queueTake).
		for _, c := range candidates {
			id, err := a.queueTake(c.source)
			if err != nil {
				res.Skipped = append(res.Skipped, remote.AddFailed{Source: c.given, Reason: sourceError(err)})
				continue
			}
			ids = append(ids, id)
		}
		for _, id := range ids {
			if job, ok := a.library.Find(id); ok {
				res.Jobs = append(res.Jobs, remoteJobOf(job))
			}
		}
		return res, nil
	})
	if err != nil {
		return remote.AddResult{}, err
	}
	for i := range added.Jobs {
		remoteJobFiles(&added.Jobs[i])
	}
	return added, nil
}

// remoteCancel cancels job id of the open library, or one the queue runs for another library, as the queue card's 取消 does, and answers it once it has stopped: a waiting job leaves the queue at once, and the running one when its pipeline returns.
func (a *app) remoteCancel(ctx context.Context, id string) (remote.Job, error) {
	job, err := remote.OnUI(ctx, a.update, func() (core.Job, error) {
		job, err := a.remoteFindJob(id)
		if err != nil {
			return core.Job{}, err
		}
		a.Cancel(id)
		return job, nil
	})
	if err != nil {
		return remote.Job{}, err
	}
	for {
		stopped, err := remote.OnUI(ctx, a.update, func() (remote.Job, error) {
			if a.queue.active == id {
				return remote.Job{}, nil
			}
			// A job of another library leaves 处理中 once it has stopped, so it is looked up on the shelf rather than with remoteFindJob.
			job, ok := a.library.Find(id)
			if !ok {
				return remote.Job{}, remote.Errorf(remote.CodeNotFound, "job %q was removed", id)
			}
			return remoteJobOf(job), nil
		})
		if err != nil {
			return remote.Job{}, err
		}
		if stopped.ID != "" {
			remoteJobFiles(&stopped)
			return stopped, nil
		}
		select {
		case <-ctx.Done():
			return remote.Job{}, fmt.Errorf("job %s did not stop: %w", job.ID, ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// remoteRetry queues job id of the open library again under the same ID, as the queue card's 重试 does for a failed or canceled job and 重新处理 for a done one, but without moving the window: the page and the card the library follows stay as they are. A job still running or waiting is refused; when it cannot start now, as the library's message bar would say after the precheck, or no language is set, it answers not_ready with that message. It answers the job as it is once queued.
func (a *app) remoteRetry(ctx context.Context, id string) (remote.Job, error) {
	job, err := remote.OnUI(ctx, a.update, func() (remote.Job, error) {
		job, err := a.remoteFindJob(id)
		if err != nil {
			return remote.Job{}, err
		}
		ready, err := a.queueRetryable(job)
		if errors.Is(err, errInQueue) {
			return remote.Job{}, remote.Errorf(remote.CodeInvalid, "job %s is still running or waiting in the queue; cancel it first, or wait until it ends", id)
		}
		if msg, _ := a.precheck([]string{job.Source}); msg != "" {
			return remote.Job{}, remote.Errorf(remote.CodeNotReady, "%s", msg)
		}
		if err != nil {
			return remote.Job{}, remote.Errorf(remote.CodeNotReady, "%s", err.Error())
		}
		// As 重新处理 does, the hero forgets the first line it read from the subtitles the job will write again.
		delete(a.lib.quotes, job.SubtitlePath)
		a.queueAdd(ready)
		queued, err := a.remoteFindJob(id)
		if err != nil {
			return remote.Job{}, err
		}
		return remoteJobOf(queued), nil
	})
	if err != nil {
		return remote.Job{}, err
	}
	remoteJobFiles(&job)
	return job, nil
}

// remoteRemove takes job id off the open library's shelf as the queue card's 移除 does: a running job is stopped, the film open in the player closes when it is this one, and its files stay on disk.
func (a *app) remoteRemove(ctx context.Context, id string) error {
	_, err := remote.OnUI(ctx, a.update, func() (struct{}, error) {
		if _, err := a.remoteFindJob(id); err != nil {
			return struct{}{}, err
		}
		a.remove(id)
		return struct{}{}, nil
	})
	return err
}

// remoteSites is the websites Kando downloads from that p.Query finds, as the sites sheet lists them, at most p.Limit of them when it is set.
func remoteSites(p remote.SitesParams) []remote.Site {
	found := catalog.Load().Filter(p.Query)
	if p.Limit > 0 && len(found) > p.Limit {
		found = found[:p.Limit]
	}
	out := make([]remote.Site, len(found))
	for i, s := range found {
		out[i] = remote.Site{Name: s.Name, Domain: s.Domain}
	}
	return out
}

// remoteShow brings the window to the front on the library, or on the film with key id: a done film opens as a click on its poster opens it, and one still in the queue shows the library, where its card is. After the main window was closed while the app runs on, as with the settings window still open, there is nothing to bring forward, as MyGo does not open a closed window again, so it answers not_ready and changes nothing.
func (a *app) remoteShow(ctx context.Context, id string) error {
	_, err := remote.OnUI(ctx, a.update, func() (struct{}, error) {
		if a.win != nil && a.win.IsDestroyed() {
			return struct{}{}, remote.Errorf(remote.CodeNotReady, "the Kando window was closed; quit Kando and open it again to show it")
		}
		if id == "" {
			a.Select(pageFilms)
		} else {
			item, err := a.remoteFind(id)
			if err != nil {
				return struct{}{}, err
			}
			switch {
			case films.InQueue(item) || item.VideoPath == "":
				a.Select(pageFilms)
			case a.page != pageFilms || a.openKey != item.Key:
				a.Select(pageFilms)
				a.Open(item)
			}
		}
		a.remoteRaise()
		return struct{}{}, nil
	})
	return err
}

// remoteRaise brings the main window to the front, restored when it was minimized, as app.show does and as a second launch of the app does (main's OnSecondInstance). It does nothing before the window is made or after it was closed. UI thread only.
func (a *app) remoteRaise() {
	if a.win == nil || a.win.IsDestroyed() {
		return
	}
	if a.win.IsMinimized() {
		a.win.Restore()
	}
	a.win.Show()
	mygo.App.Focus()
}

// remoteLibraryFilms is the open library's films, or not_ready when no library is open. UI thread only.
func (a *app) remoteLibraryFilms() ([]films.Film, error) {
	if strings.TrimSpace(a.snap().LibraryDir) == "" {
		return nil, remote.Errorf(remote.CodeNotReady, "%s", locales.PrecheckNoLibrary())
	}
	return a.films(), nil
}

// remoteFind is the open library's film with key id, the ID the socket names films by. UI thread only.
func (a *app) remoteFind(id string) (films.Film, error) {
	all, err := a.remoteLibraryFilms()
	if err != nil {
		return films.Film{}, err
	}
	item, ok := films.Find(all, id)
	if id == "" || !ok {
		return films.Film{}, remote.Errorf(remote.CodeNotFound, "no film %q in the open library", id)
	}
	return item, nil
}

// remoteFindJob is job id when it is on the open library's shelf, or one the queue runs or will run for another library (queueElsewhere), as 处理中 shows it. UI thread only.
func (a *app) remoteFindJob(id string) (core.Job, error) {
	root := a.snap().LibraryDir
	if strings.TrimSpace(root) == "" {
		return core.Job{}, remote.Errorf(remote.CodeNotReady, "%s", locales.PrecheckNoLibrary())
	}
	job, ok := a.library.Find(id)
	if id == "" || !ok || (!films.InLibrary(root, job) && !a.queueElsewhere(job)) {
		return core.Job{}, remote.Errorf(remote.CodeNotFound, "no job %q in the open library", id)
	}
	return job, nil
}

// remoteQueue is the jobs in the order jobs.list gives them: the running one, the waiting ones as the queue will run them (the queue's order, then those read back as queued that it has not taken in yet, oldest first, as pump takes them), and the rest of the open library's as the shelf keeps them, newest first. The running and waiting ones include those the queue runs for another library (queueElsewhere), as 处理中 shows them; the finished ones include those the queue ran to the end since launch for another library (jobQueue.ran), so a client polling jobs.list for a job sees how it ended after another library is opened. UI thread only.
func (a *app) remoteQueue() (running, waiting, finished []core.Job) {
	root := a.snap().LibraryDir
	seen := map[string]bool{}
	// take reports whether job goes in the list: it is on the open library's shelf, or elsewhere says the list keeps it although it saves into another library, and it is not in the list yet.
	take := func(job core.Job, elsewhere bool) bool {
		if seen[job.ID] || !(films.InLibrary(root, job) || elsewhere) {
			return false
		}
		seen[job.ID] = true
		return true
	}
	if job, ok := a.library.Find(a.queue.active); a.queue.active != "" && ok && take(job, a.queueElsewhere(job)) {
		running = append(running, job)
	}
	for _, id := range a.queue.order {
		job, ok := a.library.Find(id)
		if ok && job.Stage == core.StageQueued && take(job, a.queueElsewhere(job)) {
			waiting = append(waiting, job)
		}
	}
	for i := len(a.library.Jobs) - 1; i >= 0; i-- {
		if job := a.library.Jobs[i]; job.Stage == core.StageQueued && take(job, a.queueElsewhere(job)) {
			waiting = append(waiting, job)
		}
	}
	for _, job := range a.library.Jobs {
		if take(job, a.queue.ran[job.ID]) {
			finished = append(finished, job)
		}
	}
	return running, waiting, finished
}

// remoteFromSource reports whether item comes from source as FilmsParams names it: "" or sidebar.SourceAll for every film, a sidebar source (youtube, bilibili, web, local), or a site, by its domain, its host or the name the library shows for it, ignoring case.
func remoteFromSource(item films.Film, source string) bool {
	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" || source == sidebar.SourceAll {
		return true
	}
	if films.SourceOf(item) == source {
		return true
	}
	link, ok := core.ParseLink(item.Source)
	if !ok || link.Platform == core.PlatformLocal {
		return false
	}
	return link.Site == source || link.Host == source || strings.ToLower(films.SourceLabel(item)) == source
}

// remoteFilmTitle is the film's whole name, as the film page's title bar shows it.
func remoteFilmTitle(item films.Film) string {
	if item.Name != "" {
		return item.Name
	}
	return item.Title
}

// remoteFilmOf is item as the socket answers it, without what only its files tell (remoteFilmFiles). Its status is the line its card shows: the stage, or for a failed job the reason the card gives. UI thread only, as it says the status in the interface's language.
func remoteFilmOf(item films.Film) remote.Film {
	f := remote.Film{
		ID:        item.Key,
		Title:     remoteFilmTitle(item),
		Source:    films.SourceOf(item),
		Stage:     string(item.Stage),
		Progress:  remoteProgress(item.Stage, item.Progress, item.JobID != ""),
		JobID:     item.JobID,
		Language:  item.Language,
		VideoPath: item.VideoPath,
		Subtitles: item.SubtitlePath,
	}
	if !films.Local(item) {
		f.Link = item.Source
	}
	if item.JobID != "" {
		f.Status, _ = failureHint(item)
	}
	return f
}

// remoteFilmFiles fills in what f's files on disk tell: the original subtitles beside its video. Its bilingual subtitles are the film's own (remoteFilmOf), which for a video no job has processed are the ones the library scan found beside it (media.Video.Subtitles), so the window and the socket show the same.
func remoteFilmFiles(f *remote.Film) {
	if f.VideoPath == "" {
		return
	}
	if original := strings.TrimSuffix(f.VideoPath, filepath.Ext(f.VideoPath)) + ".srt"; fileExists(original) {
		f.OriginalSubtitles = original
	}
}

// remoteJobOf is job as the socket answers it, without its film (remoteJobFiles). UI thread only, as it says the status in the interface's language.
func remoteJobOf(job core.Job) remote.Job {
	name := jobName(job)
	status, _ := failureHint(films.FromJob(job, name, statusText(job)))
	j := remote.Job{
		ID:        job.ID,
		Source:    job.Source,
		Title:     name,
		Stage:     string(job.Stage),
		Progress:  remoteProgress(job.Stage, job.Progress, true),
		Status:    status,
		Canceled:  errors.Is(job.Err, context.Canceled),
		Language:  job.Language,
		VideoPath: job.VideoPath,
		Subtitles: job.SubtitlePath,
	}
	if job.Stage == core.StageFailed && job.Err != nil && !j.Canceled {
		j.Error = job.Err.Error()
	}
	return j
}

// remoteJobFiles names the film j makes once its video is on disk: the job's ID, the key the library gives a job's film.
func remoteJobFiles(j *remote.Job) {
	if j.VideoPath != "" && fileExists(j.VideoPath) {
		j.FilmID = j.ID
	}
}

// remoteProgress is how far a stage has gone, from 0 to 1, or -1 when its tool has not said: a done job has gone all the way, a waiting one nowhere yet, and a video no job has processed (hasJob false) has no stage to go through.
func remoteProgress(stage core.Stage, p core.Progress, hasJob bool) float64 {
	switch {
	case !hasJob:
		return -1
	case stage == core.StageComplete:
		return 1
	case stage == core.StageQueued:
		return 0
	case p.Known:
		return p.Fraction
	}
	return -1
}

// remoteSource is the job source for what the client gave: a link as it is, or a local video as the file:// URL dropping it on the window makes, with a relative path taken from dir. reason says why it cannot be added, in the library's words where it has them; it is "" for a source to add.
func remoteSource(given, dir string) (source, reason string) {
	s := strings.TrimSpace(given)
	if s == "" {
		return "", locales.PrecheckEmpty()
	}
	path, local := remoteLocalPath(s, dir)
	if !local {
		link, ok := core.ParseLink(s)
		if !ok {
			return "", locales.PrecheckNotLink()
		}
		// A channel or a list is skipped on its own here, rather than refusing the whole request in precheck, as a link that is not one is.
		if !link.OneVideo() {
			return "", precheckWhole(link)
		}
		return s, ""
	}
	if path == "" {
		return "", fmt.Sprintf("%q is a relative path, and the request names no directory to take it from", s)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err.Error()
	}
	if info.IsDir() || !media.IsVideoFile(path) {
		return "", locales.PrecheckNoVideo()
	}
	return core.FileURL(path), ""
}

// remoteLocalPath reports whether s names a local file rather than a link, and its absolute path: a file:// URL, a path that starts at the root, the home folder (~/) or the current folder (./, ../), or anything else that is a video's file name or names a file in dir. path is "" for a relative path without dir.
func remoteLocalPath(s, dir string) (path string, local bool) {
	if strings.HasPrefix(strings.ToLower(s), "file:") {
		u, err := url.Parse(s)
		if err != nil {
			return "", false
		}
		path, local := core.FilePath(u)
		if !local {
			return "", false
		}
		return filepath.Clean(path), true
	}
	if rest, ok := strings.CutPrefix(s, "~"+string(filepath.Separator)); ok {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, rest), true
		}
	}
	if filepath.IsAbs(s) {
		return filepath.Clean(s), true
	}
	if _, ok := core.ParseLink(s); ok || strings.Contains(s, "://") {
		return "", false
	}
	relative := strings.HasPrefix(s, "."+string(filepath.Separator)) || strings.HasPrefix(s, ".."+string(filepath.Separator)) || media.IsVideoFile(s)
	if dir == "" {
		return "", relative
	}
	path = filepath.Join(dir, s)
	if !relative {
		if _, err := os.Stat(path); err != nil {
			return "", false
		}
	}
	return path, true
}

// remoteReadSRT is the cues of the .srt at path: none for an empty path or a file that is not there, and a failed error for one that cannot be read.
func remoteReadSRT(path string) ([]core.Segment, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, remote.Errorf(remote.CodeFailed, "read %s: %v", path, err)
	}
	cues, err := media.ParseSRT(data)
	if err != nil {
		return nil, remote.Errorf(remote.CodeFailed, "read %s: %v", path, err)
	}
	if cues == nil {
		cues = []core.Segment{}
	}
	return cues, nil
}

// remoteCues pairs the original subtitles with the bilingual ones the pipeline exports beside them, whose cues are the translation over the original line (media's exportSubtitles), or the original alone where the translation left a line out. When both files have as many cues, shown at the same times in the same order, as the pipeline writes them, each original cue takes the bilingual cue at its index; otherwise, as after a file was edited by hand, each takes the first bilingual cue shown at the same time that no earlier cue took, so two lines shown at the same time keep their own translations. Without the original file, each bilingual cue is split as the film page splits it (film.SplitCue).
func remoteCues(original, bilingual []core.Segment) []remote.Cue {
	out := []remote.Cue{}
	if original == nil {
		for _, s := range bilingual {
			translation, text := film.SplitCue(s.Text)
			out = append(out, remote.Cue{StartMS: s.Start.Milliseconds(), EndMS: s.End.Milliseconds(), Text: text, Translation: translation})
		}
		return out
	}
	aligned := len(original) == len(bilingual)
	for i := 0; aligned && i < len(original); i++ {
		aligned = original[i].Start == bilingual[i].Start && original[i].End == bilingual[i].End
	}
	type span struct{ start, end time.Duration }
	var waiting map[span][]string
	if !aligned {
		waiting = make(map[span][]string, len(bilingual))
		for _, s := range bilingual {
			key := span{s.Start, s.End}
			waiting[key] = append(waiting[key], s.Text)
		}
	}
	for i, s := range original {
		c := remote.Cue{StartMS: s.Start.Milliseconds(), EndMS: s.End.Milliseconds(), Text: s.Text}
		var text string
		var ok bool
		if aligned {
			text, ok = bilingual[i].Text, true
		} else if queue := waiting[span{s.Start, s.End}]; len(queue) > 0 {
			text, ok = queue[0], true
			waiting[span{s.Start, s.End}] = queue[1:]
		}
		if ok && text != s.Text {
			if translation, cut := strings.CutSuffix(text, "\n"+s.Text); cut {
				c.Translation = strings.TrimSpace(translation)
			} else {
				c.Translation, _ = film.SplitCue(text)
			}
		}
		out = append(out, c)
	}
	return out
}
