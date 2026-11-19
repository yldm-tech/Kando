package media

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kando/packages/core"
	"kando/packages/track"
)

// makeFilm is a film a fake speech recognition hears: a line every ten seconds, "line N" at N×10 s, for length.
type makeFilm struct {
	length    time.Duration
	language  string
	hearErr   error
	failAfter int // hear fails after handing on this many lines in a run, when hearErr is set and it is > 0
	// translate translates a line; nil puts "译:" before it.
	translate func(string) (string, error)
	// say is what is said at a time, "line <time>" when nil; proofread corrects a batch of lines, the nth sent, nil leaving them as they are.
	say        func(time.Duration) string
	proofread  func(n int, lines []string) []string
	proofreads int
	reviewed   []int

	// linkDir is where a link downloads to; videoGate, when set, holds its video back until closed, and videoErr fails it.
	linkDir   string
	videoGate chan struct{}
	videoErr  error
	heardFrom []string

	// stall, when set, is called by the first run once it has heard past its first cell, and the run then waits until it is stopped, as a run far from a jump does.
	stall func()

	mu         sync.Mutex
	runs       [][2]time.Duration
	langs      []string
	translated [][]string
	earlier    [][]string
}

func (f *makeFilm) Check() error { return nil }
func (f *makeFilm) Download(ctx context.Context, source string) (string, error) {
	if path, ok := localVideoPath(source); ok {
		return path, nil
	}
	if f.linkDir == "" {
		return "", errors.New("not a link")
	}
	// A link downloads into a folder of its own: its sound alone as video.m4a, its video as video.mp4, once videoGate lets it.
	name := "video.mp4"
	if q, _ := ctx.Value(qualityKey{}).(core.Quality); q == core.QualitySubtitles {
		name = "video.m4a"
	} else {
		if f.videoGate != nil {
			select {
			case <-f.videoGate:
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		if f.videoErr != nil {
			return "", f.videoErr
		}
	}
	path := filepath.Join(f.linkDir, name)
	return path, os.WriteFile(path, nil, 0o644)
}
func (f *makeFilm) Transcribe(context.Context, string) ([]core.Segment, error) {
	return nil, errors.New("Make hears the film a part at a time")
}
func (f *makeFilm) Export(context.Context, string, []core.Segment, []core.Segment) (string, error) {
	return "", errors.New("Make writes the film's subtitles itself")
}
func (f *makeFilm) Length(context.Context, string) time.Duration { return f.length }

func (f *makeFilm) Hear(ctx context.Context, video string, from, to time.Duration, heard func(core.Segment)) (string, error) {
	f.mu.Lock()
	f.runs = append(f.runs, [2]time.Duration{from, to})
	f.heardFrom = append(f.heardFrom, filepath.Base(video))
	f.langs = append(f.langs, speechLanguageOf(ctx))
	f.mu.Unlock()
	n := 0
	for at := time.Duration(0); at < f.length; at += 10 * time.Second {
		if at < from || at >= to {
			continue
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if f.hearErr != nil && n >= f.failAfter {
			return "", f.hearErr
		}
		text := "line " + at.String()
		if f.say != nil {
			text = f.say(at)
		}
		heard(core.Segment{Start: at, End: at + 2*time.Second, Text: text})
		n++
		if f.stall != nil && at >= from+track.Cell+runPad {
			stall := f.stall
			f.stall = nil
			stall()
			<-ctx.Done()
			return "", ctx.Err()
		}
	}
	if f.hearErr != nil {
		return "", f.hearErr
	}
	return f.language, nil
}

// Review suggests making each translation of a line said at a whole minute read "终:" before it, a fluency change, and records the lines it read.
func (f *makeFilm) Review(_ context.Context, original, translated []core.Segment, _, _ string) ([]core.Proof, error) {
	f.mu.Lock()
	f.reviewed = append(f.reviewed, len(original))
	f.mu.Unlock()
	var out []core.Proof
	for i, s := range translated {
		if s.Text != "" && s.Start%time.Minute == 0 && !strings.HasPrefix(s.Text, "终:") {
			out = append(out, core.Proof{Span: core.Span{Start: s.Start, End: s.End}, Kind: core.ProofFluency, Old: s.Text, New: "终:" + s.Text, Why: "reads better"})
		}
		_ = original[i]
	}
	return out, nil
}

func (f *makeFilm) Proofread(_ context.Context, segments []core.Segment, _ string) ([]core.Segment, error) {
	f.mu.Lock()
	f.proofreads++
	n := f.proofreads
	f.mu.Unlock()
	if f.proofread == nil {
		return segments, nil
	}
	texts := make([]string, len(segments))
	for i, s := range segments {
		texts[i] = s.Text
	}
	fixed := f.proofread(n, texts)
	out := slices.Clone(segments)
	for i := range out {
		out[i].Text = fixed[i]
	}
	return out, nil
}

func (f *makeFilm) Translate(ctx context.Context, segments []core.Segment, _ string) ([]core.Segment, error) {
	f.mu.Lock()
	texts := make([]string, len(segments))
	for i, s := range segments {
		texts[i] = s.Text
	}
	f.translated = append(f.translated, texts)
	f.earlier = append(f.earlier, slices.Clone(earlierOf(ctx)))
	f.mu.Unlock()
	out := make([]core.Segment, len(segments))
	for i, s := range segments {
		s.Text = "译:" + s.Text
		if f.translate != nil {
			text, err := f.translate(texts[i])
			if err != nil {
				return nil, err
			}
			s.Text = text
		}
		out[i] = s
	}
	return out, nil
}

func makeJob(t *testing.T) (core.Job, string) {
	t.Helper()
	video := filepath.Join(t.TempDir(), "Episode 1.mkv")
	if err := os.WriteFile(video, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	job, err := core.NewJob(core.FileURL(video))
	if err != nil {
		t.Fatal(err)
	}
	job.VideoPath = video
	return job, video
}

// makeFilmOf runs Make on job with f, watched from at, and returns the job it ended with and every job it reported.
func makeFilmOf(t *testing.T, f Pipeline, job core.Job, at time.Duration) (core.Job, []core.Job) {
	t.Helper()
	var reports []core.Job
	done := Make(context.Background(), f, job, "简体中文", "mac", From(at), func(j core.Job) { reports = append(reports, j) })
	return done, reports
}

func TestMakeMakesAFilmAPartAtATime(t *testing.T) {
	job, video := makeJob(t)
	f := &makeFilm{length: 95 * time.Second, language: "ja"}
	done, reports := makeFilmOf(t, f, job, 0)
	if done.Stage != core.StageComplete || done.Err != nil {
		t.Fatalf("job ended %s: %v", done.Stage, done.Err)
	}
	bilingual := filepath.Join(filepath.Dir(video), "Episode 1.bilingual.srt")
	if done.SubtitlePath != bilingual || done.SpokenLanguage != "ja" || done.Untranslated != 0 || done.TranslationErr != "" || !done.Transcribed {
		t.Fatalf("job = %+v", done)
	}
	data, err := os.ReadFile(bilingual)
	if err != nil {
		t.Fatal(err)
	}
	cues, _ := ParseSRT(data)
	if len(cues) != 10 || cues[0].Text != "译:line 0s\nline 0s" || cues[9].Text != "译:line 1m30s\nline 1m30s" {
		t.Fatalf("bilingual subtitles = %+v", cues)
	}
	if transcript, _ := os.ReadFile(filepath.Join(filepath.Dir(video), "Episode 1.srt")); !strings.Contains(string(transcript), "line 1m30s") || strings.Contains(string(transcript), "译") {
		t.Fatalf("transcript = %q", transcript)
	}
	// The first run hears a cell alone to find the language; every run after is told it.
	if f.runs[0] != [2]time.Duration{0, 45 * time.Second} || f.langs[0] != "" {
		t.Fatalf("first run %v in %q, want the first cell alone, padded, in no language", f.runs[0], f.langs[0])
	}
	if f.runs[1] != [2]time.Duration{15 * time.Second, 135 * time.Second} || f.langs[1] != "ja" {
		t.Fatalf("second run %v in %q, want the rest of the film in ja", f.runs[1], f.langs[1])
	}
	// Each cell's draft goes with the lines said before it.
	if len(f.translated) != 4 || !slices.Equal(f.translated[1], []string{"line 30s", "line 40s", "line 50s"}) || !slices.Equal(f.earlier[1], []string{"line 0s", "line 10s", "line 20s"}) {
		t.Fatalf("drafts %q with %q", f.translated, f.earlier)
	}
	// The film shows its bilingual subtitles before it is done.
	if !slices.ContainsFunc(reports, func(j core.Job) bool { return j.SubtitlePath == bilingual && j.Stage != core.StageComplete }) {
		t.Fatal("the subtitles were not shown before the film was done")
	}
	doc, err := track.Read(video)
	if err != nil {
		t.Fatal(err)
	}
	if gen := doc.Shown(); !doc.Done(gen) || len(doc.View()) != 10 {
		t.Fatalf("document shows %d lines of %q, done %v", len(doc.View()), gen, doc.Done(gen))
	}
	// Reading the film back takes nothing in: the files are what the document wrote out.
	read, err := ReadTrack(video, bilingual, filepath.Join(filepath.Dir(video), "Episode 1.srt"))
	if err != nil || len(read.Events) != len(doc.Events) {
		t.Fatalf("reading took in %d records (%v)", len(read.Events)-len(doc.Events), err)
	}
}

func TestMakeStartsWhereTheFilmIsWatched(t *testing.T) {
	job, _ := makeJob(t)
	job.SpeechLanguage = "ja"
	f := &makeFilm{length: 200 * time.Second, language: "ja"}
	done, _ := makeFilmOf(t, f, job, 125*time.Second)
	if done.Stage != core.StageComplete {
		t.Fatalf("job ended %s: %v", done.Stage, done.Err)
	}
	// Told the language, the first run starts at the cell the playhead is in, 120 s, and hears on to the end; then the start of the film.
	if f.runs[0] != [2]time.Duration{105 * time.Second, 225 * time.Second} || f.runs[1] != [2]time.Duration{0, 135 * time.Second} {
		t.Fatalf("runs %v", f.runs)
	}
	if !slices.Equal(f.translated[0], []string{"line 2m0s", "line 2m10s", "line 2m20s"}) {
		t.Fatalf("first draft %q, want the cell the playhead is in", f.translated[0])
	}
}

func TestMakeGoesOnWhereAnEarlierRunStopped(t *testing.T) {
	job, video := makeJob(t)
	job.SpeechLanguage = "ja"
	f := &makeFilm{length: 60 * time.Second, language: "ja"}
	// An earlier run heard and drafted the first cell, then the app quit.
	gen := track.NewID()
	events := []track.Event{{Kind: track.KindGen, Gen: gen, Source: makeSource}}
	for i, at := range []int64{0, 10_000, 20_000} {
		id := fmt.Sprintf("e%d", i)
		events = append(events, track.Event{Kind: track.KindLine, Gen: gen, Line: id, Start: at, End: at + 2000}, track.Event{Kind: track.KindRev, Line: id, Side: track.Original, Layer: track.LayerHeard, Text: "earlier"})
	}
	events = append(events, track.Unit{Work: track.WorkHear, Gen: gen, Cell: 0, Key: "ja"}.Made())
	if _, err := track.Append(video, "mac", events...); err != nil {
		t.Fatal(err)
	}
	done, _ := makeFilmOf(t, f, job, 0)
	if done.Stage != core.StageComplete {
		t.Fatalf("job ended %s: %v", done.Stage, done.Err)
	}
	if len(f.runs) != 1 || f.runs[0][0] != 15*time.Second {
		t.Fatalf("runs %v, want only the second cell heard", f.runs)
	}
	doc, _ := track.Read(video)
	// The cell heard before keeps its lines, not heard again, and each of the next shows once.
	lines := doc.View()
	if len(lines) != 6 || lines[2].Original.Text != "earlier" || lines[2].Translation.Text != "译:earlier" || lines[3].Original.Text != "line 30s" {
		t.Fatalf("lines %+v", lines)
	}
}

// A film heard again in another language, as after the user named the language it is spoken in, holds the lines heard in it in place of those heard before, not both.
func TestACellHeardAgainHoldsItsNewLinesAlone(t *testing.T) {
	job, video := makeJob(t)
	job.SpeechLanguage = "en"
	f := &makeFilm{length: 60 * time.Second}
	if done, _ := makeFilmOf(t, f, job, 0); done.Stage != core.StageComplete {
		t.Fatalf("job ended %s: %v", done.Stage, done.Err)
	}
	// The film is made again, as a first making stopped is, but heard in Japanese.
	doc, _ := track.Read(video)
	var unfinished []track.Event
	for _, e := range doc.Events {
		if e.Kind != track.KindDone && e.Kind != track.KindHead {
			unfinished = append(unfinished, e)
		}
	}
	other := filepath.Join(filepath.Dir(video), "Episode 2.mkv")
	if err := os.WriteFile(other, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := track.Append(other, "mac", unfinished...); err != nil {
		t.Fatal(err)
	}
	job.VideoPath, job.Source, job.SpeechLanguage = other, core.FileURL(other), "ja"
	f.say = func(at time.Duration) string { return "ja " + at.String() }
	if done, _ := makeFilmOf(t, f, job, 0); done.Stage != core.StageComplete {
		t.Fatalf("job ended %s: %v", done.Stage, done.Err)
	}
	doc, _ = track.Read(other)
	lines := doc.View()
	if len(lines) != 6 || lines[0].Original.Text != "ja 0s" || lines[5].Original.Text != "ja 50s" {
		t.Fatalf("lines %+v", lines)
	}
}

// A film whose subtitles cannot be written out fails, and is not recorded done: its document still says it has lines to make.
func TestMakeFailsWhenItCannotWriteTheSubtitles(t *testing.T) {
	job, video := makeJob(t)
	job.SpeechLanguage = "ja"
	if err := os.Mkdir(strings.TrimSuffix(video, ".mkv")+".bilingual.srt", 0o755); err != nil {
		t.Fatal(err)
	}
	done, _ := makeFilmOf(t, &makeFilm{length: 60 * time.Second, language: "ja"}, job, 0)
	if done.Stage != core.StageFailed || !strings.HasPrefix(done.Err.Error(), "export: ") {
		t.Fatalf("job ended %s: %v", done.Stage, done.Err)
	}
	doc, _ := track.Read(video)
	if doc.Done(doc.Shown()) {
		t.Fatal("a film whose subtitles were not written is recorded done")
	}
}

// A film whose language the speech recognition does not tell, stopped once heard, is translated when run again: its cells, heard in no known language, are not heard again, and its lines are drafted.
func TestMakeGoesOnWithAFilmOfNoLanguageTold(t *testing.T) {
	for _, redo := range []core.Redo{"", core.RedoAll} {
		job, video := makeJob(t)
		ctx, stop := context.WithCancel(context.Background())
		f := &makeFilm{length: 30 * time.Second, translate: func(string) (string, error) { stop(); return "", context.Canceled }}
		if redo != "" {
			film, _ := makeFilmOf(t, &makeFilm{length: 30 * time.Second}, job, 0)
			job = film
			job.Redo, job.Stage = redo, core.StageQueued
		}
		if stopped := makeFilmWith(t, ctx, f, job, 0); stopped.Stage != core.StageFailed {
			t.Fatalf("%q: stopped %+v", redo, stopped)
		}
		runs := len(f.runs)
		f.translate = func(s string) (string, error) { return "又:" + s, nil }
		if done := makeFilmWith(t, context.Background(), f, job, 0); done.Stage != core.StageComplete || done.Untranslated != 0 || len(f.runs) != runs {
			t.Fatalf("%q: done %+v after %d more runs", redo, done, len(f.runs)-runs)
		}
		if doc, _ := track.Read(video); doc.View()[0].Translation.Text != "又:line 0s" {
			t.Fatalf("%q: lines %+v", redo, doc.View())
		}
	}
}

func TestMakeWithoutATranslationLeavesTheFilmUntranslated(t *testing.T) {
	job, _ := makeJob(t)
	job.SpeechLanguage = "ja"
	f := &makeFilm{length: 150 * time.Second, language: "ja", translate: func(string) (string, error) { return "", errors.New("502 bad gateway") }}
	done, _ := makeFilmOf(t, f, job, 0)
	if done.Stage != core.StageComplete || done.TranslationErr == "" || done.Untranslated != 15 {
		t.Fatalf("job = %s %q untranslated %d", done.Stage, done.TranslationErr, done.Untranslated)
	}
	// A service failing cell after cell is given up on after three cells, rather than tried for every cell three times.
	if len(f.translated) != maxFailedBatches {
		t.Fatalf("%d drafts sent, want %d", len(f.translated), maxFailedBatches)
	}
}

func TestMakeFailsWhenTheSpeechRecognitionCannotRun(t *testing.T) {
	job, _ := makeJob(t)
	job.SpeechLanguage = "ja"
	f := &makeFilm{length: 60 * time.Second, hearErr: errors.New("whisper-cli: failed to load model")}
	done, _ := makeFilmOf(t, f, job, 0)
	if done.Stage != core.StageFailed || done.Err == nil || !strings.Contains(done.Err.Error(), "failed to load model") {
		t.Fatalf("job = %s %v", done.Stage, done.Err)
	}
	if len(f.runs) != 1 {
		t.Fatalf("%d runs, want the job to fail at the first", len(f.runs))
	}
}

func TestMakeGivesUpACellThatKeepsFailing(t *testing.T) {
	job, video := makeJob(t)
	job.SpeechLanguage = "ja"
	// The speech recognition hears a line, then fails, every run.
	f := &makeFilm{length: 30 * time.Second, language: "ja", hearErr: errors.New("whisper-cli: segfault"), failAfter: 1}
	done, _ := makeFilmOf(t, f, job, 0)
	if len(f.runs) != track.Attempts {
		t.Fatalf("%d runs, want the cell tried %d times", len(f.runs), track.Attempts)
	}
	// Nothing was heard of the film, so it fails as a film without speech; the cell is given up, not tried for ever.
	if done.Stage != core.StageFailed || !errors.Is(done.Err, ErrNoSpeech) {
		t.Fatalf("job = %s %v", done.Stage, done.Err)
	}
	doc, _ := track.Read(video)
	if p := doc.Plan(track.Making{Gen: doc.Shown(), Length: f.length, Hearing: "ja", Language: "简体中文"}); len(p.GivenUp) != 1 {
		t.Fatalf("plan %+v, want the cell given up", p)
	}
}

// jumper is a Watcher whose playhead the test moves.
type jumper struct {
	mu     sync.Mutex
	at     time.Duration
	jumped chan struct{}
}

func (j *jumper) Playhead() time.Duration {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.at
}

func (j *jumper) Jumped() <-chan struct{} { return j.jumped }

func (j *jumper) jump(to time.Duration) {
	j.mu.Lock()
	j.at = to
	j.mu.Unlock()
	j.jumped <- struct{}{}
}

func TestMakeGivesWayToAJump(t *testing.T) {
	prev := minRun
	minRun = 0
	t.Cleanup(func() { minRun = prev })
	job, video := makeJob(t)
	job.SpeechLanguage = "ja"
	watch := &jumper{jumped: make(chan struct{}, 1)}
	f := &makeFilm{length: 600 * time.Second, language: "ja"}
	// The first run, hearing from the start of the film, has heard its first cell when the user jumps to 8:20.
	f.stall = func() { go watch.jump(500 * time.Second) }
	done := Make(context.Background(), f, job, "简体中文", "mac", watch, func(core.Job) {})
	if done.Stage != core.StageComplete {
		t.Fatalf("job ended %s: %v", done.Stage, done.Err)
	}
	if len(f.runs) < 2 || f.runs[1][0] != 465*time.Second {
		t.Fatalf("runs %v, want the second to start at the cell the jump landed in, padded", f.runs)
	}
	// What the stopped run heard is kept and not heard again: every line shows once.
	doc, _ := track.Read(video)
	lines := doc.View()
	if len(lines) != 60 {
		t.Fatalf("%d lines, want 60", len(lines))
	}
	for i, l := range lines {
		if want := time.Duration(i) * 10 * time.Second; l.Start != want {
			t.Fatalf("line %d starts at %v, want %v: a line heard twice or lost", i, l.Start, want)
		}
	}
}

// boundaryFilm is a film whose speech recognition places a line said across 1:00, where two cells meet, by where a run starts: a run starting before 0:45 hears it end after 1:00, so its middle is in the second cell; one starting after hears it start earlier, so its middle is in the first. Each run alone keeps it out of its cells.
type boundaryFilm struct{ makeFilm }

func (f *boundaryFilm) Hear(ctx context.Context, video string, from, to time.Duration, heard func(core.Segment)) (string, error) {
	f.mu.Lock()
	f.runs = append(f.runs, [2]time.Duration{from, to})
	f.mu.Unlock()
	for _, at := range []time.Duration{10, 20, 30, 40, 50} {
		if at*time.Second >= from && at*time.Second < to {
			heard(core.Segment{Start: at * time.Second, End: at*time.Second + 2*time.Second, Text: "line"})
		}
	}
	across := core.Segment{Start: 58 * time.Second, End: 63 * time.Second, Text: "across"}
	if from >= 45*time.Second {
		across = core.Segment{Start: 56 * time.Second, End: 61 * time.Second, Text: "across"}
	}
	if across.Start >= from && across.End <= to {
		heard(across)
	}
	for _, at := range []time.Duration{70, 80} {
		if at*time.Second >= from && at*time.Second < to {
			heard(core.Segment{Start: at * time.Second, End: at*time.Second + 2*time.Second, Text: "line"})
		}
	}
	return "ja", nil
}

func TestALineAcrossTwoCellsIsKeptOnce(t *testing.T) {
	for _, at := range []time.Duration{0, 65 * time.Second} {
		job, video := makeJob(t)
		job.SpeechLanguage = "ja"
		f := &boundaryFilm{makeFilm{length: 90 * time.Second, language: "ja"}}
		// Watched from 1:05 the second cell is heard first, then the first; from the start, the other way round. Runs of one cell each show the boundary.
		prev := runCellsMax
		runCellsMax = 1
		done, _ := makeFilmOf(t, f, job, at)
		runCellsMax = prev
		if done.Stage != core.StageComplete {
			t.Fatalf("from %v: job ended %s: %v", at, done.Stage, done.Err)
		}
		doc, _ := track.Read(video)
		across := 0
		for _, l := range doc.View() {
			if l.Original.Text == "across" {
				across++
			}
		}
		if across != 1 {
			t.Fatalf("from %v: the line across the cells shows %d times, want once; runs %v", at, across, f.runs)
		}
	}
}

func TestMakeDraftsFarFromThePlayheadInBatches(t *testing.T) {
	job, _ := makeJob(t)
	job.SpeechLanguage = "ja"
	f := &makeFilm{length: 600 * time.Second, language: "ja"}
	done, _ := makeFilmOf(t, f, job, 0)
	if done.Stage != core.StageComplete {
		t.Fatalf("job ended %s: %v", done.Stage, done.Err)
	}
	// The three cells within 1:30 of the playhead are drafted one by one; the seventeen after, of three lines each, in batches of as many whole cells as fit in DefaultBatchSize lines.
	sizes := make([]int, len(f.translated))
	for i, lines := range f.translated {
		sizes[i] = len(lines)
	}
	full := DefaultBatchSize / 3 * 3
	if !slices.Equal(sizes, []int{3, 3, 3, full, 17*3 - full}) {
		t.Fatalf("drafts of %v lines, want batches of %d", sizes, full)
	}
}

// sayFoo is a film where every line says "say foo now", which its proofreading may correct to "say bar now".
func sayFoo(time.Duration) string { return "say foo now" }

// correctFoo corrects "foo" to "bar" in lines from the nth batch the proofreading is sent on.
func correctFoo(from int) func(int, []string) []string {
	return func(n int, lines []string) []string {
		out := slices.Clone(lines)
		if n >= from {
			for i := range out {
				out[i] = strings.ReplaceAll(out[i], "foo", "bar")
			}
		}
		return out
	}
}

func TestATermFoundInTwoLinesIsMadeInTheRestOfTheFilm(t *testing.T) {
	job, video := makeJob(t)
	job.SpeechLanguage = "ja"
	// The proofreading corrects the first cell's lines only; what it found there is a term, which the cells after take.
	f := &makeFilm{length: 150 * time.Second, language: "ja", say: sayFoo, proofread: func(n int, lines []string) []string {
		if n == 1 {
			return correctFoo(1)(n, lines)
		}
		return lines
	}}
	done, _ := makeFilmOf(t, f, job, 0)
	if done.Stage != core.StageComplete {
		t.Fatalf("job ended %s: %v", done.Stage, done.Err)
	}
	doc, _ := track.Read(video)
	for _, l := range doc.View() {
		if l.Original.Text != "say bar now" || l.Translation.Text != "译:say bar now" {
			t.Fatalf("line at %v reads %q, %q; want the term made in it", l.Start, l.Original.Text, l.Translation.Text)
		}
	}
	if terms := doc.Terms(doc.Shown()); len(terms) != 1 || !strings.Contains(terms[0].To, "bar") {
		t.Fatalf("terms %q", terms)
	}
}

func TestATermFoundLateRedraftsTheLinesNearThePlayhead(t *testing.T) {
	job, video := makeJob(t)
	job.SpeechLanguage = "ja"
	// The proofreading finds the term only from the third batch, the cell at 1:00; the two cells before it, near the playhead, were drafted without it.
	f := &makeFilm{length: 240 * time.Second, language: "ja", say: sayFoo, proofread: correctFoo(3)}
	done, _ := makeFilmOf(t, f, job, 0)
	if done.Stage != core.StageComplete {
		t.Fatalf("job ended %s: %v", done.Stage, done.Err)
	}
	doc, _ := track.Read(video)
	for _, l := range doc.View() {
		if l.Original.Text != "say bar now" || l.Translation.Text != "译:say bar now" {
			t.Fatalf("line at %v reads %q, %q; want it drafted again with the term", l.Start, l.Original.Text, l.Translation.Text)
		}
	}
	// The lines drafted again are translated again, not proofread again: three proofreading batches for the cells near the playhead, one for the rest.
	if f.proofreads != 4 {
		t.Fatalf("%d proofreading batches, want 4", f.proofreads)
	}
}

// makeFilmWith runs Make as makeFilmOf does, with ctx.
func makeFilmWith(t *testing.T, ctx context.Context, f Pipeline, job core.Job, at time.Duration) core.Job {
	t.Helper()
	return Make(ctx, f, job, "简体中文", "mac", From(at), func(core.Job) {})
}

func TestMakeProofreadsTheFilmABlockAtATime(t *testing.T) {
	job, video := makeJob(t)
	job.SpeechLanguage = "ja"
	f := &makeFilm{length: 400 * time.Second, language: "ja"}
	done := makeFilmWith(t, WithReview(context.Background(), core.ProofAutoAll), f, job, 0)
	if done.Stage != core.StageComplete {
		t.Fatalf("job ended %s: %v", done.Stage, done.Err)
	}
	// 6:40 is two blocks of the final proofread, the first of five minutes.
	if !slices.Equal(f.reviewed, []int{30, 10}) {
		t.Fatalf("final proofread read blocks of %v lines", f.reviewed)
	}
	doc, _ := track.Read(video)
	if drafts := DraftSpans(doc, "简体中文"); len(drafts) != 0 {
		t.Fatalf("%d lines are drafts once the film is proofread", len(drafts))
	}
	for _, l := range doc.View() {
		want := "译:" + l.Original.Text
		if l.Start%time.Minute == 0 {
			want = "终:" + want
		}
		if l.Translation.Text != want {
			t.Fatalf("line at %v reads %q, want %q", l.Start, l.Translation.Text, want)
		}
	}
	proofs, err := LoadProofs(ProofsPath(video))
	if err != nil || len(proofs) != 7 || proofs[0].Status != core.ProofApplied {
		t.Fatalf("proofs %+v (%v), want the 7 changes kept as applied", proofs, err)
	}
	// The bilingual subtitles written out are the proofread ones.
	data, _ := os.ReadFile(done.SubtitlePath)
	if !strings.Contains(string(data), "终:译:line 1m0s") {
		t.Fatalf("subtitles %q", data)
	}
}

func TestMakeLeavesAFilmADraftWithoutTheFinalProofread(t *testing.T) {
	job, video := makeJob(t)
	job.SpeechLanguage = "ja"
	f := &makeFilm{length: 90 * time.Second, language: "ja"}
	if done := makeFilmWith(t, context.Background(), f, job, 0); done.Stage != core.StageComplete || len(f.reviewed) != 0 {
		t.Fatalf("job ended %s, %d blocks proofread", done.Stage, len(f.reviewed))
	}
	doc, _ := track.Read(video)
	if drafts := DraftSpans(doc, "简体中文"); len(drafts) != len(doc.View()) {
		t.Fatalf("%d of %d lines are drafts without a final proofread, want all", len(drafts), len(doc.View()))
	}
}

func TestTheFinalProofreadMakesALateTermInTheLinesFarFromThePlayhead(t *testing.T) {
	job, video := makeJob(t)
	job.SpeechLanguage = "ja"
	// Watched from 5:00, the cells from there are drafted first, without the term; the proofreading finds it only at the start of the film, drafted last, so the lines from 6:30 on are neither near the playhead nor drafted with it.
	var mu sync.Mutex
	foundAtStart := false
	f := &makeFilm{length: 600 * time.Second, language: "ja", say: sayFoo, proofread: func(_ int, lines []string) []string {
		mu.Lock()
		defer mu.Unlock()
		if foundAtStart {
			return correctFoo(0)(0, lines)
		}
		return lines
	}}
	ctx := WithReview(context.Background(), core.ProofAutoAll)
	done := makeFilmWith(t, ctx, &lateTerm{makeFilm: f, at: 240 * time.Second, found: func() { mu.Lock(); foundAtStart = true; mu.Unlock() }}, job, 300*time.Second)
	if done.Stage != core.StageComplete {
		t.Fatalf("job ended %s: %v", done.Stage, done.Err)
	}
	doc, _ := track.Read(video)
	for _, l := range doc.View() {
		if l.Original.Text != "say bar now" || !strings.Contains(l.Translation.Text, "say bar now") {
			t.Fatalf("line at %v reads %q, %q; want the term made in it", l.Start, l.Original.Text, l.Translation.Text)
		}
	}
}

// lateTerm is a film whose proofreading starts correcting once it is sent lines from before at, the start of the film drafted after the rest.
type lateTerm struct {
	*makeFilm
	at    time.Duration
	found func()
}

func (f *lateTerm) Proofread(ctx context.Context, segments []core.Segment, title string) ([]core.Segment, error) {
	if len(segments) > 0 && segments[0].Start < f.at {
		f.found()
	}
	return f.makeFilm.Proofread(ctx, segments, title)
}

func TestTheUsersDecisionOnAChangeIsKeptInTheFilmsDocument(t *testing.T) {
	job, video := makeJob(t)
	job.SpeechLanguage = "ja"
	f := &makeFilm{length: 90 * time.Second, language: "ja"}
	done := makeFilmWith(t, WithReview(context.Background(), core.ProofAutoNone), f, job, 0)
	if done.Stage != core.StageComplete {
		t.Fatalf("job ended %s: %v", done.Stage, done.Err)
	}
	proofs, err := LoadProofs(ProofsPath(video))
	if err != nil || len(proofs) != 2 || proofs[1].Status != core.ProofPending {
		t.Fatalf("proofs %+v (%v), want the changes at 0:00 and 1:00 waiting", proofs, err)
	}
	accepted := core.AcceptProofs(proofs)
	if err := DecideProofs(video, done.SubtitlePath, "mac", proofs, accepted); err != nil {
		t.Fatal(err)
	}
	doc, _ := track.Read(video)
	var at track.Line
	for _, l := range doc.View() {
		if l.Start == time.Minute {
			at = l
		}
	}
	if at.Translation.Text != "终:译:line 1m0s" || at.Translation.Layer != track.LayerUser {
		t.Fatalf("line at 1:00 = %+v, want the change, as the user's", at.Translation)
	}
	data, _ := os.ReadFile(done.SubtitlePath)
	if !strings.Contains(string(data), "终:译:line 1m0s") {
		t.Fatalf("subtitles %q, want the change written out", data)
	}
	read, err := ReadTrack(video, done.SubtitlePath, TranscriptOf(video))
	if err != nil || len(read.Events) != len(doc.Events) {
		t.Fatalf("reading took in %d records (%v)", len(read.Events)-len(doc.Events), err)
	}
	// Turned down after all, the line is the machine's again.
	declined := slices.Clone(accepted)
	declined[1].Status = core.ProofRejected
	if err := DecideProofs(video, done.SubtitlePath, "mac", accepted, declined); err != nil {
		t.Fatal(err)
	}
	doc, _ = track.Read(video)
	for _, l := range doc.View() {
		if l.Start == time.Minute && l.Translation.Text != "译:line 1m0s" {
			t.Fatalf("line at 1:00 = %q once taken back", l.Translation.Text)
		}
	}
}

func TestALinkIsMadeFromItsSoundWhileItsVideoDownloads(t *testing.T) {
	dir := t.TempDir()
	f := &makeFilm{length: 90 * time.Second, language: "ja", linkDir: dir, videoGate: make(chan struct{})}
	job, err := core.NewJob("https://example.com/watch?v=x")
	if err != nil {
		t.Fatal(err)
	}
	job.SpeechLanguage = "ja"
	var reports []core.Job
	var mu sync.Mutex
	var release sync.Once
	shownBefore := false
	done := Make(context.Background(), f, job, "简体中文", "mac", nil, func(j core.Job) {
		mu.Lock()
		defer mu.Unlock()
		reports = append(reports, j)
		// Every cell made and its video still to come, the film is not shown yet; then the video comes.
		shownBefore = shownBefore || j.VideoPath == "" && j.SubtitlePath != ""
		if j.Stage == core.StageDownloading && len(reports) > 2 {
			release.Do(func() { close(f.videoGate) })
		}
	})
	if done.Stage != core.StageComplete {
		t.Fatalf("job ended %s: %v", done.Stage, done.Err)
	}
	// The subtitles made before the video is in, the job says it downloads again until it is.
	var stages []core.Stage
	for _, j := range reports {
		stages = enteredStage(stages, j)
	}
	if want := []core.Stage{core.StageDownloading, core.StageTranscribing, core.StageTranslating, core.StageDownloading, core.StageComplete}; !slices.Equal(stages, want) {
		t.Fatalf("stages = %v, want %v", stages, want)
	}
	video := filepath.Join(dir, "video.mp4")
	if done.VideoPath != video || done.SubtitlePath != filepath.Join(dir, "video.bilingual.srt") || shownBefore {
		t.Fatalf("job shows %q with %q (shown before its video: %v)", done.VideoPath, done.SubtitlePath, shownBefore)
	}
	// The sound is heard from hidden, so the library does not take it for a video, and goes once the video is there.
	for _, from := range f.heardFrom {
		if from != ".video.sound" {
			t.Fatalf("heard from %v, want the sound alone, hidden", f.heardFrom)
		}
	}
	for _, name := range []string{"video.m4a", ".video.sound"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s is left beside the video: %v", name, err)
		}
	}
	if doc, err := track.Read(video); err != nil || len(doc.View()) != 9 {
		t.Fatalf("the video's document shows %d lines (%v)", len(doc.View()), err)
	}
}

func TestALinkWhoseVideoFailsFailsOnceItsSubtitlesAreMade(t *testing.T) {
	dir := t.TempDir()
	f := &makeFilm{length: 60 * time.Second, language: "ja", linkDir: dir, videoErr: errors.New("HTTP Error 403: Forbidden")}
	job, _ := core.NewJob("https://example.com/watch?v=x")
	job.SpeechLanguage = "ja"
	done := Make(context.Background(), f, job, "简体中文", "mac", nil, func(core.Job) {})
	if done.Stage != core.StageFailed || done.Err == nil || !strings.Contains(done.Err.Error(), "download") {
		t.Fatalf("job ended %s: %v", done.Stage, done.Err)
	}
	// What was made is kept for a retry, which goes on from it.
	if doc, _ := track.Read(filepath.Join(dir, "video.m4a")); len(doc.View()) != 6 {
		t.Fatalf("the film's document shows %d lines", len(doc.View()))
	}
}

// TestADecisionKeepsSubtitlesRewrittenOutsideTheDocument decides a change of a film made into its document whose subtitles something else rewrote since, as a redo that Run makes: the decision is laid over what the files hold, not over what the document held before.
func TestADecisionKeepsSubtitlesRewrittenOutsideTheDocument(t *testing.T) {
	job, video := makeJob(t)
	job.SpeechLanguage = "ja"
	f := &makeFilm{length: 90 * time.Second, language: "ja"}
	done := makeFilmWith(t, WithReview(context.Background(), core.ProofAutoNone), f, job, 0)
	if done.Stage != core.StageComplete {
		t.Fatalf("job ended %s: %v", done.Stage, done.Err)
	}
	proofs, _ := LoadProofs(ProofsPath(video))
	// The film is translated again outside the document: every translation is now "redo:".
	data, _ := os.ReadFile(done.SubtitlePath)
	if err := os.WriteFile(done.SubtitlePath, []byte(strings.ReplaceAll(string(data), "译:", "redo:")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := DecideProofs(video, done.SubtitlePath, "mac", proofs, core.AcceptProofs(proofs)[:1]); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(done.SubtitlePath)
	if strings.Contains(string(data), "译:line 30s") || !strings.Contains(string(data), "redo:line 30s") {
		t.Fatalf("subtitles once decided %q, want the redo kept", data)
	}
}

// A making stopped while a draft is being translated returns only once the translation has: nothing of a stopped job still asks a service or writes beside the video after Make returns.
func TestMakeReturnsOnceItsWorkHasStopped(t *testing.T) {
	job, _ := makeJob(t)
	job.SpeechLanguage = "ja"
	ctx, stop := context.WithCancel(context.Background())
	var returned atomic.Bool
	f := &makeFilm{length: 30 * time.Second, translate: func(string) (string, error) {
		stop()
		time.Sleep(50 * time.Millisecond)
		returned.Store(true)
		return "", context.Canceled
	}}
	if done := makeFilmWith(t, ctx, f, job, 0); done.Stage != core.StageFailed || !returned.Load() {
		t.Fatalf("stopped %+v; the translation had returned: %v", done.Stage, returned.Load())
	}
}

// Lines written out as the document last wrote them, to files that still hold them, are not written again: a film made a part at a time is written out every few seconds, and its log would otherwise grow by two records each time.
func TestProjectingTheSameLinesWritesNothing(t *testing.T) {
	job, video := makeJob(t)
	job.SpeechLanguage = "ja"
	if done, _ := makeFilmOf(t, &makeFilm{length: 60 * time.Second, language: "ja"}, job, 0); done.Stage != core.StageComplete {
		t.Fatalf("job ended %s: %v", done.Stage, done.Err)
	}
	doc, _ := track.Read(video)
	transcript, bilingual := TranscriptOf(video), strings.TrimSuffix(video, ".mkv")+".bilingual.srt"
	if files, err := projectTrack(doc, doc.View(), transcript, bilingual); err != nil || len(files) != 0 {
		t.Fatalf("the same lines wrote %+v, %v", files, err)
	}
	// A file changed since is written again.
	if err := os.WriteFile(bilingual, []byte("edited elsewhere"), 0o644); err != nil {
		t.Fatal(err)
	}
	if files, err := projectTrack(doc, doc.View(), transcript, bilingual); err != nil || len(files) != 2 {
		t.Fatalf("a changed file wrote %+v, %v", files, err)
	}
	if data, _ := os.ReadFile(bilingual); string(data) == "edited elsewhere" {
		t.Fatal("the changed file was not written again")
	}
}

// A film's subtitles edited in another editor while it is made keep the edit: it is taken into the document before the lines are written out again.
func TestAnEditOutsideKandoWhileAFilmIsMadeIsKept(t *testing.T) {
	job, video := makeJob(t)
	job.SpeechLanguage = "ja"
	transcript, bilingual := TranscriptOf(video), strings.TrimSuffix(video, ".mkv")+".bilingual.srt"
	edited := false
	f := &makeFilm{length: 150 * time.Second, language: "ja"}
	f.translate = func(s string) (string, error) {
		// Once the first lines are written out, the user corrects the first one in both files.
		if !edited && s == "line 2m0s" && fileExists(bilingual) {
			edited = true
			for _, path := range []string{transcript, bilingual} {
				data, err := os.ReadFile(path)
				if err != nil {
					return "", err
				}
				if err := os.WriteFile(path, []byte(strings.ReplaceAll(string(data), "line 0s", "line zero")), 0o644); err != nil {
					return "", err
				}
			}
		}
		return "译:" + s, nil
	}
	if done, _ := makeFilmOf(t, f, job, 0); done.Stage != core.StageComplete {
		t.Fatalf("job ended %s: %v", done.Stage, done.Err)
	}
	if !edited {
		t.Fatal("the film was not written out before its last cell was drafted")
	}
	doc, _ := track.Read(video)
	if got := doc.View()[0].Original.Text; got != "line zero" {
		t.Fatalf("the first line reads %q, want the edit", got)
	}
	if data, _ := os.ReadFile(transcript); !strings.Contains(string(data), "line zero") {
		t.Fatalf("the transcript was written over: %s", data)
	}
}

// Two computers sharing a library: one goes on with a film the other began and did not finish only once the other's log has had no record for track.Lease, as the other may still be making it; until then it makes a generation of its own.
func TestMakeLeavesAnotherDevicesMakingToItWhileItWrites(t *testing.T) {
	for _, tc := range []struct {
		name  string
		since time.Duration
		gens  int
	}{
		{"the other device wrote a minute ago", time.Minute, 2},
		{"the other device has written nothing for the lease", track.Lease + time.Minute, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			job, video := makeJob(t)
			job.SpeechLanguage = "ja"
			ctx, stop := context.WithCancel(context.Background())
			f := &makeFilm{length: 90 * time.Second, language: "ja", translate: func(string) (string, error) {
				stop()
				return "", context.Canceled
			}}
			track.Now = func() time.Time { return time.Now().Add(-tc.since) }
			stopped := Make(ctx, f, job, "简体中文", "studio", From(0), func(core.Job) {})
			track.Now = time.Now
			if stopped.Stage != core.StageFailed {
				t.Fatalf("stopped %+v", stopped)
			}
			f.translate = nil
			if done, _ := makeFilmOf(t, f, job, 0); done.Stage != core.StageComplete {
				t.Fatalf("done %+v", done)
			}
			doc, _ := track.Read(video)
			gens := 0
			for _, e := range doc.Events {
				if e.Kind == track.KindGen {
					gens++
				}
			}
			if gens != tc.gens {
				t.Fatalf("%d generations, want %d", gens, tc.gens)
			}
		})
	}
}

// lateFilm is makeFilm whose speech recognition hands on the line said at 20 s after the one said at 30 s, in the next cell, as whisper may place a line a little out of order.
type lateFilm struct{ *makeFilm }

func (f lateFilm) Hear(ctx context.Context, video string, from, to time.Duration, heard func(core.Segment)) (string, error) {
	var held *core.Segment
	language, err := f.makeFilm.Hear(ctx, video, from, to, func(s core.Segment) {
		switch {
		case s.Start == 20*time.Second:
			held = &s
			return
		case held != nil && s.Start == 30*time.Second:
			heard(s)
			heard(*held)
			held = nil
			return
		}
		heard(s)
	})
	if held != nil {
		heard(*held)
	}
	return language, err
}

// A line the speech recognition hands on after a line of a later cell, whose own cell was handed on already, is kept, not dropped.
func TestMakeKeepsALineHeardLate(t *testing.T) {
	job, video := makeJob(t)
	job.SpeechLanguage = "ja"
	if done, _ := makeFilmOf(t, lateFilm{&makeFilm{length: 60 * time.Second, language: "ja"}}, job, 0); done.Stage != core.StageComplete {
		t.Fatalf("done %+v", done)
	}
	doc, _ := track.Read(video)
	var texts []string
	for _, l := range doc.View() {
		texts = append(texts, l.Original.Text)
	}
	if want := []string{"line 0s", "line 10s", "line 20s", "line 30s", "line 40s", "line 50s"}; !slices.Equal(texts, want) {
		t.Fatalf("lines %q, want %q", texts, want)
	}
}
