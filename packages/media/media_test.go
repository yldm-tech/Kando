package media

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"kando/packages/core"
)

type fakePipeline struct {
	checkErr     error
	transcribe   []core.Segment
	translateErr error
	language     string
}

func (f *fakePipeline) Check() error { return f.checkErr }

func (f *fakePipeline) Download(context.Context, string) (string, error) {
	return "/videos/talk.mp4", nil
}

func (f *fakePipeline) Transcribe(context.Context, string) ([]core.Segment, error) {
	return f.transcribe, nil
}

func (f *fakePipeline) Translate(_ context.Context, segments []core.Segment, language string) ([]core.Segment, error) {
	f.language = language
	return segments, f.translateErr
}

func (f *fakePipeline) Export(context.Context, string, []core.Segment, []core.Segment) (string, error) {
	return "/videos/talk.bilingual.srt", nil
}

func runStages(p Pipeline) ([]core.Stage, core.Job) {
	job, _ := core.NewJob("https://example.com/video")
	var stages []core.Stage
	final := Run(context.Background(), p, job, "简体中文", func(j core.Job) { stages = append(stages, j.Stage) })
	return stages, final
}

func TestRunReportsEveryStage(t *testing.T) {
	p := &fakePipeline{transcribe: []core.Segment{{End: time.Second, Text: "hello"}}}
	stages, job := runStages(p)

	want := []core.Stage{core.StageDownloading, core.StageTranscribing, core.StageTranslating, core.StageExporting, core.StageComplete}
	if !reflect.DeepEqual(stages, want) {
		t.Fatalf("stages = %v, want %v", stages, want)
	}
	if job.VideoPath != "/videos/talk.mp4" || job.SubtitlePath != "/videos/talk.bilingual.srt" || job.Err != nil {
		t.Fatalf("unexpected job: %#v", job)
	}
	if p.language != "简体中文" {
		t.Fatalf("translated into %q", p.language)
	}
}

func TestRunStopsAtFailedCheck(t *testing.T) {
	stages, job := runStages(&fakePipeline{checkErr: errors.New("ffmpeg is not installed")})
	if !reflect.DeepEqual(stages, []core.Stage{core.StageFailed}) {
		t.Fatalf("stages = %v", stages)
	}
	if job.Err == nil || job.Err.Error() != "check: ffmpeg is not installed" {
		t.Fatalf("err = %v", job.Err)
	}
}

func TestRunFailsWithoutSpeech(t *testing.T) {
	_, job := runStages(&fakePipeline{})
	if job.Stage != core.StageFailed || !errors.Is(job.Err, ErrNoSpeech) {
		t.Fatalf("unexpected job: %#v", job)
	}
}

func TestRunKeepsVideoPathWhenLaterStageFails(t *testing.T) {
	p := &fakePipeline{transcribe: []core.Segment{{Text: "hello"}}, translateErr: context.Canceled}
	_, job := runStages(p)
	if job.Stage != core.StageFailed || !errors.Is(job.Err, context.Canceled) || job.VideoPath != "/videos/talk.mp4" {
		t.Fatalf("unexpected job: %#v", job)
	}
}

func TestRunSkipsDownloadingForALocalVideo(t *testing.T) {
	p := &fakePipeline{transcribe: []core.Segment{{End: time.Second, Text: "hello"}}}
	job, _ := core.NewJob("file:///Users/test/interview.mp4")
	var stages []core.Stage
	before := time.Now()
	final := Run(context.Background(), p, job, "简体中文", func(j core.Job) { stages = append(stages, j.Stage) })
	want := []core.Stage{core.StageTranscribing, core.StageTranslating, core.StageExporting, core.StageComplete}
	if !reflect.DeepEqual(stages, want) {
		t.Fatalf("stages = %v, want %v", stages, want)
	}
	if final.VideoPath != "/videos/talk.mp4" {
		t.Fatalf("video = %q; Download should still give the path", final.VideoPath)
	}
	if final.CompletedAt.Before(before) || final.CompletedAt.After(time.Now()) {
		t.Fatalf("completed at %v", final.CompletedAt)
	}
}

func TestRunLeavesCompletedAtZeroOnFailure(t *testing.T) {
	_, job := runStages(&fakePipeline{})
	if job.Stage != core.StageFailed || !job.CompletedAt.IsZero() {
		t.Fatalf("unexpected job: %#v", job)
	}
}

