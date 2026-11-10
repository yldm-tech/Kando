package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/locales"
	"kando/apps/kando/remote"
	"kando/packages/core"
	"kando/packages/media"
)

// remoteServe opens a's control socket at a short path of its own (macOS caps a socket's path at 104 bytes, which t.TempDir can pass), and closes it when the test ends.
func remoteServe(t *testing.T, a *app) {
	t.Helper()
	dir, err := os.MkdirTemp("", "k")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("KANDO_SOCKET", filepath.Join(dir, "k.sock"))
	if err := a.remoteListen(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.remoteClose)
}

// remoteCall calls method over the socket from another goroutine, as the command line would, while the test applies the app's updates on its own goroutine, as the window's UI thread would, until the answer comes.
func remoteCall[T any](t *testing.T, updates <-chan func(), method string, params any) (T, error) {
	t.Helper()
	var out T
	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		c, err := remote.Dial(ctx, false)
		if err != nil {
			done <- err
			return
		}
		defer c.Close()
		done <- c.Call(ctx, method, params, &out)
	}()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case err := <-done:
			return out, err
		case fn := <-updates:
			fn()
		case <-timeout:
			t.Fatalf("%s did not answer", method)
		}
	}
}

func remoteWantCode(t *testing.T, err error, code string) {
	t.Helper()
	if !remote.IsCode(err, code) {
		t.Fatalf("error %v, want %s", err, code)
	}
}

func remoteWriteFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

// remoteLibrary is a library with a finished YouTube film and both its subtitle files, a video found in the folder with only a bilingual file beside it, and a local video waiting in the queue.
func remoteLibrary(t *testing.T, a *app) (dir string) {
	t.Helper()
	dir = t.TempDir()
	talk := filepath.Join(dir, "Rust Talk", "talk.mp4")
	remoteWriteFile(t, talk, "video")
	remoteWriteFile(t, filepath.Join(dir, "Rust Talk", "talk.srt"), "1\n00:00:01,000 --> 00:00:02,500\nHello there\n\n2\n00:00:03,000 --> 00:00:04,000\nGeneral Kenobi\n\n")
	remoteWriteFile(t, filepath.Join(dir, "Rust Talk", "talk.bilingual.srt"), "1\n00:00:01,000 --> 00:00:02,500\n你好\nHello there\n\n2\n00:00:03,000 --> 00:00:04,000\nGeneral Kenobi\n\n")
	interview := filepath.Join(dir, "Interview", "interview.mp4")
	remoteWriteFile(t, interview, "video")
	remoteWriteFile(t, filepath.Join(dir, "Interview", "interview.bilingual.srt"), "1\n00:00:00,500 --> 00:00:01,000\n早上好\nGood morning\n\n")
	// The videos are as the library scan lists them, which finds the interview's bilingual file beside it.
	videos, err := media.ListVideos(dir)
	if err != nil {
		t.Fatal(err)
	}
	a.edit(func(s *session) {
		s.LibraryDir = dir
		s.Videos = videos
	})
	a.library.Jobs = []core.Job{
		{ID: "clip", Source: "file:///elsewhere/clip.mp4", Language: "简体中文", Stage: core.StageQueued, LibraryDir: dir},
		{ID: "yt", Source: "https://www.youtube.com/watch?v=abcdefg", Title: "Rust Talk", Language: "简体中文", Stage: core.StageComplete, VideoPath: talk, SubtitlePath: filepath.Join(dir, "Rust Talk", "talk.bilingual.srt"), LibraryDir: dir, CompletedAt: time.Now()},
	}
	return dir
}

