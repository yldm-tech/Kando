package media

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/packages/core"
	"kando/packages/track"
)

// stageOf is the step err names, failing the test when it names none.
func stageOf(t *testing.T, err error) *StageError {
	t.Helper()
	var failed *StageError
	if !errors.As(err, &failed) {
		t.Fatalf("%v (%T) names no step", err, err)
	}
	return failed
}

// Every way Make fails names the step it failed at, with its text as a failed job's text has always read, and errors.Is sees the error that stopped it.
func TestMakeNamesTheStepItFailedAt(t *testing.T) {
	ctx := context.Background()
	link, _ := core.NewJob("https://example.com/video")

	_, checked := runStages(t, &fakePipeline{checkErr: errors.New("ffmpeg is not installed")})
	noDevice := Make(ctx, &fakePipeline{}, link, "简体中文", "", nil, func(core.Job) {})
	// A fake given no folder cannot download the link.
	downloaded := Make(ctx, &fakePipeline{}, link, "简体中文", "mac", nil, func(core.Job) {})
	videoOnly := link
	videoOnly.Redo = core.RedoVideo
	videoDownloaded := Make(ctx, &fakePipeline{}, videoOnly, "简体中文", "", nil, func(core.Job) {})
	_, silent := runStages(t, &fakePipeline{})
	job, video := makeJob(t)
	job.SpeechLanguage = "ja"
	unheard, _ := makeFilmOf(t, &makeFilm{length: 60 * time.Second, hearErr: errors.New("whisper-cli: failed to load model")}, job, 0)
	again := job
	again.Redo = core.RedoTranslation
	untranscribed := Make(ctx, &fakePipeline{}, again, "简体中文", "mac", nil, func(core.Job) {})
	if err := os.Mkdir(strings.TrimSuffix(video, ".mkv")+".bilingual.srt", 0o755); err != nil {
		t.Fatal(err)
	}
	unwritten, _ := makeFilmOf(t, &makeFilm{length: 60 * time.Second, language: "ja"}, job, 0)

	for _, tc := range []struct {
		name  string
		job   core.Job
		stage Stage
		is    error
	}{
		{"check", checked, StageCheck, nil},
		{"no device", noDevice, StageCheck, track.ErrDevice},
		{"download", downloaded, StageDownload, nil},
		{"video download", videoDownloaded, StageDownload, nil},
		{"no speech", silent, StageTranscribe, ErrNoSpeech},
		{"speech recognition", unheard, StageTranscribe, nil},
		{"translation", untranscribed, StageTranslate, ErrNoTranscript},
		{"export", unwritten, StageExport, nil},
	} {
		if tc.job.Stage != core.StageFailed {
			t.Errorf("%s: job ended %s: %v", tc.name, tc.job.Stage, tc.job.Err)
			continue
		}
		failed := stageOf(t, tc.job.Err)
		if failed.Stage != tc.stage || failed.Source != "" {
			t.Errorf("%s: failed at %q from %q, want %q", tc.name, failed.Stage, failed.Source, tc.stage)
		}
		if want := string(tc.stage) + ": " + failed.Err.Error(); tc.job.Err.Error() != want {
			t.Errorf("%s: text %q, want %q", tc.name, tc.job.Err.Error(), want)
		}
		if tc.is != nil && !errors.Is(tc.job.Err, tc.is) {
			t.Errorf("%s: errors.Is(%v, %v) is false", tc.name, tc.job.Err, tc.is)
		}
	}
}

// A job stopped while it is made fails at its step with an error errors.Is tells as stopped, as the queue reads it.
func TestAStoppedJobIsStoppedThroughItsStep(t *testing.T) {
	job, _ := makeJob(t)
	ctx, stop := context.WithCancel(context.Background())
	stop()
	done := makeFilmWith(t, ctx, &makeFilm{length: 30 * time.Second}, job, 0)
	if done.Stage != core.StageFailed || !errors.Is(done.Err, context.Canceled) {
		t.Fatalf("job ended %s: %v", done.Stage, done.Err)
	}
	stageOf(t, done.Err)
}