// localSteps is Tools with the check passed and the steps after Download stubbed, so Run reaches the real Download of a local file.
type localSteps struct{ Tools }

func (localSteps) Check() error { return nil }

func (localSteps) Transcribe(context.Context, string) ([]core.Segment, error) {
	return []core.Segment{{End: time.Second, Text: "hello"}}, nil
}

func (localSteps) Translate(_ context.Context, segments []core.Segment, _ string) ([]core.Segment, error) {
	return segments, nil
}

func (localSteps) Export(context.Context, string, []core.Segment, []core.Segment) (string, error) {
	return "/videos/interview.bilingual.srt", nil
}

// TestRunReadsALocalVideoInPlace checks that Tools does not download a file:// source: the job never shows downloading and its video is the file itself, and a missing file still fails the job.
func TestRunReadsALocalVideoInPlace(t *testing.T) {
	video := filepath.Join(t.TempDir(), "interview.mp4")
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	job, err := core.NewJob(core.FileURL(video))
	if err != nil {
		t.Fatal(err)
	}
	var stages []core.Stage
	final := Run(context.Background(), localSteps{}, job, "简体中文", func(j core.Job) { stages = append(stages, j.Stage) })
	want := []core.Stage{core.StageTranscribing, core.StageTranslating, core.StageExporting, core.StageComplete}
	if !reflect.DeepEqual(stages, want) || final.VideoPath != video {
		t.Fatalf("stages %v video %q", stages, final.VideoPath)
	}

	gone, err := core.NewJob(core.FileURL(filepath.Join(t.TempDir(), "gone.mp4")))
	if err != nil {
		t.Fatal(err)
	}
	if failed := Run(context.Background(), localSteps{}, gone, "简体中文", func(core.Job) {}); failed.Stage != core.StageFailed || !errors.Is(failed.Err, os.ErrNotExist) {
		t.Fatalf("missing file %+v", failed)
	}
}

// tracingPipeline reports through the context's Trace as Tools does: the video's name and how far the download has gone, then how far the transcription has gone.
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
	return "/videos/talk.mp4", nil
}

func (p *tracingPipeline) Transcribe(ctx context.Context, _ string) ([]core.Segment, error) {
	reportProgress(ctx, core.Fraction(0.7))
	return []core.Segment{{End: time.Second, Text: "hello"}}, nil
}

func TestRunReportsWhatTheStagesTrace(t *testing.T) {
	job, _ := core.NewJob("https://www.youtube.com/watch?v=jNQXAC9IVRw")
	var seen []core.Job
	final := Run(context.Background(), &tracingPipeline{}, job, "简体中文", func(j core.Job) { seen = append(seen, j) })
	if final.Stage != core.StageComplete || final.Title != "Me at the zoo" || final.Thumbnail == "" {
		t.Fatalf("final = %+v", final)
	}
	var downloading, transcribing bool
	for _, j := range seen {
		switch {
		case j.Stage == core.StageDownloading && j.Progress.Known && j.Progress.Fraction == 0.4:
			downloading = j.Title == "Me at the zoo"
		case j.Stage == core.StageTranscribing && j.Progress.Known && j.Progress.Fraction == 0.7:
			transcribing = true
		case j.Stage == core.StageTranslating && j.Progress.Known && j.Progress.Fraction != 0:
			t.Fatalf("the translation started with the transcription's progress: %+v", j.Progress)
		}
	}
	if !downloading || !transcribing {
		t.Fatalf("reports %+v", seen)
	}
	if final.Progress != (core.Progress{}) {
		t.Fatalf("a finished job keeps progress %+v", final.Progress)
	}
}

func TestRunKeepsTheTitleOfAFailedDownload(t *testing.T) {
	job, _ := core.NewJob("https://www.youtube.com/watch?v=jNQXAC9IVRw")
	final := Run(context.Background(), &tracingPipeline{downloadErr: errors.New("yt-dlp: exit status 1: ERROR: [youtube] jNQXAC9IVRw: Private video")}, job, "简体中文", func(core.Job) {})
	if final.Stage != core.StageFailed || final.Title != "Me at the zoo" || final.Progress != (core.Progress{}) {
		t.Fatalf("final = %+v", final)
	}
}