func TestRemoteStatusSaysWhatTheAppIs(t *testing.T) {
	a, _, updates := newTestApp(&fakePipeline{})
	remoteServe(t, a)
	a.checkReadiness(false)
	got, err := remoteCall[remote.Status](t, updates, remote.MethodStatus, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != appVersion || got.LibraryDir != "/videos" || !got.Ready || got.Readiness != locales.ReadinessReady() || got.Language != "简体中文" || got.Running != "" || got.Queued != 0 {
		t.Fatalf("status %+v", got)
	}

	tools := media.Tools{YTDLP: "/bin/yt-dlp", FFmpeg: "/bin/ffmpeg", WhisperModel: "/models/ggml-base.bin", Translator: media.ChatTranslator{Model: "gpt-x"}}
	b, _, updates := newTestApp(tools)
	remoteServe(t, b)
	b.checkReadiness(false)
	got, err = remoteCall[remote.Status](t, updates, remote.MethodStatus, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Tools["yt-dlp"] != "/bin/yt-dlp" || got.Tools["ffmpeg"] != "/bin/ffmpeg" || got.Tools["whisper-cli"] != "" || got.SpeechModel != "/models/ggml-base.bin" || got.TranslationModel != "gpt-x" {
		t.Fatalf("status %+v", got)
	}
	if got.Ready || !strings.Contains(got.Readiness, "whisper-cli") {
		t.Fatalf("status without whisper-cli %+v", got)
	}
}

func TestRemoteListsAndFindsTheLibrarysFilms(t *testing.T) {
	a, _, updates := newTestApp(&fakePipeline{})
	remoteServe(t, a)
	dir := remoteLibrary(t, a)
	interview := filepath.Join(dir, "Interview", "interview.mp4")

	all, err := remoteCall[[]remote.Film](t, updates, remote.MethodFilms, nil)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, len(all))
	for i, f := range all {
		ids[i] = f.ID
	}
	if strings.Join(ids, ",") != "clip,yt,"+interview {
		t.Fatalf("films %v", ids)
	}
	yt := all[1]
	if yt.Title != "Rust Talk" || yt.Source != "youtube" || yt.Link != "https://www.youtube.com/watch?v=abcdefg" || yt.Stage != "complete" || yt.Progress != 1 || yt.JobID != "yt" || yt.Language != "简体中文" || yt.Subtitles != filepath.Join(dir, "Rust Talk", "talk.bilingual.srt") || yt.OriginalSubtitles != filepath.Join(dir, "Rust Talk", "talk.srt") || yt.Status == "" {
		t.Fatalf("youtube film %+v", yt)
	}
	found := all[2]
	if found.Title != "interview" || found.Source != "local" || found.Link != "" || found.Stage != "" || found.Progress != -1 || found.JobID != "" || found.Status != "" || found.Subtitles != filepath.Join(dir, "Interview", "interview.bilingual.srt") || found.OriginalSubtitles != "" {
		t.Fatalf("found film %+v", found)
	}
	if clip := all[0]; clip.Source != "local" || clip.Link != "" || clip.Stage != "queued" || clip.Progress != 0 || clip.Status != locales.QueueStatusQueued() {
		t.Fatalf("queued film %+v", clip)
	}

	for _, tc := range []struct {
		params remote.FilmsParams
		want   int
	}{
		{remote.FilmsParams{Source: "youtube"}, 1},
		{remote.FilmsParams{Source: "YouTube.com"}, 1},
		{remote.FilmsParams{Source: "local"}, 2},
		{remote.FilmsParams{Source: "all"}, 3},
		{remote.FilmsParams{Source: "bilibili"}, 0},
		{remote.FilmsParams{Query: "rust"}, 1},
		{remote.FilmsParams{Source: "local", Query: "INTER"}, 1},
	} {
		got, err := remoteCall[[]remote.Film](t, updates, remote.MethodFilms, tc.params)
		if err != nil {
			t.Fatal(err)
		}
		if got == nil || len(got) != tc.want {
			t.Fatalf("%+v found %+v, want %d films", tc.params, got, tc.want)
		}
	}

	got, err := remoteCall[remote.Film](t, updates, remote.MethodFilm, remote.IDParams{ID: interview})
	if err != nil || got.ID != interview || got.Title != "interview" {
		t.Fatalf("films.get %+v, %v", got, err)
	}
	_, err = remoteCall[remote.Film](t, updates, remote.MethodFilm, remote.IDParams{ID: "nope"})
	remoteWantCode(t, err, remote.CodeNotFound)

	a.edit(func(s *session) { s.LibraryDir = "" })
	_, err = remoteCall[[]remote.Film](t, updates, remote.MethodFilms, nil)
	remoteWantCode(t, err, remote.CodeNotReady)
}

func TestRemoteReadsAFilmsSubtitles(t *testing.T) {
	a, _, updates := newTestApp(&fakePipeline{})
	remoteServe(t, a)
	dir := remoteLibrary(t, a)

	cues, err := remoteCall[[]remote.Cue](t, updates, remote.MethodSubtitles, remote.IDParams{ID: "yt"})
	if err != nil {
		t.Fatal(err)
	}
	want := []remote.Cue{{StartMS: 1000, EndMS: 2500, Text: "Hello there", Translation: "你好"}, {StartMS: 3000, EndMS: 4000, Text: "General Kenobi"}}
	if len(cues) != len(want) || cues[0] != want[0] || cues[1] != want[1] {
		t.Fatalf("cues %+v", cues)
	}

	cues, err = remoteCall[[]remote.Cue](t, updates, remote.MethodSubtitles, remote.IDParams{ID: filepath.Join(dir, "Interview", "interview.mp4")})
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 1 || cues[0] != (remote.Cue{StartMS: 500, EndMS: 1000, Text: "Good morning", Translation: "早上好"}) {
		t.Fatalf("cues from the bilingual file alone %+v", cues)
	}

	_, err = remoteCall[[]remote.Cue](t, updates, remote.MethodSubtitles, remote.IDParams{ID: "clip"})
	remoteWantCode(t, err, remote.CodeNotFound)
	_, err = remoteCall[[]remote.Cue](t, updates, remote.MethodSubtitles, remote.IDParams{ID: "nope"})
	remoteWantCode(t, err, remote.CodeNotFound)
}

func TestRemoteAddsListsAndCancelsJobs(t *testing.T) {
	a, _, updates := newTestApp(&fakePipeline{block: true})
	remoteServe(t, a)
	dir := t.TempDir()
	done := filepath.Join(dir, "Old", "old.mp4")
	remoteWriteFile(t, done, "video")
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.library.Jobs = []core.Job{{ID: "old", Source: "https://example.com/old", Stage: core.StageComplete, VideoPath: done, LibraryDir: dir}}
	from := t.TempDir()
	remoteWriteFile(t, filepath.Join(from, "clip.mp4"), "video")
	remoteWriteFile(t, filepath.Join(from, "notes.txt"), "text")

	added, err := remoteCall[remote.AddResult](t, updates, remote.MethodAdd, remote.AddParams{
		Sources: []string{"https://example.com/a", " ", "not a link", "clip.mp4", "notes.txt", "missing.mp4", "https://example.com/b"},
		Dir:     from,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(added.Jobs) != 3 || added.Jobs[0].Source != "https://example.com/a" || added.Jobs[1].Source != fileURL(filepath.Join(from, "clip.mp4")) || added.Jobs[2].Source != "https://example.com/b" {
		t.Fatalf("added %+v", added.Jobs)
	}
	for _, job := range added.Jobs {
		if job.ID == "" || job.Language != "简体中文" {
			t.Fatalf("added job %+v", job)
		}
	}
	reasons := map[string]string{}
	for _, s := range added.Skipped {
		reasons[s.Source] = s.Reason
	}
	if len(added.Skipped) != 4 || reasons[" "] != locales.PrecheckEmpty() || reasons["not a link"] != locales.PrecheckNotLink() || reasons["notes.txt"] != locales.PrecheckNoVideo() || !strings.Contains(reasons["missing.mp4"], "no such file") {
		t.Fatalf("skipped %+v", added.Skipped)
	}
	first, clip, last := added.Jobs[0].ID, added.Jobs[1].ID, added.Jobs[2].ID

	jobs, err := remoteCall[[]remote.Job](t, updates, remote.MethodJobs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 4 || jobs[0].ID != first || jobs[1].ID != clip || jobs[2].ID != last || jobs[3].ID != "old" {
		t.Fatalf("jobs %+v", jobs)
	}
	if jobs[0].Stage != "downloading" || jobs[1].Stage != "queued" || jobs[1].Status != locales.QueueStatusQueued() || jobs[3].FilmID != "old" || jobs[0].FilmID != "" {
		t.Fatalf("jobs %+v", jobs)
	}
	status, err := remoteCall[remote.Status](t, updates, remote.MethodStatus, nil)
	if err != nil || status.Running != first || status.Queued != 2 {
		t.Fatalf("status %+v, %v", status, err)
	}

	canceled, err := remoteCall[remote.Job](t, updates, remote.MethodCancel, remote.IDParams{ID: last})
	if err != nil {
		t.Fatal(err)
	}
	if canceled.ID != last || canceled.Stage != "failed" || !canceled.Canceled || canceled.Error != "" || canceled.Status != "已取消" {
		t.Fatalf("canceled waiting job %+v", canceled)
	}
	canceled, err = remoteCall[remote.Job](t, updates, remote.MethodCancel, remote.IDParams{ID: first})
	if err != nil {
		t.Fatal(err)
	}
	if canceled.ID != first || canceled.Stage != "failed" || !canceled.Canceled {
		t.Fatalf("canceled running job %+v", canceled)
	}
	if a.queue.active == first {
		t.Fatal("the canceled job still runs")
	}
	_, err = remoteCall[remote.Job](t, updates, remote.MethodCancel, remote.IDParams{ID: "nope"})
	remoteWantCode(t, err, remote.CodeNotFound)

	a.queue.stopActive(context.Canceled)
	waitUntil(t, updates, func() bool { return !a.running() })
}

// TestRemoteAddSkipsListsAndVideosAlreadyThere checks that kando add and the MCP server's add skip, each with why, a channel or a list that names no single video, a video already waiting or done in the library, and the same video given twice, while a video whose job failed is queued again on its card and answered like a new one.
func TestRemoteAddSkipsListsAndVideosAlreadyThere(t *testing.T) {
	a, _, updates := newTestApp(&fakePipeline{block: true})
	remoteServe(t, a)
	dir := t.TempDir()
	done := filepath.Join(dir, "Old", "video.mp4")
	remoteWriteFile(t, done, "video")
	a.edit(func(s *session) { s.LibraryDir = dir })
	a.library.Jobs = []core.Job{
		{ID: "done", Source: "https://www.youtube.com/watch?v=TRWgBxVAb1M", Stage: core.StageComplete, VideoPath: done, LibraryDir: dir},
		{ID: "failed", Source: "https://www.bilibili.com/video/BV1GJ411x7h7", Language: "简体中文", Stage: core.StageFailed, Err: errors.New("download: boom"), LibraryDir: dir},
	}
	// Adding from kando add or MCP, a failed video queued again included, leaves the window where the user is.
	a.page = pageAccount

	added, err := remoteCall[remote.AddResult](t, updates, remote.MethodAdd, remote.AddParams{
		Sources: []string{"https://example.com/a", "https://www.youtube.com/@veritasium", "https://www.youtube.com/playlist?list=PLx0sYbCqOb8", "https://youtu.be/TRWgBxVAb1M", "https://example.com/a", "https://www.bilibili.com/video/BV1GJ411x7h7/"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(added.Jobs) != 2 || added.Jobs[0].Source != "https://example.com/a" || added.Jobs[1].ID != "failed" || added.Jobs[1].Stage != "queued" {
		t.Fatalf("added %+v", added.Jobs)
	}
	var reasons []string
	for _, s := range added.Skipped {
		reasons = append(reasons, s.Source+": "+s.Reason)
	}
	want := []string{
		"https://www.youtube.com/@veritasium: " + locales.PrecheckChannel("YouTube"),
		"https://www.youtube.com/playlist?list=PLx0sYbCqOb8: " + locales.PrecheckList("YouTube"),
		"https://youtu.be/TRWgBxVAb1M: " + locales.QueueAlreadyInLibrary(),
		"https://example.com/a: " + locales.QueueAlreadyQueued(),
	}
	if !slices.Equal(reasons, want) {
		t.Fatalf("skipped %q\nwant %q", reasons, want)
	}
	if len(a.library.Jobs) != 3 {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
	if a.page != pageAccount {
		t.Fatalf("page %q after a remote add, want the account page", a.page)
	}

	for _, job := range a.library.Jobs {
		if job.ID != a.queue.active {
			a.Cancel(job.ID)
		}
	}
	a.queue.stopActive(context.Canceled)
	waitUntil(t, updates, func() bool { return !a.running() })
}

// TestRemoteListsAndCancelsAJobOfAnotherLibrary opens another library while a job runs and another waits in the first one: the status and the job list name them as the window's 处理中 shows them, the first library's finished job stays out of the list, and the running one can be cancelled.
func TestRemoteListsAndCancelsAJobOfAnotherLibrary(t *testing.T) {
	a, _, updates := newTestApp(&fakePipeline{block: true})
	remoteServe(t, a)
	first, second := t.TempDir(), t.TempDir()
	a.edit(func(s *session) {
		s.LibraryDir = first
		s.Libraries = []string{first, second}
	})
	done := filepath.Join(first, "Old", "old.mp4")
	remoteWriteFile(t, done, "video")
	a.library.Jobs = []core.Job{{ID: "old", Source: "https://example.com/old", Stage: core.StageComplete, VideoPath: done, LibraryDir: first}}
	if err := a.Enqueue("https://example.com/a", "https://example.com/b"); err != nil {
		t.Fatal(err)
	}
	ids := jobIDs(a)
	running, waiting := ids["https://example.com/a"], ids["https://example.com/b"]
	a.edit(func(s *session) { s.LibraryDir = second })

	status, err := remoteCall[remote.Status](t, updates, remote.MethodStatus, nil)
	if err != nil || status.Running != running || status.Queued != 1 {
		t.Fatalf("status %+v, %v", status, err)
	}
	jobs, err := remoteCall[[]remote.Job](t, updates, remote.MethodJobs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 || jobs[0].ID != running || jobs[1].ID != waiting {
		t.Fatalf("jobs %+v", jobs)
	}
	canceled, err := remoteCall[remote.Job](t, updates, remote.MethodCancel, remote.IDParams{ID: running})
	if err != nil {
		t.Fatal(err)
	}
	if canceled.ID != running || canceled.Stage != "failed" || !canceled.Canceled {
		t.Fatalf("canceled %+v", canceled)
	}
	_, err = remoteCall[remote.Job](t, updates, remote.MethodCancel, remote.IDParams{ID: "old"})
	remoteWantCode(t, err, remote.CodeNotFound)

	a.queue.stopActive(context.Canceled)
	waitUntil(t, updates, func() bool { return !a.running() })
}

// TestRemoteListsAJobThatEndedInAnotherLibrary lets a job finish after another library is opened: jobs.list still gives it, complete, so a client waiting on it (kando add --wait) sees it succeed rather than lose it.
func TestRemoteListsAJobThatEndedInAnotherLibrary(t *testing.T) {
	g := newGatePipeline()
	a, _, updates := newTestApp(g)
	remoteServe(t, a)
	first, second := t.TempDir(), t.TempDir()
	a.edit(func(s *session) {
		s.LibraryDir = first
		s.Libraries = []string{first, second}
	})
	if err := a.Enqueue("https://example.com/a"); err != nil {
		t.Fatal(err)
	}
	if got := nextStarted(t, g, updates); got != "https://example.com/a" {
		t.Fatalf("started %s", got)
	}
	id := a.RunningJob()
	a.edit(func(s *session) { s.LibraryDir = second })
	g.release <- struct{}{}
	waitUntil(t, updates, func() bool { return !a.running() })

	jobs, err := remoteCall[[]remote.Job](t, updates, remote.MethodJobs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ID != id || jobs[0].Stage != string(core.StageComplete) {
		t.Fatalf("jobs %+v", jobs)
	}
}

func TestRemoteAddSaysWhatIsMissing(t *testing.T) {
	a, _, updates := newTestApp(&fakePipeline{})
	remoteServe(t, a)
	a.edit(func(s *session) { s.LibraryDir = "" })
	_, err := remoteCall[remote.AddResult](t, updates, remote.MethodAdd, remote.AddParams{Sources: []string{"https://example.com/a"}})
	remoteWantCode(t, err, remote.CodeNotReady)
	var e *remote.Error
	if !errors.As(err, &e) || e.Message != locales.PrecheckNoLibrary() {
		t.Fatalf("error %v", err)
	}
	if len(a.library.Jobs) != 0 {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
	_, err = remoteCall[remote.AddResult](t, updates, remote.MethodAdd, remote.AddParams{})
	remoteWantCode(t, err, remote.CodeInvalid)
}

func TestRemoteSearchesSites(t *testing.T) {
	a, _, updates := newTestApp(&fakePipeline{})
	remoteServe(t, a)
	got, err := remoteCall[[]remote.Site](t, updates, remote.MethodSites, remote.SitesParams{Query: "https://youtu.be/abc", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Domain != "youtube.com" || got[0].Name == "" {
		t.Fatalf("sites %+v", got)
	}
	all, err := remoteCall[[]remote.Site](t, updates, remote.MethodSites, remote.SitesParams{})
	if err != nil || len(all) < 2 {
		t.Fatalf("all sites %d, %v", len(all), err)
	}
}

func TestRemoteShowOpensTheFilm(t *testing.T) {
	a, _, updates := newTestApp(&fakePipeline{})
	remoteServe(t, a)
	remoteLibrary(t, a)
	a.page = pageYouTube

	if _, err := remoteCall[any](t, updates, remote.MethodShow, remote.IDParams{ID: "clip"}); err != nil {
		t.Fatal(err)
	}
	if a.page != pageFilms || a.openKey != "" {
		t.Fatalf("a queued film shows page %q, film %q", a.page, a.openKey)
	}
	if _, err := remoteCall[any](t, updates, remote.MethodShow, remote.IDParams{ID: "yt"}); err != nil {
		t.Fatal(err)
	}
	if a.page != pageFilms || a.openKey != "yt" {
		t.Fatalf("a done film shows page %q, film %q", a.page, a.openKey)
	}
	if _, err := remoteCall[any](t, updates, remote.MethodShow, nil); err != nil {
		t.Fatal(err)
	}
	if a.page != pageFilms || a.openKey != "" {
		t.Fatalf("the library shows page %q, film %q", a.page, a.openKey)
	}
	_, err := remoteCall[any](t, updates, remote.MethodShow, remote.IDParams{ID: "nope"})
	remoteWantCode(t, err, remote.CodeNotFound)
}

func TestRemoteAnswersAnUnknownMethod(t *testing.T) {
	a, _, updates := newTestApp(&fakePipeline{})
	remoteServe(t, a)
	_, err := remoteCall[any](t, updates, "films.burn", nil)
	remoteWantCode(t, err, remote.CodeUnknownMethod)
	_, err = remoteCall[any](t, updates, remote.MethodFilm, []int{1})
	remoteWantCode(t, err, remote.CodeInvalid)
}

func TestRemoteSocketRefusesASecondApp(t *testing.T) {
	a, _, _ := newTestApp(&fakePipeline{})
	remoteServe(t, a)
	b, _, _ := newTestApp(&fakePipeline{})
	if err := b.remoteListen(); !errors.Is(err, remote.ErrInUse) {
		t.Fatalf("second listen: %v", err)
	}
}

func TestRemoteCuesKeepTheTranslationsOfLinesShownAtTheSameTime(t *testing.T) {
	seg := func(start, end int, text string) core.Segment {
		return core.Segment{Start: time.Duration(start) * time.Millisecond, End: time.Duration(end) * time.Millisecond, Text: text}
	}
	original := []core.Segment{seg(0, 1000, "Left"), seg(0, 1000, "Right"), seg(2000, 3000, "Done")}
	bilingual := []core.Segment{seg(0, 1000, "左\nLeft"), seg(0, 1000, "右\nRight"), seg(2000, 3000, "完\nDone")}
	want := []remote.Cue{{StartMS: 0, EndMS: 1000, Text: "Left", Translation: "左"}, {StartMS: 0, EndMS: 1000, Text: "Right", Translation: "右"}, {StartMS: 2000, EndMS: 3000, Text: "Done", Translation: "完"}}
	if got := remoteCues(original, bilingual); !slices.Equal(got, want) {
		t.Fatalf("aligned files: cues %+v", got)
	}

	// The bilingual file lacks the last line, so the files are not aligned and each cue takes the first unused one shown at its time.
	want[2].Translation = ""
	if got := remoteCues(original, bilingual[:2]); !slices.Equal(got, want) {
		t.Fatalf("files of different lengths: cues %+v", got)
	}
}