func TestStageErrorReadsAndUnwraps(t *testing.T) {
	err := failedAt(StageDownload, ErrOffline)
	if err.Error() != "download: "+ErrOffline.Error() || !errors.Is(err, ErrOffline) || errors.Unwrap(err) != ErrOffline {
		t.Fatalf("%q unwraps to %v", err.Error(), errors.Unwrap(err))
	}
	partial := &UntranslatedError{Lines: 2, Total: 3, Err: errors.New("busy")}
	var got *UntranslatedError
	if !errors.As(failedAt(StageTranslate, partial), &got) || got != partial {
		t.Fatal("errors.As does not see through the step")
	}
}

// A failed job's text saved before the step was kept apart reads back as the step it names and the rest, as the app read it before; any other text reads as it is.
func TestReadStageErrorReadsWhatWasSaved(t *testing.T) {
	for text, want := range map[string]Stage{
		"check: ffmpeg is not installed":                                           StageCheck,
		"download: yt-dlp: exit status 1: ERROR: [youtube] abc: Video unavailable": StageDownload,
		"transcribe: whisper-cli: signal: killed: whisper_init: out of memory":     StageTranscribe,
		"translate: translation API returned 401 Unauthorized: invalid key":        StageTranslate,
		"export: open /v/a.srt: permission denied":                                 StageExport,
		"translate: ":           StageTranslate,
		"yt-dlp: exit status 1": "",
		"proofread: nothing":    "",
		"download:no space":     "",
		"Download: capital":     "",
		"":                      "",
	} {
		err := ReadStageError(text)
		if err.Error() != text {
			t.Errorf("%q reads back as %q", text, err.Error())
		}
		var failed *StageError
		switch {
		case want == "" && errors.As(err, &failed):
			t.Errorf("%q names step %q", text, failed.Stage)
		case want != "" && (!errors.As(err, &failed) || failed.Stage != want || failed.Source != "" || failed.Err.Error() != strings.TrimPrefix(text, string(want)+": ")):
			t.Errorf("%q reads as %+v, want step %q", text, failed, want)
		}
	}
}

// fakeEngine is a translation source of its own (Tools.Engine): it puts its name before each line, or fails with err, and counts its calls.
type fakeEngine struct {
	err        error
	checkErr   error
	local      bool
	calls      int
	candidates []string
}

func (e *fakeEngine) Translate(_ context.Context, segments []core.Segment, _ string) ([]core.Segment, error) {
	e.calls++
	if e.err != nil {
		return nil, e.err
	}
	out := slices.Clone(segments)
	for i := range out {
		out[i].Text = "MT:" + out[i].Text
	}
	return out, nil
}

func (e *fakeEngine) CheckTranslate() error { return e.checkErr }

func (e *fakeEngine) Local() bool { return e.local }

// candidateEngine is a fakeEngine that offers other translations of a line.
type candidateEngine struct{ *fakeEngine }

func (e candidateEngine) Candidates(context.Context, CandidateRequest) ([]string, error) {
	e.calls++
	return e.candidates, e.err
}

func TestToolsTranslateWithTheEngineSet(t *testing.T) {
	ctx := context.Background()
	lines := []core.Segment{{End: time.Second, Text: "hello"}}
	engine := &fakeEngine{}
	tools := Tools{Engine: engine, EngineName: "DeepL"}
	got, err := tools.Translate(ctx, lines, "简体中文")
	if err != nil || len(got) != 1 || got[0].Text != "MT:hello" || engine.calls != 1 {
		t.Fatalf("translated %+v, %v after %d calls", got, err, engine.calls)
	}
	// It needs neither the AI service's model nor its sign-in.
	if err := tools.CheckTranslate(); err != nil {
		t.Fatalf("check: %v", err)
	}

	quota := errors.New("456 Quota Exceeded")
	engine.err = quota
	_, err = tools.Translate(ctx, lines, "简体中文")
	if err == nil || err.Error() != quota.Error() || !errors.Is(err, quota) {
		t.Fatalf("the engine's failure reads %v", err)
	}
	if failed := failedAt(StageTranslate, err); failed.Source != "DeepL" || failed.Error() != "translate: "+quota.Error() {
		t.Fatalf("the step names %q: %q", failed.Source, failed.Error())
	}
	partial := &UntranslatedError{Lines: 1, Total: 1, Err: errors.New("busy")}
	engine.err = partial
	var still *UntranslatedError
	if _, err := tools.Translate(ctx, lines, "简体中文"); !errors.As(err, &still) || still != partial {
		t.Fatalf("lines left untranslated read %v", err)
	}

	engine.checkErr = errors.New("no DeepL key")
	if err := tools.CheckTranslate(); err == nil || err.Error() != "no DeepL key" || failedAt(StageCheck, errors.Join(nil, err)).Source != "DeepL" {
		t.Fatalf("check: %v", err)
	}
}