func TestRunSaysWhyTheDownloadFailed(t *testing.T) {
	age, err := os.ReadFile(filepath.Join("testdata", "ytdlp", "errors", "age-youtube.stderr"))
	if err != nil {
		t.Fatal(err)
	}
	job, _ := core.NewJob("https://www.youtube.com/watch?v=Tq92D6wQ1mg")
	failed := Run(context.Background(), &tracingPipeline{downloadErr: errors.New("yt-dlp: exit status 1: " + string(age))}, job, "简体中文", func(core.Job) {})
	if failed.FailKind != core.FailAge {
		t.Fatalf("kind %q", failed.FailKind)
	}
	// The same answer with a browser's login borrowed means that login has run out.
	job.Cookies = "chrome"
	expired := Run(context.Background(), &tracingPipeline{downloadErr: errors.New("yt-dlp: exit status 1: " + string(age))}, job, "简体中文", func(core.Job) {})
	if expired.FailKind != core.FailLoginExpired {
		t.Fatalf("kind with cookies %q", expired.FailKind)
	}
	// A later stage's failure is not a download's.
	later := Run(context.Background(), &fakePipeline{transcribe: []core.Segment{{Text: "a"}}, translateErr: errors.New("ERROR: Private video")}, job, "简体中文", func(core.Job) {})
	if later.FailKind != "" {
		t.Fatalf("translate failure kind %q", later.FailKind)
	}
}

// partialPipeline translates every line but the first, which its translator could not.
type partialPipeline struct{ fakePipeline }

func (p *partialPipeline) Translate(_ context.Context, segments []core.Segment, _ string) ([]core.Segment, error) {
	out := append([]core.Segment(nil), segments...)
	out[0].Text = ""
	return out, &UntranslatedError{Lines: 1, Total: len(segments), Err: errors.New("translation returned 1 lines for 2")}
}

// A film whose translation left some lines untranslated is done, not lost: its job completes and says how many lines are missing.
func TestRunCompletesAFilmWithUntranslatedLines(t *testing.T) {
	p := &partialPipeline{fakePipeline{transcribe: []core.Segment{{Text: "a"}, {Text: "b"}}}}
	stages, job := runStages(p)
	if job.Stage != core.StageComplete || job.Err != nil || job.Untranslated != 1 || job.SubtitlePath == "" {
		t.Fatalf("job %+v, stages %v", job, stages)
	}
	// Running it again starts the count over.
	again := Run(context.Background(), &fakePipeline{transcribe: []core.Segment{{Text: "a"}}}, job, "简体中文", func(core.Job) {})
	if again.Untranslated != 0 {
		t.Fatalf("again %+v", again)
	}
}

// resumeTools is Tools with the downloads of splitYTDLP, an ffmpeg that writes an empty audio file, a whisper-cli that hears two lines and counts its runs in the returned file, and the translator at chat.
func resumeTools(t *testing.T, chat string) (tools Tools, ytdlpRuns, whisperRuns string) {
	t.Helper()
	ytdlp, ytdlpRuns := splitYTDLP(t)
	dir := t.TempDir()
	whisperRuns = filepath.Join(dir, "whisper-runs")
	ffmpeg := writeFakeTool(t, dir, "ffmpeg", fakeTool{Act: "extract-audio"})
	whisper := writeFakeTool(t, dir, "whisper-cli", fakeTool{Runs: whisperRuns, Act: "transcribe", Transcript: `{"transcription":[{"offsets":{"from":0,"to":1500},"text":" Hello there."},{"offsets":{"from":1500,"to":3250},"text":" Nice to meet you."}]}`})
	model := filepath.Join(dir, "ggml-small.bin")
	if err := os.WriteFile(model, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	tools = Tools{
		OutputDir: t.TempDir(), DownloadTemp: t.TempDir(),
		YTDLP: ytdlp, FFmpeg: ffmpeg, Whisper: whisper, WhisperModel: model,
		Translator: ChatTranslator{BaseURL: chat, Model: "m"},
	}
	return tools, ytdlpRuns, whisperRuns
}

func runCount(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(data), "run")
}

// runRecording runs job as the queue does, with the error of an earlier run forgotten, and returns each stage it entered.
func runRecording(p Pipeline, job core.Job) ([]core.Stage, core.Job) {
	job.Stage, job.Err = core.StageQueued, nil
	var stages []core.Stage
	final := Run(context.Background(), p, job, "简体中文", func(j core.Job) {
		if len(stages) == 0 || stages[len(stages)-1] != j.Stage {
			stages = append(stages, j.Stage)
		}
	})
	return stages, final
}

