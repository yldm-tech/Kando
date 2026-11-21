package media

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"kando/packages/core"
	"kando/packages/track"
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

func (p *redoPipeline) Length(context.Context, string) time.Duration {
	p.t.Error("translating again measured the video")
	return 0
}

func (p *redoPipeline) Hear(context.Context, string, time.Duration, time.Duration, func(core.Segment)) (string, error) {
	p.t.Error("translating again heard the video")
	return "", errors.New("no hearing")
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
	return exportSubtitles(video, original, translated)
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
	subtitles, err := exportSubtitles(video, original, translated)
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

// Translating only the missing lines sends the translator nothing but the lines the film's subtitles left untranslated, and keeps the translations it already had.
func TestMakeTranslatesOnlyTheMissingLines(t *testing.T) {
	job := doneFilm(t, "你好。", "", "再见。")
	if job.Untranslated != 1 {
		t.Fatalf("the film has %d untranslated lines", job.Untranslated)
	}
	job.Redo = core.RedoMissing
	p := &redoPipeline{t: t}
	final, stages := remake(p, job, "简体中文")
	if want := []core.Stage{core.StageTranslating, core.StageComplete}; !reflect.DeepEqual(stages, want) {
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
func TestMakeCountsTheMissingLinesThatFailAgain(t *testing.T) {
	job := doneFilm(t, "", "", "再见。")
	job.Redo = core.RedoMissing
	p := &redoPipeline{t: t, untranslated: "How are you?"}
	final, _ := remake(p, job, "简体中文")
	if final.Stage != core.StageComplete || final.Untranslated != 1 {
		t.Fatalf("final %+v", final)
	}
	if want := []string{"简体中文:Hello there.\nHello there.", "How are you?", "再见。\nGoodbye."}; !reflect.DeepEqual(bilingualTexts(t, final), want) {
		t.Fatalf("bilingual %q", bilingualTexts(t, final))
	}
}

// Subtitles that no longer match the transcript cue for cue, as ones edited by hand, do not say which lines are missing: translating only those fails with ErrSubtitlesChanged, sends the translator nothing and leaves the edited file as it is, rather than translating every line over it.
func TestMakeKeepsSubtitlesThatNoLongerMatch(t *testing.T) {
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
		final, _ := remake(p, job, "简体中文")
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
func TestMakeTranslatesEveryLineWhenTheBilingualSubtitlesAreGone(t *testing.T) {
	job := doneFilm(t, "你好。", "", "再见。")
	job.Redo = core.RedoMissing
	if err := os.Remove(job.SubtitlePath); err != nil {
		t.Fatal(err)
	}
	p := &redoPipeline{t: t}
	if final, _ := remake(p, job, "简体中文"); final.Stage != core.StageComplete {
		t.Fatalf("final %+v", final)
	}
	if want := []string{"Hello there.", "How are you?", "Goodbye."}; !reflect.DeepEqual(p.sent, want) {
		t.Fatalf("sent %q", p.sent)
	}
}

// A film whose transcript is gone cannot be translated again: the job fails with ErrNoTranscript, still saying what it was redoing, so the app can put the film back as it was.
func TestMakeTranslatingAgainWithoutATranscript(t *testing.T) {
	for _, redo := range []core.Redo{core.RedoTranslation, core.RedoMissing} {
		job := doneFilm(t, "你好。", "", "再见。")
		job.Redo = redo
		if err := os.Remove(strings.TrimSuffix(job.VideoPath, ".mp4") + ".srt"); err != nil {
			t.Fatal(err)
		}
		final, _ := remake(&redoPipeline{t: t}, job, "简体中文")
		if final.Stage != core.StageFailed || !errors.Is(final.Err, ErrNoTranscript) || final.Redo != redo {
			t.Fatalf("%s: final %+v", redo, final)
		}
	}
}

// modelFilm is makeFilm heard by a speech model, as Tools names the one it hears by (hearer).
type modelFilm struct {
	*makeFilm
	model string
}

func (f modelFilm) hearing(context.Context) (string, string) { return f.model, "" }

// redoOf is done, a film made, queued to be processed again entirely, as the app's 重新处理 queues it.
func redoOf(done core.Job) core.Job {
	done.Redo, done.RedoLanguage, done.Stage, done.Err = core.RedoAll, done.Language, core.StageQueued, nil
	return done
}

// Processing a done film again entirely makes it anew beside what it shows, which stays as it was, files and job, until the new one is done: one that fails leaves the film as it was, and run again goes on with what it heard, then shows the film anew. Its final proofread's changes for review replace the film's only then.
func TestMakeProcessingAgainKeepsTheFilmUntilItIsDone(t *testing.T) {
	job, video := makeJob(t)
	job.SpeechLanguage = "ja"
	ctx := WithReview(context.Background(), core.ProofAutoAll)
	f := &makeFilm{length: 30 * time.Second, language: "ja"}
	film := makeFilmWith(t, ctx, f, job, 0)
	if film.Stage != core.StageComplete {
		t.Fatalf("film ended %s: %v", film.Stage, film.Err)
	}
	bilingual := strings.TrimSuffix(video, ".mkv") + ".bilingual.srt"
	read := func(path string) string {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	srt, both, proofs := read(TranscriptOf(video)), read(bilingual), read(ProofsPath(video))

	f.say = func(at time.Duration) string { return "anew " + at.String() }
	f.translate = func(string) (string, error) { return "", ErrOffline }
	heard := len(f.runs)
	failed := makeFilmWith(t, ctx, f, redoOf(film), 0)
	if failed.Stage != core.StageFailed || failed.Redo != core.RedoAll || !strings.HasPrefix(failed.Err.Error(), "translate: ") {
		t.Fatalf("failed %+v", failed)
	}
	if failed.SubtitlePath != film.SubtitlePath || !failed.CompletedAt.Equal(film.CompletedAt) || failed.Language != film.Language || failed.SpokenLanguage != film.SpokenLanguage {
		t.Fatalf("the failed job no longer describes the film: %+v", failed)
	}
	if read(TranscriptOf(video)) != srt || read(bilingual) != both || read(ProofsPath(video)) != proofs {
		t.Fatal("a failed redo changed the film's files")
	}
	if doc, _ := track.Read(video); doc.View()[0].Original.Text != "line 0s" {
		t.Fatalf("the film shows %+v", doc.View()[0])
	}

	// Run again where its subtitles cannot be written, it fails once its final proofread is made, and the film is still as it was, its changes for review too.
	f.translate = nil
	f.reviewed = nil
	reheard := len(f.runs) - heard
	if err := os.Rename(bilingual, bilingual+".kept"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(bilingual, 0o755); err != nil {
		t.Fatal(err)
	}
	if stuck := makeFilmWith(t, ctx, f, failed, 0); stuck.Stage != core.StageFailed || len(f.reviewed) == 0 || read(ProofsPath(video)) != proofs {
		t.Fatalf("stuck %+v after %d final proofreads", stuck, len(f.reviewed))
	}
	if err := os.Remove(bilingual); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(bilingual+".kept", bilingual); err != nil {
		t.Fatal(err)
	}
	done := makeFilmWith(t, ctx, f, failed, 0)
	if done.Stage != core.StageComplete || done.Redo != "" || done.RedoLanguage != "" || !done.CompletedAt.After(film.CompletedAt) {
		t.Fatalf("done %+v", done)
	}
	if again := len(f.runs) - heard - reheard; again != 0 {
		t.Fatalf("the redo run again heard the film %d more times", again)
	}
	if got := read(bilingual); !strings.Contains(got, "译:anew 0s\nanew 0s") || strings.Contains(got, "line 0s") {
		t.Fatalf("bilingual %q", got)
	}
	if got := read(ProofsPath(video)); !strings.Contains(got, "anew 0s") || strings.Contains(got, "line 0s") {
		t.Fatalf("proofs %q", got)
	}
}

// A redo is the film heard anew by the speech model set now: one heard by another model is begun again, not gone on with, and the language is heard anew rather than taken from what the film was heard in before.
func TestMakeRedoHearsTheFilmAnew(t *testing.T) {
	job, video := makeJob(t)
	f := &makeFilm{length: 30 * time.Second, language: "en"}
	film, _ := makeFilmOf(t, modelFilm{f, "small"}, job, 0)
	if film.SpokenLanguage != "en" {
		t.Fatalf("film %+v", film)
	}
	f.language, f.translate = "ko", func(string) (string, error) { return "", ErrOffline }
	failed, _ := makeFilmOf(t, modelFilm{f, "small"}, redoOf(film), 0)
	if failed.Stage != core.StageFailed || failed.SpokenLanguage != "en" {
		t.Fatalf("failed %+v", failed)
	}
	if got := f.langs[len(f.langs)-1]; got != "" {
		t.Fatalf("the redo heard the film in %q, not anew", got)
	}
	f.translate = nil
	runs := len(f.runs)
	done, _ := makeFilmOf(t, modelFilm{f, "large"}, failed, 0)
	if done.Stage != core.StageComplete || done.SpokenLanguage != "ko" || len(f.runs) != runs+1 {
		t.Fatalf("done %+v after %d runs", done, len(f.runs)-runs)
	}
	doc, _ := track.Read(video)
	gens := 0
	for _, e := range doc.Events {
		if e.Kind == track.KindGen {
			gens++
		}
	}
	if gens != 3 {
		t.Fatalf("%d generations, want the film, the redo by the small model and the one by the large", gens)
	}
}

// remake makes job with p as the queue does, into language, returning the job it ended with and the stages it went through.
func remake(p Pipeline, job core.Job, language string) (core.Job, []core.Stage) {
	var stages []core.Stage
	done := Make(context.Background(), p, job, language, "mac", nil, func(j core.Job) { stages = enteredStage(stages, j) })
	return done, stages
}

// Translating a film from before the document again translates every line of its transcript into the language asked for, without downloading or hearing the film, and keeps its subtitles where they were.
func TestMakeTranslatesAFilmAgainFromItsFiles(t *testing.T) {
	job := doneFilm(t, "你好。", "", "再见。")
	job.Redo = core.RedoTranslation
	p := &redoPipeline{t: t}
	final, stages := remake(p, job, "日本語")
	if want := []core.Stage{core.StageTranslating, core.StageComplete}; !reflect.DeepEqual(stages, want) {
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

// A translation again that fails leaves the film as it was, files and job, and a film whose lines are gone cannot be translated again.
func TestMakeTranslatingAgainThatFailsKeepsTheFilm(t *testing.T) {
	job := doneFilm(t, "你好。", "", "再见。")
	job.Redo, job.RedoLanguage = core.RedoTranslation, "日本語"
	job.CompletedAt = time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	before := bilingualTexts(t, job)
	final, _ := remake(&redoPipeline{t: t, translateErr: ErrOffline}, job, "日本語")
	if final.Stage != core.StageFailed || final.Redo != core.RedoTranslation || !strings.HasPrefix(final.Err.Error(), "translate: ") {
		t.Fatalf("final %+v", final)
	}
	if final.Language != "简体中文" || final.Untranslated != 1 || !final.CompletedAt.Equal(job.CompletedAt) {
		t.Fatalf("the failed job no longer describes the film: %+v", final)
	}
	if after := bilingualTexts(t, final); !reflect.DeepEqual(after, before) {
		t.Fatalf("subtitles %q, were %q", after, before)
	}

	gone := doneFilm(t, "你好。", "", "再见。")
	gone.Redo = core.RedoTranslation
	if err := os.Remove(strings.TrimSuffix(gone.VideoPath, ".mp4") + ".srt"); err != nil {
		t.Fatal(err)
	}
	if final, _ := remake(&redoPipeline{t: t}, gone, "简体中文"); final.Stage != core.StageFailed || !errors.Is(final.Err, ErrNoTranscript) {
		t.Fatalf("final %+v", final)
	}
}

// A film made a part at a time translated again keeps its lines as they read, proofread already, and translates them alone: the film is not heard or proofread again.
func TestMakeTranslatesAMadeFilmAgain(t *testing.T) {
	job, video := makeJob(t)
	job.SpeechLanguage = "ja"
	f := &makeFilm{length: 70 * time.Second, language: "ja", say: sayFoo, proofread: correctFoo(1)}
	film, _ := makeFilmOf(t, f, job, 0)
	if film.Stage != core.StageComplete {
		t.Fatalf("film %+v", film)
	}
	runs, proofreads, batches := len(f.runs), f.proofreads, len(f.translated)
	// The translation goes after the first cell, then comes back: run again, the translation again goes on from the cell it stopped at.
	gone := false
	f.translate = func(s string) (string, error) {
		if gone && len(f.translated) == batches+2 {
			return "", ErrOffline
		}
		return "ja:" + s, nil
	}
	gone = true
	again := film
	again.Redo, again.RedoLanguage, again.Stage = core.RedoTranslation, "日本語", core.StageQueued
	if failed, _ := makeFilmOf(t, f, again, 0); failed.Stage != core.StageFailed {
		t.Fatalf("failed %+v", failed)
	}
	gone = false
	done, _ := makeFilmOf(t, f, again, 0)
	if sent := len(f.translated) - batches; sent != 4 {
		t.Fatalf("%d drafts sent, want the 3 cells' and the one that failed", sent)
	}
	if done.Stage != core.StageComplete || done.Redo != "" || done.SpokenLanguage != "ja" {
		t.Fatalf("done %+v", done)
	}
	if len(f.runs) != runs || f.proofreads != proofreads {
		t.Fatalf("translating again heard the film %d times and proofread it %d times", len(f.runs)-runs, f.proofreads-proofreads)
	}
	doc, _ := track.Read(video)
	lines := doc.View()
	if len(lines) != 7 || lines[0].Original.Text != "say bar now" || lines[0].Translation.Text != "ja:say bar now" {
		t.Fatalf("lines %+v", lines)
	}
}

// A line whose translation was taken out of the film's subtitles by hand is missing, and translating the missing lines translates it, shown over what the file said.
func TestMakeFillsALineTakenOutOfTheFile(t *testing.T) {
	job, video := makeJob(t)
	job.SpeechLanguage = "ja"
	f := &makeFilm{length: 30 * time.Second, language: "ja"}
	film, _ := makeFilmOf(t, f, job, 0)
	bilingual := strings.TrimSuffix(video, ".mkv") + ".bilingual.srt"
	data, err := os.ReadFile(bilingual)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(data), "译:line 10s\n", "", 1)
	if err := os.WriteFile(bilingual, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	f.translate = func(s string) (string, error) { return "补:" + s, nil }
	again := film
	again.Redo, again.Stage = core.RedoMissing, core.StageQueued
	done, _ := makeFilmOf(t, f, again, 0)
	if done.Stage != core.StageComplete || done.Untranslated != 0 {
		t.Fatalf("done %+v", done)
	}
	doc, _ := track.Read(video)
	if lines := doc.View(); len(lines) != 3 || lines[0].Translation.Text != "译:line 0s" || lines[1].Translation.Text != "补:line 10s" {
		t.Fatalf("lines %+v", lines)
	}
	if got := f.translated[len(f.translated)-1]; !reflect.DeepEqual(got, []string{"line 10s"}) {
		t.Fatalf("sent %q", got)
	}
}

// Translating the missing lines keeps each batch as it is translated: one that fails partway keeps the lines it filled, and says how many are left.
func TestMakeFillingThatFailsKeepsWhatItFilled(t *testing.T) {
	job, video := makeJob(t)
	job.SpeechLanguage = "ja"
	f := &makeFilm{length: 500 * time.Second, language: "ja", translate: func(string) (string, error) { return "", errors.New("502 bad gateway") }}
	film, _ := makeFilmOf(t, f, job, 0)
	if film.Untranslated != 50 {
		t.Fatalf("film %+v", film)
	}
	// The first batch is translated; the service goes at the second.
	batches := len(f.translated)
	f.translate = func(s string) (string, error) {
		if len(f.translated) > batches+1 {
			return "", ErrOffline
		}
		return "补:" + s, nil
	}
	again := film
	again.Redo, again.Stage = core.RedoMissing, core.StageQueued
	failed, _ := makeFilmOf(t, f, again, 0)
	if failed.Stage != core.StageFailed || failed.Untranslated != 10 {
		t.Fatalf("failed %s, %d untranslated", failed.Stage, failed.Untranslated)
	}
	doc, _ := track.Read(video)
	if lines := doc.View(); lines[39].Translation.Text != "补:line 6m30s" || lines[40].Translation.Text != "" {
		t.Fatalf("lines %+v, %+v", lines[39], lines[40])
	}
}

// A film whose video was moved off this computer, its subtitles and the files beside it left in the library (the app's storage locations), is translated again and has its missing lines translated as one whose video is here: neither reads the video.
func TestMakeTranslatesAFilmWhoseVideoIsElsewhere(t *testing.T) {
	for _, redo := range []core.Redo{core.RedoTranslation, core.RedoMissing} {
		job := doneFilm(t, "你好。", "", "再见。")
		if err := os.Remove(job.VideoPath); err != nil {
			t.Fatal(err)
		}
		job.Redo = redo
		final, _ := remake(&redoPipeline{t: t}, job, "简体中文")
		if final.Stage != core.StageComplete || final.Untranslated != 0 {
			t.Fatalf("%s: %+v", redo, final)
		}
	}
}

// A redo that gives up hearing a part of the film fails rather than replace the film with one that lacks that part, and the next redo begins again rather than going on with the one that gave up.
func TestMakeRedoThatGivesUpAPartKeepsTheFilm(t *testing.T) {
	job, video := makeJob(t)
	job.SpeechLanguage = "ja"
	f := &makeFilm{length: 60 * time.Second, language: "ja"}
	film, _ := makeFilmOf(t, f, job, 0)
	if film.Stage != core.StageComplete {
		t.Fatalf("film ended %s: %v", film.Stage, film.Err)
	}
	// The speech recognition fails every run before it hears the second cell's last line, so that cell is given up.
	f.say = func(at time.Duration) string { return "anew " + at.String() }
	f.hearErr, f.failAfter = errors.New("whisper-cli: segfault"), 4
	failed, _ := makeFilmOf(t, f, redoOf(film), 0)
	if failed.Stage != core.StageFailed || !errors.Is(failed.Err, errGivenUp) || failed.Redo != core.RedoAll {
		t.Fatalf("failed %+v", failed)
	}
	doc, _ := track.Read(video)
	if lines := doc.View(); len(lines) != 6 || lines[0].Original.Text != "line 0s" || lines[5].Original.Text != "line 50s" {
		t.Fatalf("the film shows %+v, want it as it was", lines)
	}
	f.hearErr = nil
	done, _ := makeFilmOf(t, f, failed, 0)
	if done.Stage != core.StageComplete {
		t.Fatalf("done %+v", done)
	}
	doc, _ = track.Read(video)
	if lines := doc.View(); len(lines) != 6 || lines[0].Original.Text != "anew 0s" || lines[5].Original.Text != "anew 50s" {
		t.Fatalf("the film shows %+v, want it heard anew", lines)
	}
}

// A film made again from the start, not as a redo, after a translation again of it was stopped begins a generation of its own: it does not go on with the copy the translation made of the film's lines, which were never heard.
func TestMakeDoesNotGoOnWithAStoppedTranslationAgain(t *testing.T) {
	job, video := makeJob(t)
	job.SpeechLanguage = "ja"
	f := &makeFilm{length: 30 * time.Second, language: "ja"}
	film, _ := makeFilmOf(t, f, job, 0)
	if film.Stage != core.StageComplete {
		t.Fatalf("film %+v", film)
	}
	f.translate = func(string) (string, error) { return "", ErrOffline }
	again := film
	again.Redo, again.RedoLanguage, again.Stage = core.RedoTranslation, "日本語", core.StageQueued
	if failed, _ := makeFilmOf(t, f, again, 0); failed.Stage != core.StageFailed {
		t.Fatalf("failed %+v", failed)
	}
	f.translate = nil
	plain := film
	plain.Stage = core.StageQueued
	if done, _ := makeFilmOf(t, f, plain, 0); done.Stage != core.StageComplete {
		t.Fatalf("done %+v", done)
	}
	doc, _ := track.Read(video)
	for _, e := range doc.Events {
		if e.Kind == track.KindGen && e.Gen == doc.Shown() && strings.HasPrefix(e.Key, copyHearing+" ") {
			t.Fatalf("the film was made into the copy %q", e.Key)
		}
	}
}

// A film translated again is as long as its lines reach: a long line that starts before the last one and ends after it is translated too.
func TestMakeTranslatesAgainALineThatEndsLast(t *testing.T) {
	video := filepath.Join(t.TempDir(), "video.mp4")
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	lines := []core.Segment{
		{Start: 20 * time.Second, End: 100 * time.Second, Text: "A long line."},
		{Start: 30 * time.Second, End: 32 * time.Second, Text: "Short."},
	}
	if _, err := track.Append(video, "mac", track.Generation(track.Made{Source: "import", Original: track.LayerFinal, Translation: track.LayerFinal}, lines, nil)...); err != nil {
		t.Fatal(err)
	}
	job := core.Job{ID: "long", Source: core.FileURL(video), Language: "简体中文", Stage: core.StageQueued, VideoPath: video, Redo: core.RedoTranslation, RedoLanguage: "日本語"}
	final, _ := remake(&redoPipeline{t: t}, job, "日本語")
	if final.Stage != core.StageComplete || final.Untranslated != 0 {
		t.Fatalf("final %+v", final)
	}
	doc, _ := track.Read(video)
	for _, l := range doc.View() {
		if l.Translation.Text != "日本語:"+l.Original.Text {
			t.Fatalf("lines %+v", doc.View())
		}
	}
}

// Every report of a film translated again names the generation it makes, which the player shows where it is made while the film's files keep their subtitles, and the job done names none.
func TestMakeTranslatingAgainNamesItsGeneration(t *testing.T) {
	job := doneFilm(t, "你好。", "", "再见。")
	job.Redo = core.RedoTranslation
	var gens []string
	final := Make(context.Background(), &redoPipeline{t: t}, job, "日本語", "mac", nil, func(j core.Job) {
		if j.Stage != core.StageComplete {
			gens = append(gens, j.RedoGen)
		}
	})
	if final.Stage != core.StageComplete || len(gens) == 0 || gens[0] == "" || final.RedoGen != "" {
		t.Fatalf("reports named generations %q, the job done %q at %s: %v", gens, final.RedoGen, final.Stage, final.Err)
	}
	for _, g := range gens {
		if g != gens[0] {
			t.Fatalf("reports named generations %q, want one", gens)
		}
	}
}

// batchFailing is a redoPipeline whose translation of a batch starting with first fails, as one batch whose reply cannot be read does.
type batchFailing struct {
	*redoPipeline
	first string
	err   error
}

func (p batchFailing) Translate(ctx context.Context, segments []core.Segment, language string) ([]core.Segment, error) {
	if segments[0].Text == p.first {
		return nil, p.err
	}
	return p.redoPipeline.Translate(ctx, segments, language)
}

// manyMissing is a done film of n lines, "line 0" on, none of them translated.
func manyMissing(t *testing.T, n int) core.Job {
	t.Helper()
	video := filepath.Join(t.TempDir(), "Talk", "video.mp4")
	if err := os.MkdirAll(filepath.Dir(video), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	original := make([]core.Segment, n)
	for i := range original {
		original[i] = core.Segment{Start: time.Duration(i) * time.Second, End: time.Duration(i+1)*time.Second - time.Millisecond, Text: fmt.Sprintf("line %d", i)}
	}
	subtitles, err := exportSubtitles(video, original, make([]core.Segment, n))
	if err != nil {
		t.Fatal(err)
	}
	return core.Job{ID: "done", Source: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Language: "简体中文", Stage: core.StageQueued, VideoPath: video, SubtitlePath: subtitles, Untranslated: n, Redo: core.RedoMissing}
}

// A batch of missing lines that fails is left untranslated, and the batches after it are translated; the job completes saying how many are left.
func TestMakeGoesOnPastAMissingBatchThatFails(t *testing.T) {
	job := manyMissing(t, DefaultBatchSize+5)
	p := batchFailing{redoPipeline: &redoPipeline{t: t}, first: "line 0", err: errors.New("translation is not a JSON array of strings: unexpected end of JSON input")}
	final, _ := remake(p, job, "简体中文")
	if final.Stage != core.StageComplete || final.Untranslated != DefaultBatchSize || final.Redo != "" {
		t.Fatalf("final %s %v, untranslated %d", final.Stage, final.Err, final.Untranslated)
	}
	if want := []string{"line 40", "line 41", "line 42", "line 43", "line 44"}; !reflect.DeepEqual(p.sent, want) {
		t.Fatalf("sent %q", p.sent)
	}
	// A key the service refuses fails every batch: the job fails at once, as before.
	job = manyMissing(t, DefaultBatchSize+5)
	refused := batchFailing{redoPipeline: &redoPipeline{t: t}, first: "line 0", err: &statusError{code: 401, status: "401 Unauthorized"}}
	if final, _ := remake(refused, job, "简体中文"); final.Stage != core.StageFailed || len(refused.sent) != 0 {
		t.Fatalf("with a refused key: %s, sent %q", final.Stage, refused.sent)
	}
	// Every batch failing is a translation that failed.
	job = manyMissing(t, DefaultBatchSize+5)
	gone := &redoPipeline{t: t, translateErr: errors.New("translation is not a JSON array of strings")}
	if final, _ := remake(gone, job, "简体中文"); final.Stage != core.StageFailed {
		t.Fatalf("with every batch failing: %s", final.Stage)
	}
}