// Offline mode refuses an engine not on this computer before anything is sent, as it refuses the AI service, and leaves one that is on.
func TestToolsOfflineRefusesARemoteEngine(t *testing.T) {
	ctx := context.Background()
	lines := []core.Segment{{End: time.Second, Text: "hello"}}
	engine := &fakeEngine{}
	tools := Tools{Engine: engine, EngineName: "DeepL", Offline: true}
	if _, err := tools.Translate(ctx, lines, "简体中文"); !errors.Is(err, ErrOffline) || engine.calls != 0 {
		t.Fatalf("offline: %v after %d calls", err, engine.calls)
	}
	if err := tools.CheckTranslate(); !errors.Is(err, ErrOffline) {
		t.Fatalf("offline check: %v", err)
	}
	engine.local = true
	if got, err := tools.Translate(ctx, lines, "简体中文"); err != nil || got[0].Text != "MT:hello" {
		t.Fatalf("a local engine offline: %+v, %v", got, err)
	}
}

// Without an engine, Tools translates with its translator as it always has: here offline mode refuses the translator's service, which is not on this computer.
func TestToolsTranslateWithoutAnEngineUsesTheTranslator(t *testing.T) {
	tools := Tools{Offline: true, Translator: ChatTranslator{BaseURL: "https://api.example.com/v1", APIKey: "k", Model: "m"}}
	_, err := tools.Translate(context.Background(), []core.Segment{{Text: "hello"}}, "简体中文")
	var engine *engineError
	if !errors.Is(err, ErrOffline) || errors.As(err, &engine) {
		t.Fatalf("translated: %v", err)
	}
	if failed := failedAt(StageTranslate, err); failed.Source != "" {
		t.Fatalf("the translator's failure names %q", failed.Source)
	}
	if _, err := tools.Candidates(context.Background(), CandidateRequest{Line: core.Line{Original: "hi"}}); !errors.Is(err, ErrOffline) {
		t.Fatalf("candidates: %v", err)
	}
}

// A film whose translation source fails block after block fails a redo at the translation, naming the source.
func TestMakeNamesTheEngineThatFailed(t *testing.T) {
	job, _ := makeJob(t)
	job.SpeechLanguage = "ja"
	f := &makeFilm{length: 30 * time.Second, language: "ja"}
	film := makeFilmWith(t, context.Background(), f, job, 0)
	if film.Stage != core.StageComplete {
		t.Fatalf("film ended %s: %v", film.Stage, film.Err)
	}
	engine := &fakeEngine{err: errors.New("456 Quota Exceeded")}
	failed := makeFilmWith(t, context.Background(), engineFilm{makeFilm: f, tools: Tools{Engine: engine, EngineName: "DeepL"}}, redoOf(film), 0)
	if failed.Stage != core.StageFailed || engine.calls == 0 {
		t.Fatalf("redo ended %s: %v after %d calls", failed.Stage, failed.Err, engine.calls)
	}
	if got := stageOf(t, failed.Err); got.Stage != StageTranslate || got.Source != "DeepL" || failed.Err.Error() != "translate: 456 Quota Exceeded" {
		t.Fatalf("failed at %q from %q: %q", got.Stage, got.Source, failed.Err.Error())
	}
}

// engineFilm is a makeFilm translated by tools, whose Engine is set.
type engineFilm struct {
	*makeFilm
	tools Tools
}

func (f engineFilm) Translate(ctx context.Context, segments []core.Segment, language string) ([]core.Segment, error) {
	return f.tools.Translate(ctx, segments, language)
}