var heard = []core.Segment{
	{End: 1500 * time.Millisecond, Text: "Hello there."},
	{Start: 1500 * time.Millisecond, End: 3250 * time.Millisecond, Text: "Nice to meet you."},
}

// A translation that fails, as with a key the API rejects, leaves the transcript beside the video, and the retry goes straight to translating it: the video is not downloaded again and whisper-cli does not run again.
func TestRunResumesAfterAFailedTranslation(t *testing.T) {
	srv, _ := flakyChat(t, http.StatusUnauthorized)
	tools, ytdlpRuns, whisperRuns := resumeTools(t, srv.URL)
	job, _ := core.NewJob("https://www.youtube.com/watch?v=jNQXAC9IVRw")

	_, failed := runRecording(tools, job)
	if failed.Stage != core.StageFailed || !strings.HasPrefix(failed.Err.Error(), "translate:") {
		t.Fatalf("first run %+v", failed)
	}
	if !failed.Transcribed {
		t.Fatal("the failed job does not say its transcript was saved")
	}
	transcript := strings.TrimSuffix(failed.VideoPath, ".mp4") + ".srt"
	data, err := os.ReadFile(transcript)
	if err != nil {
		t.Fatalf("no transcript after the failed translation: %v", err)
	}
	if saved, err := ParseSRT(data); err != nil || !reflect.DeepEqual(saved, heard) {
		t.Fatalf("saved transcript %+v, %v", saved, err)
	}
	if entries, _ := os.ReadDir(filepath.Dir(transcript)); len(entries) != 2 {
		t.Fatalf("the video's folder holds %v", entries)
	}

	stages, done := runRecording(tools, failed)
	want := []core.Stage{core.StageTranslating, core.StageExporting, core.StageComplete}
	if !reflect.DeepEqual(stages, want) {
		t.Fatalf("retry stages %v, want %v", stages, want)
	}
	if done.Err != nil || done.VideoPath != failed.VideoPath || done.SubtitlePath == "" || !done.Transcribed {
		t.Fatalf("retry %+v", done)
	}
	if got := runCount(t, ytdlpRuns); got != 1 {
		t.Fatalf("yt-dlp ran %d times", got)
	}
	if got := runCount(t, whisperRuns); got != 1 {
		t.Fatalf("whisper-cli ran %d times", got)
	}
	bilingual, err := os.ReadFile(done.SubtitlePath)
	if err != nil || !strings.Contains(string(bilingual), "Nice to meet you.") {
		t.Fatalf("bilingual %q, %v", bilingual, err)
	}
}

// A done film processed again, its Transcribed cleared, keeps the video it downloaded but is transcribed anew.
func TestRunTranscribesAgainWhenTheJobHasNoTranscript(t *testing.T) {
	srv, _ := flakyChat(t)
	tools, ytdlpRuns, whisperRuns := resumeTools(t, srv.URL)
	job, _ := core.NewJob("https://www.youtube.com/watch?v=jNQXAC9IVRw")
	_, done := runRecording(tools, job)
	if done.Stage != core.StageComplete || !done.Transcribed {
		t.Fatalf("first run %+v", done)
	}

	done.Transcribed = false
	stages, again := runRecording(tools, done)
	want := []core.Stage{core.StageTranscribing, core.StageTranslating, core.StageExporting, core.StageComplete}
	if !reflect.DeepEqual(stages, want) || again.Stage != core.StageComplete || !again.Transcribed {
		t.Fatalf("stages %v, job %+v", stages, again)
	}
	if got := runCount(t, ytdlpRuns); got != 1 {
		t.Fatalf("yt-dlp ran %d times", got)
	}
	if got := runCount(t, whisperRuns); got != 2 {
		t.Fatalf("whisper-cli ran %d times", got)
	}
}

