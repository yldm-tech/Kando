package media

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"kando/packages/core"
	"kando/packages/track"
)

// fakePipeline downloads a link into a folder of the test, as video.mp4, hears transcribe at the start of the film, and translates each line as it is.
type fakePipeline struct {
	dir          string
	checkErr     error
	transcribe   []core.Segment
	translateErr error
	language     string
	// videoIn, when set, holds hearing back until it is closed, as a link's video that downloads faster than the film is heard is in before the film is made.
	videoIn chan struct{}
}

func (f *fakePipeline) Check() error { return f.checkErr }

func (f *fakePipeline) Download(_ context.Context, source string) (string, error) {
	if path, ok := localVideoPath(source); ok {
		return path, nil
	}
	if f.dir == "" {
		return "", errors.New("the test gave the fake no folder to download into")
	}
	path := filepath.Join(f.dir, "video.mp4")
	return path, os.WriteFile(path, []byte("v"), 0o644)
}

func (f *fakePipeline) Length(context.Context, string) time.Duration { return 30 * time.Second }

func (f *fakePipeline) Hear(ctx context.Context, _ string, from, to time.Duration, heard func(core.Segment)) (string, error) {
	if f.videoIn != nil {
		select {
		case <-f.videoIn:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	for _, s := range f.transcribe {
		if mid := (s.Start + s.End) / 2; mid >= from && mid < to {
			heard(s)
		}
	}
	return "en", nil
}

func (f *fakePipeline) Translate(_ context.Context, segments []core.Segment, language string) ([]core.Segment, error) {
	f.language = language
	return segments, f.translateErr
}

// runStages makes a link's film with p as the queue does, returning each stage it entered and the job it ended with.
func runStages(t *testing.T, p Pipeline) ([]core.Stage, core.Job) {
	t.Helper()
	if f, ok := p.(interface{ setDir(string) }); ok {
		f.setDir(t.TempDir())
	}
	job, _ := core.NewJob("https://example.com/video")
	var stages []core.Stage
	final := Make(context.Background(), p, job, "简体中文", "mac", nil, func(j core.Job) { stages = enteredStage(stages, j) })
	return stages, final
}

func (f *fakePipeline) setDir(dir string) { f.dir = dir }

// A link's video downloads beside the making of its subtitles; one in before they are made is taken at once, so the job downloads once. Hearing waits for the video here, as the race between the two is not what this test is about: a video still coming once the subtitles are made has the job say it downloads again (TestALinkIsMadeFromItsSoundWhileItsVideoDownloads).
func TestMakeReportsEveryStage(t *testing.T) {
	p := &fakePipeline{dir: t.TempDir(), transcribe: []core.Segment{{End: time.Second, Text: "hello"}}, videoIn: make(chan struct{})}
	job, _ := core.NewJob("https://example.com/video")
	var stages []core.Stage
	var videoIn sync.Once
	job = Make(context.Background(), p, job, "简体中文", "mac", nil, func(j core.Job) {
		stages = enteredStage(stages, j)
		// A link's job has a video once the making took in the one downloaded beside it (making.videoCame).
		if j.VideoPath != "" {
			videoIn.Do(func() { close(p.videoIn) })
		}
	})
	want := []core.Stage{core.StageDownloading, core.StageTranscribing, core.StageTranslating, core.StageComplete}
	if !reflect.DeepEqual(stages, want) {
		t.Fatalf("stages = %v, want %v", stages, want)
	}
	if filepath.Base(job.VideoPath) != "video.mp4" || filepath.Base(job.SubtitlePath) != "video.bilingual.srt" || job.Err != nil {
		t.Fatalf("unexpected job: %#v", job)
	}
	if p.language != "简体中文" {
		t.Fatalf("translated into %q", p.language)
	}
}

func TestMakeStopsAtFailedCheck(t *testing.T) {
	stages, job := runStages(t, &fakePipeline{checkErr: errors.New("ffmpeg is not installed")})
	if !reflect.DeepEqual(stages, []core.Stage{core.StageFailed}) {
		t.Fatalf("stages = %v", stages)
	}
	if job.Err == nil || job.Err.Error() != "check: ffmpeg is not installed" {
		t.Fatalf("err = %v", job.Err)
	}
}

func TestMakeFailsWithoutSpeech(t *testing.T) {
	_, job := runStages(t, &fakePipeline{})
	if job.Stage != core.StageFailed || !errors.Is(job.Err, ErrNoSpeech) || !job.CompletedAt.IsZero() {
		t.Fatalf("unexpected job: %#v", job)
	}
}

func TestMakeSkipsDownloadingForALocalVideo(t *testing.T) {
	p := &fakePipeline{transcribe: []core.Segment{{End: time.Second, Text: "hello"}}}
	video := filepath.Join(t.TempDir(), "interview.mp4")
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	job, _ := core.NewJob(core.FileURL(video))
	var stages []core.Stage
	before := time.Now()
	final := Make(context.Background(), p, job, "简体中文", "mac", nil, func(j core.Job) { stages = enteredStage(stages, j) })
	want := []core.Stage{core.StageTranscribing, core.StageTranslating, core.StageComplete}
	if !reflect.DeepEqual(stages, want) || final.VideoPath != video {
		t.Fatalf("stages = %v, want %v; video %q", stages, want, final.VideoPath)
	}
	if final.CompletedAt.Before(before) || final.CompletedAt.After(time.Now()) {
		t.Fatalf("completed at %v", final.CompletedAt)
	}
}

// A film whose length cannot be told, as a file that is not a video, fails, and so does one whose document a newer Kando wrote, which is not written to.
func TestMakeFailsWhatItCannotMake(t *testing.T) {
	job, video := makeJob(t)
	job.SpeechLanguage = "ja"
	if done, _ := makeFilmOf(t, &makeFilm{}, job, 0); done.Stage != core.StageFailed || !errors.Is(done.Err, ErrNoLength) {
		t.Fatalf("no length: %+v", done)
	}
	if err := os.WriteFile(track.LogPath(video, "pc"), []byte(`{"k":"head","n":1,"t":1,"v":99}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if done, _ := makeFilmOf(t, &makeFilm{length: 30 * time.Second}, job, 0); done.Stage != core.StageFailed || !strings.Contains(done.Err.Error(), "newer Kando") {
		t.Fatalf("newer: %+v", done)
	}
}

// localSteps is Tools with the check passed and the steps after Download stubbed, so Run reaches the real Download of a local file.
type localSteps struct{ Tools }

func (localSteps) Check() error { return nil }

func (localSteps) Length(context.Context, string) time.Duration { return 30 * time.Second }

func (localSteps) Hear(_ context.Context, _ string, from, _ time.Duration, heard func(core.Segment)) (string, error) {
	if from == 0 {
		heard(core.Segment{End: time.Second, Text: "hello"})
	}
	return "en", nil
}

func (localSteps) Translate(_ context.Context, segments []core.Segment, _ string) ([]core.Segment, error) {
	return segments, nil
}

// TestMakeReadsALocalVideoInPlace checks that Tools does not download a file:// source: the job never shows downloading and its video is the file itself, and a missing file still fails the job.
func TestMakeReadsALocalVideoInPlace(t *testing.T) {
	video := filepath.Join(t.TempDir(), "interview.mp4")
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	job, err := core.NewJob(core.FileURL(video))
	if err != nil {
		t.Fatal(err)
	}
	var stages []core.Stage
	final := Make(context.Background(), localSteps{}, job, "简体中文", "mac", nil, func(j core.Job) { stages = enteredStage(stages, j) })
	want := []core.Stage{core.StageTranscribing, core.StageTranslating, core.StageComplete}
	if !reflect.DeepEqual(stages, want) || final.VideoPath != video {
		t.Fatalf("stages %v video %q", stages, final.VideoPath)
	}

	gone, err := core.NewJob(core.FileURL(filepath.Join(t.TempDir(), "gone.mp4")))
	if err != nil {
		t.Fatal(err)
	}
	if failed := Make(context.Background(), localSteps{}, gone, "简体中文", "mac", nil, func(core.Job) {}); failed.Stage != core.StageFailed || !errors.Is(failed.Err, os.ErrNotExist) {
		t.Fatalf("missing file %+v", failed)
	}
}

// tracingPipeline reports through the context's Trace as Tools does: the video's name and how far the download has gone.
type tracingPipeline struct {
	fakePipeline
	downloadErr error
}

func (p *tracingPipeline) Download(ctx context.Context, _ string) (string, error) {
	reportInfo(ctx, Info{Title: "Me at the zoo", Thumbnail: "https://i.ytimg.com/vi/jNQXAC9IVRw/hqdefault.jpg"})
	reportProgress(ctx, core.Fraction(0.4))
	if p.downloadErr != nil {
		return "", p.downloadErr
	}
	return p.fakePipeline.Download(ctx, "")
}

func TestMakeReportsWhatTheStagesTrace(t *testing.T) {
	job, _ := core.NewJob("https://www.youtube.com/watch?v=jNQXAC9IVRw")
	var seen []core.Job
	p := &tracingPipeline{fakePipeline: fakePipeline{dir: t.TempDir(), transcribe: []core.Segment{{End: time.Second, Text: "hello"}}}}
	final := Make(context.Background(), p, job, "简体中文", "mac", nil, func(j core.Job) { seen = append(seen, j) })
	if final.Stage != core.StageComplete || final.Title != "Me at the zoo" || final.Thumbnail == "" || final.SpokenLanguage != "en" {
		t.Fatalf("final = %+v", final)
	}
	downloading := false
	for _, j := range seen {
		switch {
		case j.Stage == core.StageTranslating && j.SpokenLanguage != "en":
			t.Fatalf("the translation started without the language heard: %+v", j)
		case j.Stage == core.StageDownloading && j.Progress.Known && j.Progress.Fraction == 0.4:
			downloading = j.Title == "Me at the zoo"
		}
	}
	if !downloading {
		t.Fatalf("reports %+v", seen)
	}
	// A stage is timed from its first report that says how far it has gone, which its later reports keep, so how long it has left can be told from how fast it goes.
	for i, j := range seen {
		p := j.Progress
		if p.Known != !p.Started.IsZero() || (p.Known && p.From != 0 && p.From > p.Fraction) {
			t.Fatalf("report %d of stage %s: %+v", i, j.Stage, p)
		}
		if before := seen[max(i-1, 0)].Progress; i > 0 && seen[i-1].Stage == j.Stage && before.Known && (!p.Started.Equal(before.Started) || p.From != before.From) {
			t.Fatalf("report %d of stage %s timed from %v at %v, the one before from %v at %v", i, j.Stage, p.Started, p.From, before.Started, before.From)
		}
	}
	if final.Progress != (core.Progress{}) {
		t.Fatalf("a finished job keeps progress %+v", final.Progress)
	}
}

func TestMakeKeepsTheTitleOfAFailedDownload(t *testing.T) {
	job, _ := core.NewJob("https://www.youtube.com/watch?v=jNQXAC9IVRw")
	final := Make(context.Background(), &tracingPipeline{downloadErr: errors.New("yt-dlp: exit status 1: ERROR: [youtube] jNQXAC9IVRw: Private video")}, job, "简体中文", "mac", nil, func(core.Job) {})
	if final.Stage != core.StageFailed || final.Title != "Me at the zoo" || final.Progress != (core.Progress{}) {
		t.Fatalf("final = %+v", final)
	}
}

func TestMakeSaysWhyTheDownloadFailed(t *testing.T) {
	age, err := os.ReadFile(filepath.Join("testdata", "ytdlp", "errors", "age-youtube.stderr"))
	if err != nil {
		t.Fatal(err)
	}
	job, _ := core.NewJob("https://www.youtube.com/watch?v=Tq92D6wQ1mg")
	failed := Make(context.Background(), &tracingPipeline{downloadErr: errors.New("yt-dlp: exit status 1: " + string(age))}, job, "简体中文", "mac", nil, func(core.Job) {})
	if failed.FailKind != core.FailAge {
		t.Fatalf("kind %q", failed.FailKind)
	}
	// The same answer with a browser's login borrowed means that login has run out.
	job.Cookies = "chrome"
	expired := Make(context.Background(), &tracingPipeline{downloadErr: errors.New("yt-dlp: exit status 1: " + string(age))}, job, "简体中文", "mac", nil, func(core.Job) {})
	if expired.FailKind != core.FailLoginExpired {
		t.Fatalf("kind with cookies %q", expired.FailKind)
	}
	// A later stage's failure is not a download's.
	later := Make(context.Background(), &fakePipeline{dir: t.TempDir(), transcribe: []core.Segment{{End: time.Second, Text: "a"}}, translateErr: errors.New("ERROR: Private video")}, job, "简体中文", "mac", nil, func(core.Job) {})
	if later.FailKind != "" {
		t.Fatalf("translate failure kind %q", later.FailKind)
	}
}

// partialPipeline translates every line but the first, which its translator could not.
type partialPipeline struct{ fakePipeline }

func (p *partialPipeline) setDir(dir string) { p.dir = dir }

func (p *partialPipeline) Translate(_ context.Context, segments []core.Segment, _ string) ([]core.Segment, error) {
	out := append([]core.Segment(nil), segments...)
	out[0].Text = ""
	return out, &UntranslatedError{Lines: 1, Total: len(segments), Err: errors.New("translation returned 1 lines for 2")}
}

// A film whose translation left some lines untranslated is done, not lost: its job completes and says how many lines are missing.
func TestMakeCompletesAFilmWithUntranslatedLines(t *testing.T) {
	p := &partialPipeline{fakePipeline{transcribe: []core.Segment{{End: time.Second, Text: "a"}, {Start: time.Second, End: 2 * time.Second, Text: "b"}}}}
	stages, job := runStages(t, p)
	if job.Stage != core.StageComplete || job.Err != nil || job.Untranslated != 1 || job.SubtitlePath == "" {
		t.Fatalf("job %+v, stages %v", job, stages)
	}
}

// A film made without an AI service set up is a film too: the translator is not asked, and the job says why it translated nothing. A job making a done film again needs the service, so it fails rather than replace the film's translation.
func TestMakeWithoutATranslatorKeepsTheOriginalLines(t *testing.T) {
	p := &noTranslator{fakePipeline{transcribe: []core.Segment{{End: time.Second, Text: "a"}, {Start: time.Second, End: 2 * time.Second, Text: "b"}}}}
	_, done := runStages(t, p)
	if done.Stage != core.StageComplete || done.Untranslated != 2 || !strings.Contains(done.TranslationErr, "KANDO_LLM_MODEL") || p.language != "" {
		t.Fatalf("job %+v, translated into %q", done, p.language)
	}
	for _, redo := range []core.Redo{core.RedoAll, core.RedoMissing, core.RedoTranslation} {
		again := done
		again.Redo, again.Stage = redo, core.StageQueued
		if failed := Make(context.Background(), p, again, "简体中文", "mac", nil, func(core.Job) {}); failed.Stage != core.StageFailed || !strings.HasPrefix(failed.Err.Error(), "check: ") || failed.TranslationErr != done.TranslationErr {
			t.Fatalf("%s without a translator: %+v", redo, failed)
		}
	}
}

// noTranslator is a pipeline whose translator is not set up (TranslateChecker).
type noTranslator struct{ fakePipeline }

func (p *noTranslator) setDir(dir string) { p.dir = dir }

func (p *noTranslator) CheckTranslate() error { return errors.New("KANDO_LLM_MODEL is not set") }

var heard = []core.Segment{
	{End: 1500 * time.Millisecond, Text: "Hello there."},
	{Start: 1500 * time.Millisecond, End: 3250 * time.Millisecond, Text: "Nice to meet you."},
}

// A video the library keeps in a folder of its own is named video.<ext>: the folder's name is what it is called.
func TestFilmTitleOfAVideoInAFolderOfItsOwn(t *testing.T) {
	for _, c := range []struct {
		job  core.Job
		want string
	}{
		{core.Job{Title: " The Site's Title ", VideoPath: "/films/Night Walk/video.mp4"}, "The Site's Title"},
		{core.Job{VideoPath: "/films/Night Walk/video.mp4"}, "Night Walk"},
		{core.Job{VideoPath: "/films/Night Walk.mkv"}, "Night Walk"},
		{core.Job{}, ""},
	} {
		if got := filmTitle(c.job); got != c.want {
			t.Fatalf("filmTitle(%+v) = %q, want %q", c.job, got, c.want)
		}
	}
}