// An engine that offers other translations of a line is asked for them in place of the translator, and they are kept as the translator's are; one that offers none leaves them to the translator.
func TestToolsCandidatesAskTheEngine(t *testing.T) {
	ctx := context.Background()
	req := CandidateRequest{Language: "简体中文", Line: core.Line{Original: "Hi.", Translation: "嗨。"}}
	engine := candidateEngine{&fakeEngine{candidates: []string{"你好。", "嗨。", " ", "你好。", "哈喽。", "喂。", "第五个"}}}
	tools := Tools{Engine: engine, EngineName: "DeepL"}
	got, err := tools.Candidates(ctx, req)
	if want := []string{"你好。", "哈喽。", "喂。"}; err != nil || !slices.Equal(got, want) {
		t.Fatalf("candidates %q, %v; want %q", got, err, want)
	}
	engine.err = errors.New("456 Quota Exceeded")
	var named *engineError
	if _, err := tools.Candidates(ctx, req); !errors.As(err, &named) || named.name != "DeepL" {
		t.Fatalf("the engine's failure reads %v", err)
	}
	tools.Offline = true
	calls := engine.calls
	if _, err := tools.Candidates(ctx, req); !errors.Is(err, ErrOffline) || engine.calls != calls {
		t.Fatalf("offline: %v", err)
	}

	srv, seen := candidateChat(t, `["你好。"]`)
	plain := Tools{Engine: &fakeEngine{}, Translator: ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m"}}
	if got, err := plain.Candidates(ctx, req); err != nil || !slices.Equal(got, []string{"你好。"}) || len(*seen) != 1 {
		t.Fatalf("an engine without candidates: %q, %v after %d requests", got, err, len(*seen))
	}
}

func (f engineFilm) chats() bool { return f.tools.chats() }

// With a translation source of its own and no AI service set up, a film is translated by the source and made without proofreading, the final proofread or term suggestions, which need the AI service, rather than fail at them; with the AI service set up as well, it does them too.
func TestMakeWithAnEngineLeavesOutWhatNeedsTheAIService(t *testing.T) {
	for _, setUp := range []bool{false, true} {
		job, video := makeJob(t)
		job.SpeechLanguage = "ja"
		tools := Tools{Engine: &fakeEngine{}, EngineName: "DeepL"}
		if setUp {
			tools.Translator = ChatTranslator{BaseURL: "https://api.example.com/v1", APIKey: "k", Model: "m"}
		}
		f := &makeFilm{length: 30 * time.Second, language: "ja"}
		ctx := WithTermSuggestions(WithReview(context.Background(), core.ProofAutoAll))
		done := makeFilmWith(t, ctx, engineFilm{makeFilm: f, tools: tools}, job, 0)
		if done.Stage != core.StageComplete || done.TranslationErr != "" {
			t.Fatalf("set up %v: job ended %s: %v %q", setUp, done.Stage, done.Err, done.TranslationErr)
		}
		if doc, _ := track.Read(video); !strings.Contains(doc.View()[0].Translation.Text, "MT:") {
			t.Fatalf("set up %v: lines %+v", setUp, doc.View())
		}
		if asked := f.proofreads > 0 || len(f.reviewed) > 0; asked != setUp {
			t.Errorf("set up %v: %d proofreads, final proofread of %v", setUp, f.proofreads, f.reviewed)
		}
	}
}

func TestToolsChatsOnlyWithTheAIServiceSetUp(t *testing.T) {
	ready := ChatTranslator{BaseURL: "https://api.example.com/v1", APIKey: "k", Model: "m"}
	for _, tc := range []struct {
		name  string
		tools Tools
		want  bool
	}{
		{"no engine", Tools{}, true},
		{"engine alone", Tools{Engine: &fakeEngine{}}, false},
		{"engine and the AI service", Tools{Engine: &fakeEngine{}, Translator: ready}, true},
		{"no model", Tools{Engine: &fakeEngine{}, Translator: ChatTranslator{BaseURL: ready.BaseURL, APIKey: "k"}}, false},
		{"EveryAPI without a key or a sign-in", Tools{Engine: &fakeEngine{}, Translator: ChatTranslator{BaseURL: EveryAPIBaseURL, Model: "m"}}, false},
		{"EveryAPI signed in", Tools{Engine: &fakeEngine{}, Translator: ChatTranslator{BaseURL: EveryAPIBaseURL, Model: "m", Credential: func(context.Context, bool) (Endpoint, error) { return Endpoint{}, nil }}}, true},
		{"offline", Tools{Engine: &fakeEngine{}, Translator: ready, Offline: true}, false},
		{"offline on this computer", Tools{Engine: &fakeEngine{}, Translator: ChatTranslator{BaseURL: "http://127.0.0.1:11434/v1", Model: "m"}, Offline: true}, true},
	} {
		if got := tc.tools.chats(); got != tc.want {
			t.Errorf("%s: chats %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A check that joins Kando's own missing tools with a translation source's complaint is not the source's failure; one the source's complaint alone fails is, and so is the failure offline mode gives a source not on this computer.
func TestTheSourceIsNamedOnlyForItsOwnFailure(t *testing.T) {
	tools := Tools{Engine: &fakeEngine{checkErr: errors.New("no DeepL key")}, EngineName: "DeepL"}
	own := tools.CheckTranslate()
	if got := failedAt(StageCheck, errors.Join(errors.New("ffmpeg is not installed"), own)); got.Source != "" {
		t.Fatalf("a joined check names %q", got.Source)
	}
	if got := failedAt(StageCheck, errors.Join(nil, own)); got.Source != "DeepL" {
		t.Fatalf("the source's check alone names %q", got.Source)
	}
	if got := failedAt(StageTranslate, fmt.Errorf("draft: %w", own)); got.Source != "DeepL" {
		t.Fatalf("a wrapped failure names %q", got.Source)
	}
	other := Tools{Engine: &fakeEngine{checkErr: errors.New("no key")}, EngineName: "Papago"}.CheckTranslate()
	if got := failedAt(StageCheck, errors.Join(own, other)); got.Source != "" {
		t.Fatalf("two sources' failures name %q", got.Source)
	}
	if got := failedAt(StageCheck, nil); got.Source != "" {
		t.Fatalf("no error names %q", got.Source)
	}

	tools = Tools{Engine: &fakeEngine{}, EngineName: "DeepL", Offline: true}
	_, err := tools.Translate(context.Background(), []core.Segment{{Text: "hi"}}, "简体中文")
	if name, ok := EngineOf(err); !ok || name != "DeepL" || !errors.Is(err, ErrOffline) {
		t.Fatalf("offline: %v names %q", err, name)
	}
}

// An engine set without a name is named by its Go type, so its failure is never taken for the AI service's.
func TestAnEngineWithoutANameIsNamedByItsType(t *testing.T) {
	tools := Tools{Engine: &fakeEngine{err: errors.New("boom")}, EngineName: "  "}
	_, err := tools.Translate(context.Background(), []core.Segment{{Text: "hi"}}, "简体中文")
	if got := failedAt(StageTranslate, err); got.Source != "*media.fakeEngine" {
		t.Fatalf("named %q", got.Source)
	}
}

// A StageError with no error reads as its step alone, and making one does not panic.
func TestAStageErrorWithoutAnError(t *testing.T) {
	failed := failedAt(StageExport, nil)
	if failed.Error() != "export" || failed.Detail() != "" || failed.Source != "" || errors.Unwrap(failed) != nil {
		t.Fatalf("%q %q %q", failed.Error(), failed.Detail(), failed.Source)
	}
	if got := (&StageError{Stage: StageTranslate, Err: errors.New("busy")}).Detail(); got != "busy" {
		t.Fatalf("detail %q", got)
	}
}

// A film whose translation source fails cell after cell is made with its original lines, and says which source failed (core.Job.TranslationSource); the AI service's failure names none.
func TestAFilmNotTranslatedNamesTheSource(t *testing.T) {
	job, _ := makeJob(t)
	job.SpeechLanguage = "ja"
	f := &makeFilm{length: 60 * time.Second, language: "ja"}
	done := makeFilmWith(t, context.Background(), engineFilm{makeFilm: f, tools: Tools{Engine: &fakeEngine{err: errors.New("456 Quota Exceeded")}, EngineName: "DeepL"}}, job, 0)
	if done.Stage != core.StageComplete || done.TranslationErr != "456 Quota Exceeded" || done.TranslationSource != "DeepL" {
		t.Fatalf("job ended %s: %q from %q", done.Stage, done.TranslationErr, done.TranslationSource)
	}
	job, _ = makeJob(t)
	job.SpeechLanguage = "ja"
	f = &makeFilm{length: 60 * time.Second, language: "ja", translate: func(string) (string, error) { return "", errors.New("502 bad gateway") }}
	if done := makeFilmWith(t, context.Background(), f, job, 0); done.TranslationErr == "" || done.TranslationSource != "" {
		t.Fatalf("the AI service's failure: %q from %q", done.TranslationErr, done.TranslationSource)
	}
}