// A job that says it was transcribed but whose transcript is gone is transcribed again, and one whose video is gone is downloaded again.
func TestRunRedoesWhatIsMissing(t *testing.T) {
	srv, _ := flakyChat(t)
	tools, ytdlpRuns, whisperRuns := resumeTools(t, srv.URL)
	job, _ := core.NewJob("https://www.youtube.com/watch?v=jNQXAC9IVRw")
	_, done := runRecording(tools, job)
	if done.Stage != core.StageComplete {
		t.Fatalf("first run %+v", done)
	}

	if err := os.Remove(strings.TrimSuffix(done.VideoPath, ".mp4") + ".srt"); err != nil {
		t.Fatal(err)
	}
	stages, again := runRecording(tools, done)
	if want := []core.Stage{core.StageTranscribing, core.StageTranslating, core.StageExporting, core.StageComplete}; !reflect.DeepEqual(stages, want) || !again.Transcribed {
		t.Fatalf("without the transcript: stages %v, job %+v", stages, again)
	}
	if runCount(t, ytdlpRuns) != 1 || runCount(t, whisperRuns) != 2 {
		t.Fatalf("yt-dlp ran %d times, whisper-cli %d", runCount(t, ytdlpRuns), runCount(t, whisperRuns))
	}

	if err := os.RemoveAll(filepath.Dir(again.VideoPath)); err != nil {
		t.Fatal(err)
	}
	stages, third := runRecording(tools, again)
	if want := []core.Stage{core.StageDownloading, core.StageTranscribing, core.StageTranslating, core.StageExporting, core.StageComplete}; !reflect.DeepEqual(stages, want) || third.Stage != core.StageComplete {
		t.Fatalf("without the video: stages %v, job %+v", stages, third)
	}
	if runCount(t, ytdlpRuns) != 2 || runCount(t, whisperRuns) != 3 {
		t.Fatalf("yt-dlp ran %d times, whisper-cli %d", runCount(t, ytdlpRuns), runCount(t, whisperRuns))
	}
}

// A video with a partial download beside it never merged, so the retry downloads it again rather than transcribing a silent track.
func TestRunDownloadsAgainAVideoThatNeverMerged(t *testing.T) {
	srv, _ := flakyChat(t, http.StatusUnauthorized)
	tools, ytdlpRuns, whisperRuns := resumeTools(t, srv.URL)
	job, _ := core.NewJob("https://www.youtube.com/watch?v=jNQXAC9IVRw")
	_, failed := runRecording(tools, job)
	if failed.Stage != core.StageFailed || failed.VideoPath == "" || !failed.Transcribed {
		t.Fatalf("first run %+v", failed)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(failed.VideoPath), "video.f251.webm"), []byte("track 251"), 0o644); err != nil {
		t.Fatal(err)
	}
	stages, done := runRecording(tools, failed)
	if want := []core.Stage{core.StageDownloading, core.StageTranscribing, core.StageTranslating, core.StageExporting, core.StageComplete}; !reflect.DeepEqual(stages, want) || done.Stage != core.StageComplete {
		t.Fatalf("stages %v, job %+v", stages, done)
	}
	if got := runCount(t, ytdlpRuns); got != 3 {
		t.Fatalf("yt-dlp ran %d times, want the first download and two for the unmerged one", got)
	}
	// The transcript beside the video was made from the file the new download replaced.
	if got := runCount(t, whisperRuns); got != 2 {
		t.Fatalf("whisper-cli ran %d times, want the re-downloaded video transcribed again", got)
	}
}

// A local video's job checks its file through Download every time, and reads the transcript saved beside it instead of transcribing it again.
func TestRunResumesALocalVideo(t *testing.T) {
	srv, _ := flakyChat(t, http.StatusUnauthorized)
	tools, _, whisperRuns := resumeTools(t, srv.URL)
	video := filepath.Join(t.TempDir(), "interview.mp4")
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	job, _ := core.NewJob(core.FileURL(video))
	_, failed := runRecording(tools, job)
	if failed.Stage != core.StageFailed || !failed.Transcribed || failed.VideoPath != video {
		t.Fatalf("first run %+v", failed)
	}
	stages, done := runRecording(tools, failed)
	if want := []core.Stage{core.StageTranslating, core.StageExporting, core.StageComplete}; !reflect.DeepEqual(stages, want) || done.Stage != core.StageComplete {
		t.Fatalf("stages %v, job %+v", stages, done)
	}
	if got := runCount(t, whisperRuns); got != 1 {
		t.Fatalf("whisper-cli ran %d times", got)
	}

	if err := os.Remove(video); err != nil {
		t.Fatal(err)
	}
	if _, gone := runRecording(tools, done); gone.Stage != core.StageFailed || !errors.Is(gone.Err, os.ErrNotExist) {
		t.Fatalf("missing file %+v", gone)
	}
}
