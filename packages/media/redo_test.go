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

// redoPipeline is a done film's pipeline as a job translating it again sees it: Download and Transcribe fail the test, since the film is neither downloaded nor transcribed again, Translate prefixes each line it is sent with the language and remembers them, and Export writes the subtitles as Tools does. untranslated, when set, is the text Translate leaves untranslated, reporting it as Tools does.
type redoPipeline struct {
	t            *testing.T
	sent         []string
	language     string
	untranslated string
	translateErr error
}

func (p *redoPipeline) Check() error { return nil }

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
	if final.Redo != "" || final.Untranslated != 0 || final.CompletedAt.IsZero() || final.SubtitlePath != job.SubtitlePath {
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

// A translation that fails leaves the film's subtitles as they were, and the failed job still redoes only its translation when it is retried.
func TestRunKeepsTheSubtitlesWhenTranslatingAgainFails(t *testing.T) {
	job := doneFilm(t, "你好。", "", "再见。")
	job.Redo = core.RedoMissing
	before := bilingualTexts(t, job)
	final := Run(context.Background(), &redoPipeline{t: t, translateErr: errors.New("translation API returned 401")}, job, "简体中文", func(core.Job) {})
	if final.Stage != core.StageFailed || final.Redo != core.RedoMissing || !strings.HasPrefix(final.Err.Error(), "translate: ") {
		t.Fatalf("final %+v", final)
	}
	if after := bilingualTexts(t, final); !reflect.DeepEqual(after, before) {
		t.Fatalf("subtitles %q, were %q", after, before)
	}
}

// Subtitles that no longer match the transcript cue for cue, as ones edited by hand, do not say which lines are missing: every line is translated.
func TestRunTranslatesEveryLineWhenTheSubtitlesDoNotMatch(t *testing.T) {
	job := doneFilm(t, "你好。", "", "再见。")
	job.Redo = core.RedoMissing
	if err := os.WriteFile(job.SubtitlePath, []byte("1\n00:00:00,000 --> 00:00:02,000\n你好。\nHello there.\n\n"), 0o644); err != nil {
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

// A film whose transcript is gone cannot be translated again: the job fails with ErrNoTranscript and no longer redoes only the translation, so its retry processes the film entirely.
func TestRunTranslatingAgainWithoutATranscript(t *testing.T) {
	for _, redo := range []core.Redo{core.RedoTranslation, core.RedoMissing} {
		job := doneFilm(t, "你好。", "", "再见。")
		job.Redo = redo
		if err := os.Remove(strings.TrimSuffix(job.VideoPath, ".mp4") + ".srt"); err != nil {
			t.Fatal(err)
		}
		final := Run(context.Background(), &redoPipeline{t: t}, job, "简体中文", func(core.Job) {})
		if final.Stage != core.StageFailed || !errors.Is(final.Err, ErrNoTranscript) || final.Redo != "" {
			t.Fatalf("%s: final %+v", redo, final)
		}
	}
}
