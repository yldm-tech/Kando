package media

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"kando/packages/core"
)

// redoPipeline is a done film's pipeline as a job translating it again sees it: Check fails as it does once the speech model is deleted, which a job translating again does not need, so only CheckTranslate may run; Download and Transcribe fail the test, since the film is neither downloaded nor transcribed again, Translate prefixes each line it is sent with the language and remembers them, and Export writes the subtitles as Tools does. untranslated, when set, is the text Translate leaves untranslated, reporting it as Tools does.
type redoPipeline struct {
	t            *testing.T
	sent         []string
	language     string
	untranslated string
	translateErr error
}

func (p *redoPipeline) Check() error {
	return errors.New("whisper model: stat ggml-small.bin: no such file or directory")
}

func (p *redoPipeline) CheckTranslate() error { return nil }

func (p *redoPipeline) Download(context.Context, string) (string, error) {
	p.t.Error("translating again downloaded the video")
	return "", errors.New("no download")
}

func (p *redoPipeline) Transcribe(context.Context, string) ([]core.Segment, error) {
	p.t.Error("translating again transcribed the video")
	return nil, errors.New("no transcription")
}

func (p *redoPipeline) Translate(_ context.Context, segments []core.Segment, language string) ([]core.Segment, error) {
	p.language = language
	if p.translateErr != nil {
		return nil, p.translateErr
	}
	out := make([]core.Segment, len(segments))
	left := 0
	for i, s := range segments {
		p.sent = append(p.sent, s.Text)
		out[i] = s
		if s.Text == p.untranslated {
			out[i].Text = ""
			left++
			continue
		}
		out[i].Text = language + ":" + s.Text
	}
	if left > 0 {
		return out, &UntranslatedError{Lines: left, Total: len(segments), Err: errors.New("batch failed")}
	}
	return out, nil
}

func (p *redoPipeline) Export(ctx context.Context, video string, original, translated []core.Segment) (string, error) {
	return exportSubtitles(ctx, video, original, translated)
}

// doneFilm writes a done film's video and the subtitles its job exported, with the translations given line for line ("" for a line left untranslated), and returns its job.
func doneFilm(t *testing.T, translations ...string) core.Job {
	t.Helper()
	video := filepath.Join(t.TempDir(), "Talk", "video.mp4")
	if err := os.MkdirAll(filepath.Dir(video), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	originals := []string{"Hello there.", "How are you?", "Goodbye."}
	original := make([]core.Segment, len(translations))
	translated := make([]core.Segment, len(translations))
	for i, text := range translations {
		span := core.Segment{Start: time.Duration(i) * time.Second, End: time.Duration(i+1)*time.Second - time.Millisecond}
		original[i], translated[i] = span, span
		original[i].Text, translated[i].Text = originals[i], text
	}
	subtitles, err := exportSubtitles(context.Background(), video, original, translated)
	if err != nil {
		t.Fatal(err)
	}
	return core.Job{
		ID: "done", Source: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Language: "简体中文", Stage: core.StageQueued,
		VideoPath: video, SubtitlePath: subtitles, Untranslated: strings.Count(strings.Join(translations, "|")+"|", "||"),
	}
}

// enteredStage appends j's stage to stages when it is not the stage last reported, which the stages report again as they go on.
func enteredStage(stages []core.Stage, j core.Job) []core.Stage {
	if len(stages) > 0 && stages[len(stages)-1] == j.Stage {
		return stages
	}
	return append(stages, j.Stage)
}

// bilingualTexts reads the bilingual subtitles of job's video back as their cues' texts.
func bilingualTexts(t *testing.T, job core.Job) []string {
	t.Helper()
	data, err := os.ReadFile(strings.TrimSuffix(job.VideoPath, ".mp4") + ".bilingual.srt")
	if err != nil {
		t.Fatal(err)
	}
	cues, err := ParseSRT(data)
	if err != nil {
		t.Fatal(err)
	}
	texts := make([]string, len(cues))
	for i, cue := range cues {
		texts[i] = cue.Text
	}
	return texts
}

// Translating a done film again translates its transcript, every line of it, into the language asked for, without downloading or transcribing the film: the job goes from queued straight to translating, and once it is complete it no longer redoes anything.
func TestRunTranslatesAFilmAgainFromItsTranscript(t *testing.T) {
	job := doneFilm(t, "你好。", "", "再见。")
	job.Redo = core.RedoTranslation
	p := &redoPipeline{t: t}
	var stages []core.Stage
	final := Run(context.Background(), p, job, "日本語", func(j core.Job) { stages = enteredStage(stages, j) })
	if want := []core.Stage{core.StageTranslating, core.StageExporting, core.StageComplete}; !reflect.DeepEqual(stages, want) {
		t.Fatalf("stages %v, want %v; err %v", stages, want, final.Err)
	}
	if want := []string{"Hello there.", "How are you?", "Goodbye."}; !reflect.DeepEqual(p.sent, want) || p.language != "日本語" {
		t.Fatalf("sent %q in %q", p.sent, p.language)
	}
	if want := []string{"日本語:Hello there.\nHello there.", "日本語:How are you?\nHow are you?", "日本語:Goodbye.\nGoodbye."}; !reflect.DeepEqual(bilingualTexts(t, final), want) {
		t.Fatalf("bilingual %q", bilingualTexts(t, final))
	}
	if final.Redo != "" || final.RedoLanguage != "" || final.Language != "日本語" || final.Untranslated != 0 || final.CompletedAt.IsZero() || final.SubtitlePath != job.SubtitlePath {
		t.Fatalf("final %+v", final)
	}
}

// Translating only the missing lines sends the translator nothing but the lines the film's subtitles left untranslated, and keeps the translations it already had.
func TestRunTranslatesOnlyTheMissingLines(t *testing.T) {
	job := doneFilm(t, "你好。", "", "再见。")
	if job.Untranslated != 1 {
		t.Fatalf("the film has %d untranslated lines", job.Untranslated)
	}
	job.Redo = core.RedoMissing
	p := &redoPipeline{t: t}
	var stages []core.Stage
	final := Run(context.Background(), p, job, "简体中文", func(j core.Job) { stages = enteredStage(stages, j) })
	if want := []core.Stage{core.StageTranslating, core.StageExporting, core.StageComplete}; !reflect.DeepEqual(stages, want) {
		t.Fatalf("stages %v, want %v; err %v", stages, want, final.Err)
	}
	if want := []string{"How are you?"}; !reflect.DeepEqual(p.sent, want) {
		t.Fatalf("sent %q, want only the missing line", p.sent)
	}
	if want := []string{"你好。\nHello there.", "简体中文:How are you?\nHow are you?", "再见。\nGoodbye."}; !reflect.DeepEqual(bilingualTexts(t, final), want) {
		t.Fatalf("bilingual %q", bilingualTexts(t, final))
	}
	if final.Redo != "" || final.Untranslated != 0 {
		t.Fatalf("final %+v", final)
	}
}

// A missing line that fails again stays untranslated, and the job says how many still are.
func TestRunCountsTheMissingLinesThatFailAgain(t *testing.T) {
	job := doneFilm(t, "", "", "再见。")
	job.Redo = core.RedoMissing
	p := &redoPipeline{t: t, untranslated: "How are you?"}
	final := Run(context.Background(), p, job, "简体中文", func(core.Job) {})
	if final.Stage != core.StageComplete || final.Untranslated != 1 {
		t.Fatalf("final %+v", final)
	}
	if want := []string{"简体中文:Hello there.\nHello there.", "How are you?", "再见。\nGoodbye."}; !reflect.DeepEqual(bilingualTexts(t, final), want) {
		t.Fatalf("bilingual %q", bilingualTexts(t, final))
	}
}

// A translation that fails leaves the film's subtitles as they were, and the failed job still says what they are: the language, the lines left untranslated and when they were made, with the part it was redoing.
func TestRunKeepsTheSubtitlesWhenTranslatingAgainFails(t *testing.T) {
	job := doneFilm(t, "你好。", "", "再见。")
	job.Redo = core.RedoTranslation
	job.RedoLanguage = "日本語"
	job.CompletedAt = time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	before := bilingualTexts(t, job)
	final := Run(context.Background(), &redoPipeline{t: t, translateErr: errors.New("translation API returned 401")}, job, "日本語", func(core.Job) {})
	if final.Stage != core.StageFailed || final.Redo != core.RedoTranslation || !strings.HasPrefix(final.Err.Error(), "translate: ") {
		t.Fatalf("final %+v", final)
	}
	if final.Language != "简体中文" || final.Untranslated != 1 || !final.CompletedAt.Equal(job.CompletedAt) {
		t.Fatalf("the failed job no longer describes the film's subtitles: %+v", final)
	}
	if after := bilingualTexts(t, final); !reflect.DeepEqual(after, before) {
		t.Fatalf("subtitles %q, were %q", after, before)
	}
}

// Subtitles that no longer match the transcript cue for cue, as ones edited by hand, do not say which lines are missing: translating only those fails with ErrSubtitlesChanged, sends the translator nothing and leaves the edited file as it is, rather than translating every line over it.
func TestRunKeepsSubtitlesThatNoLongerMatch(t *testing.T) {
	for name, edited := range map[string]string{
		"merged":   "1\n00:00:00,000 --> 00:00:02,000\n你好。\nHello there.\n\n2\n00:00:02,000 --> 00:00:02,999\n再见。\nGoodbye.\n\n",
		"retimed":  "1\n00:00:00,500 --> 00:00:00,999\n你好。\nHello there.\n\n2\n00:00:01,000 --> 00:00:01,999\nHow are you?\n\n3\n00:00:02,000 --> 00:00:02,999\n再见。\nGoodbye.\n\n",
		"reworded": "1\n00:00:00,000 --> 00:00:00,999\n你好。\nHi there.\n\n2\n00:00:01,000 --> 00:00:01,999\nHow are you?\n\n3\n00:00:02,000 --> 00:00:02,999\n再见。\nGoodbye.\n\n",
	} {
		job := doneFilm(t, "你好。", "", "再见。")
		job.Redo = core.RedoMissing
		if err := os.WriteFile(job.SubtitlePath, []byte(edited), 0o644); err != nil {
			t.Fatal(err)
		}
		p := &redoPipeline{t: t}
		final := Run(context.Background(), p, job, "简体中文", func(core.Job) {})
		if final.Stage != core.StageFailed || !errors.Is(final.Err, ErrSubtitlesChanged) || final.Redo != core.RedoMissing {
			t.Fatalf("%s: final %+v", name, final)
		}
		if len(p.sent) != 0 {
			t.Fatalf("%s: sent %q", name, p.sent)
		}
		if data, err := os.ReadFile(job.SubtitlePath); err != nil || string(data) != edited {
			t.Fatalf("%s: the edited subtitles were rewritten: %q, %v", name, data, err)
		}
	}
}

// A film whose bilingual subtitles are gone has no translated line left: translating the missing lines translates every line.
func TestRunTranslatesEveryLineWhenTheBilingualSubtitlesAreGone(t *testing.T) {
	job := doneFilm(t, "你好。", "", "再见。")
	job.Redo = core.RedoMissing
	if err := os.Remove(job.SubtitlePath); err != nil {
		t.Fatal(err)
	}
	p := &redoPipeline{t: t}
	if final := Run(context.Background(), p, job, "简体中文", func(core.Job) {}); final.Stage != core.StageComplete {
		t.Fatalf("final %+v", final)
	}
	if want := []string{"Hello there.", "How are you?", "Goodbye."}; !reflect.DeepEqual(p.sent, want) {
		t.Fatalf("sent %q", p.sent)
	}
}

// A film whose transcript is gone cannot be translated again: the job fails with ErrNoTranscript, still saying what it was redoing, so the app can put the film back as it was.
func TestRunTranslatingAgainWithoutATranscript(t *testing.T) {
	for _, redo := range []core.Redo{core.RedoTranslation, core.RedoMissing} {
		job := doneFilm(t, "你好。", "", "再见。")
		job.Redo = redo
		if err := os.Remove(strings.TrimSuffix(job.VideoPath, ".mp4") + ".srt"); err != nil {
			t.Fatal(err)
		}
		final := Run(context.Background(), &redoPipeline{t: t}, job, "简体中文", func(core.Job) {})
		if final.Stage != core.StageFailed || !errors.Is(final.Err, ErrNoTranscript) || final.Redo != redo {
			t.Fatalf("%s: final %+v", redo, final)
		}
	}
}

// reprocessPipeline is a done film's pipeline as a job processing it again entirely sees it: every tool is there, and whisper hears the film's speech anew, as other lines than its transcript holds, counting how often it ran.
type reprocessPipeline struct {
	redoPipeline
	transcriptions int
}

func (p *reprocessPipeline) Check() error { return nil }

func (p *reprocessPipeline) Transcribe(context.Context, string) ([]core.Segment, error) {
	p.transcriptions++
	return []core.Segment{{End: time.Second, Text: "Hi."}, {Start: time.Second, End: 2 * time.Second, Text: "Bye now."}}, nil
}

// Processing a done film again entirely leaves its subtitles, the transcript and the bilingual file, as they were until it exports, so one that fails partway still describes the film, with the part it was redoing: the speech it heard is kept aside, and the run that follows translates it without transcribing again, then replaces the film's subtitles and drops what it kept aside.
func TestRunProcessingAgainKeepsTheFilmUntilItExports(t *testing.T) {
	job := doneFilm(t, "你好。", "", "再见。")
	job.Redo, job.RedoLanguage = core.RedoAll, "日本語"
	job.CompletedAt = time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	transcript := strings.TrimSuffix(job.VideoPath, ".mp4") + ".srt"
	read := func(path string) string {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	srt, bilingual := read(transcript), read(job.SubtitlePath)

	p := &reprocessPipeline{redoPipeline: redoPipeline{t: t, translateErr: errors.New("translation API returned 503")}}
	failed := Run(context.Background(), p, job, "日本語", func(core.Job) {})
	if failed.Stage != core.StageFailed || failed.Redo != core.RedoAll || failed.RedoLanguage != "日本語" || !failed.Transcribed {
		t.Fatalf("failed %+v", failed)
	}
	if failed.Language != "简体中文" || failed.Untranslated != 1 || !failed.CompletedAt.Equal(job.CompletedAt) || failed.SubtitlePath != job.SubtitlePath {
		t.Fatalf("the failed job no longer describes the film's subtitles: %+v", failed)
	}
	if read(transcript) != srt || read(job.SubtitlePath) != bilingual {
		t.Fatal("a failed run changed the film's subtitles")
	}

	p.translateErr = nil
	failed.Stage, failed.Err = core.StageQueued, nil
	var stages []core.Stage
	done := Run(context.Background(), p, failed, "日本語", func(j core.Job) { stages = enteredStage(stages, j) })
	if want := []core.Stage{core.StageTranslating, core.StageExporting, core.StageComplete}; !reflect.DeepEqual(stages, want) || p.transcriptions != 1 {
		t.Fatalf("stages %v, want %v; transcribed %d times", stages, want, p.transcriptions)
	}
	if done.Redo != "" || done.RedoLanguage != "" || done.Language != "日本語" || done.Untranslated != 0 || !done.CompletedAt.After(job.CompletedAt) {
		t.Fatalf("done %+v", done)
	}
	if want := []string{"日本語:Hi.\nHi.", "日本語:Bye now.\nBye now."}; !reflect.DeepEqual(bilingualTexts(t, done), want) {
		t.Fatalf("bilingual %q", bilingualTexts(t, done))
	}
	if saved, err := loadTranscript(transcript); err != nil || len(saved) != 2 || saved[0].Text != "Hi." {
		t.Fatalf("transcript %+v, %v", saved, err)
	}
	if _, err := os.Stat(pendingTranscriptPath(job.VideoPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the speech kept aside is still there: %v", err)
	}
}

// hearingPipeline is reprocessPipeline heard as Tools hears a video: by model, in language, and its transcription says it heard spoken.
type hearingPipeline struct {
	*reprocessPipeline
	model, language, spoken string
}

func (p hearingPipeline) hearing(context.Context) (string, string) { return p.model, p.language }

func (p hearingPipeline) Transcribe(ctx context.Context, video string) ([]core.Segment, error) {
	reportSpoken(ctx, p.spoken)
	return p.reprocessPipeline.Transcribe(ctx, video)
}

// A Process Again that fails after its transcription keeps the film's spoken language, as it keeps its subtitles, and the speech it heard aside. Run again the way the app retries it, from a done film, it reads that speech back rather than transcribing the whole film again, and once it exports the film is heard in the language its transcription heard. Run again with another speech model or in another language, it transcribes again.
func TestRunRetryingAFailedProcessAgainReadsBackItsSpeech(t *testing.T) {
	job := doneFilm(t, "你好。", "", "再见。")
	job.Redo, job.RedoLanguage, job.SpokenLanguage = core.RedoAll, "日本語", "en"
	p := hearingPipeline{reprocessPipeline: &reprocessPipeline{redoPipeline: redoPipeline{t: t, translateErr: errors.New("translation API returned 503")}}, model: "ggml-small.bin", language: "ja", spoken: "ja"}
	failed := Run(context.Background(), p, job, "日本語", func(core.Job) {})
	if failed.Stage != core.StageFailed || failed.SpokenLanguage != "en" || p.transcriptions != 1 {
		t.Fatalf("failed %+v, %d transcriptions", failed, p.transcriptions)
	}

	// The app puts a failed redo back as the done film it was and queues it again as Process Again does, with Transcribed false.
	again := func(j core.Job) core.Job {
		j.Stage, j.Err, j.Transcribed = core.StageQueued, nil, false
		return j
	}
	p.translateErr = nil
	p.spoken = ""
	done := Run(context.Background(), p, again(failed), "日本語", func(core.Job) {})
	if done.Stage != core.StageComplete || p.transcriptions != 1 || done.SpokenLanguage != "ja" {
		t.Fatalf("done %+v, %d transcriptions", done, p.transcriptions)
	}
	for _, path := range []string{pendingTranscriptPath(job.VideoPath), pendingHeardPath(job.VideoPath)} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s is still there: %v", path, err)
		}
	}

	for name, change := range map[string]func(*hearingPipeline){
		"another model":    func(p *hearingPipeline) { p.model = "ggml-large.bin" },
		"another language": func(p *hearingPipeline) { p.language = "en" },
	} {
		t.Run(name, func(t *testing.T) {
			job := doneFilm(t, "你好。", "", "再见。")
			job.Redo, job.RedoLanguage = core.RedoAll, "日本語"
			p := hearingPipeline{reprocessPipeline: &reprocessPipeline{redoPipeline: redoPipeline{t: t, translateErr: errors.New("translation API returned 503")}}, model: "ggml-small.bin", language: "ja"}
			failed := Run(context.Background(), p, job, "日本語", func(core.Job) {})
			p.translateErr = nil
			change(&p)
			if done := Run(context.Background(), p, again(failed), "日本語", func(core.Job) {}); done.Stage != core.StageComplete || p.transcriptions != 2 {
				t.Fatalf("done %+v, %d transcriptions", done, p.transcriptions)
			}
		})
	}
}
