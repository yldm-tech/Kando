package jobfailure

import (
	"context"
	"errors"
	"strings"
	"testing"

	"kando/apps/kando/locales"
	"kando/apps/kando/plugins"
	"kando/apps/kando/settings/sectionid"
	"kando/packages/core"
	"kando/packages/media"
)

var en = locales.In("en")

// hint is what Hint says of a job that failed with err, as its card's status line says it.
func hint(err error) (string, string) {
	job := core.Job{Stage: core.StageFailed, Err: err}
	return Hint(en, en.QueueStatusFailed(err.Error()), Of(job))
}

// A failure a translation source of its own gave is named with what the source said, before any reason the AI service's words would give: a refused key, a missing program. Offline mode has words of its own (TestSourceHintOffline).
func TestHintNamesTheSourceFirst(t *testing.T) {
	for _, said := range []string{"translation API returned 401 Unauthorized: invalid key", "deepl is not installed"} {
		reason, section := hint(&media.StageError{Stage: media.StageTranslate, Source: "DeepL", Err: errors.New(said)})
		if want := en.PrecheckFailedSource("DeepL", said); reason != want || section != "" {
			t.Errorf("%q: %q %q, want %q", said, reason, section, want)
		}
	}
}

// A check that joined Kando's own missing tool with a source's complaint names no source, so it reads as the missing tool.
func TestHintOfAJoinedCheck(t *testing.T) {
	tools := media.Tools{Engine: checkless{}, EngineName: "DeepL"}
	_, own := tools.Translate(t.Context(), []core.Segment{{Text: "hi"}}, "en")
	joined := &media.StageError{Stage: media.StageCheck, Err: errors.Join(errors.New("ffmpeg is not installed"), own)}
	if name, ok := media.EngineOf(joined.Err); ok {
		t.Fatalf("the joined check is %q's", name)
	}
	if reason, section := hint(joined); reason != en.PrecheckNoTools() || section != sectionid.General {
		t.Fatalf("%q %q", reason, section)
	}
}

// A job of a video kept outside a library whose folder is not there says so, not that a tool failed.
func TestHintOfALibraryGone(t *testing.T) {
	failed := &media.StageError{Stage: media.StageCheck, Err: core.ErrLibraryGone}
	if reason, section := hint(failed); reason != en.LibraryFolderGone() || section != "" {
		t.Fatalf("%q %q", reason, section)
	}
}

// checkless is a translation source that always fails.
type checkless struct{}

func (checkless) Translate(context.Context, []core.Segment, string) ([]core.Segment, error) {
	return nil, errors.New("no DeepL key")
}

func TestHintReadsTheStep(t *testing.T) {
	for err, want := range map[error]string{
		media.ReadStageError("download: yt-dlp: exit status 1: ERROR: Something else"):               "Download failed: Something else",
		media.ReadStageError("transcribe: whisper-cli: signal: killed: whisper_init: out of memory"): "Generating subtitles failed: whisper_init: out of memory",
		media.ReadStageError("translate: translation API returned 401 Unauthorized: invalid key"):    en.PrecheckTranslateAuth(),
		media.ReadStageError("download: yt-dlp: " + media.ErrOffline.Error()):                        en.PrivacyNoDownload(),
		media.ReadStageError("check: " + media.ErrNoHTTP.Error()):                                    en.MediaSourceNoHTTP(),
		errors.New("boom"): en.PrecheckFailedProcess("boom"),
		// A step's error with no error says the step failed, and does not panic.
		&media.StageError{Stage: media.StageExport}: en.PrecheckFailedExport(""),
	} {
		if reason, _ := hint(err); reason != want {
			t.Errorf("%q: %q, want %q", err.Error(), reason, want)
		}
	}
}

// A done film the translation left untranslated says why as a failed translation does, naming the source that failed when one did.
func TestTranslationHint(t *testing.T) {
	if reason, section := TranslationHint(en, "translation API returned 401 Unauthorized: invalid key", ""); reason != en.PrecheckTranslateAuth() || section != sectionid.AI {
		t.Fatalf("the AI service's: %q %q", reason, section)
	}
	if reason, section := TranslationHint(en, "456 Quota Exceeded", "DeepL"); reason != en.PrecheckFailedSource("DeepL", "456 Quota Exceeded") || section != "" {
		t.Fatalf("the source's: %q %q", reason, section)
	}
}

// A stopped job's status line is kept as it is.
func TestHintKeepsWhatIsNotAFailure(t *testing.T) {
	if reason, section := Hint(en, en.QueueStatusCanceled(), nil); reason != en.QueueStatusCanceled() || section != "" {
		t.Fatalf("%q %q", reason, section)
	}
}

func TestOfAndStage(t *testing.T) {
	failed := &media.StageError{Stage: media.StageDownload, Err: errors.New("boom")}
	if got := Of(core.Job{Stage: core.StageFailed, Err: failed}); got != failed {
		t.Fatalf("Of %v", got)
	}
	if got := Of(core.Job{Stage: core.StageComplete, Err: failed}); got != nil {
		t.Fatalf("Of a done job %v", got)
	}
	if got := Stage(core.Job{Err: errors.New("download: boom")}); got != "" {
		t.Fatalf("a plain error's step %q", got)
	}
	if got := Stage(core.Job{Err: failed}); got != media.StageDownload {
		t.Fatalf("step %q", got)
	}
}

func TestToolMessage(t *testing.T) {
	if got := ToolMessage("yt-dlp: exit status 1: ERROR: [youtube] abc: Video unavailable\nmore"); got != "Video unavailable" {
		t.Fatalf("%q", got)
	}
}

// A plugin's translator that failed with a code Kando knows is told in words of its own in every interface language, each naming the source and none of them the code or the AI service's settings; the key and the plugin's switch are in 插件, the target language in 翻译. A code it does not know is told with what it said.
func TestSourceHintOfACode(t *testing.T) {
	sections := map[string]string{
		plugins.TranslateQuota: "", plugins.TranslateAuth: sectionid.Plugins, plugins.TranslateNoCredential: sectionid.Plugins, plugins.TranslateLanguage: sectionid.Translate,
		plugins.TranslateRateLimit: "", plugins.TranslateUnavailable: "", plugins.TranslateNotRunning: sectionid.Plugins, plugins.TranslateBadReply: "",
	}
	for _, lang := range locales.Interface {
		m := locales.In(lang)
		seen := map[string]string{}
		for code, want := range sections {
			failed := &media.StageError{Stage: media.StageTranslate, Source: "DeepL", Err: plugins.NewTranslateError(code, "the service said so")}
			reason, section := Hint(m, m.QueueStatusFailed(failed.Error()), failed)
			if !strings.Contains(reason, "DeepL") || strings.Contains(reason, code+":") || strings.Contains(reason, "the service said so") || section != want {
				t.Errorf("%s %s: %q %q", lang, code, reason, section)
			}
			if other, ok := seen[reason]; ok {
				t.Errorf("%s: %s and %s read the same: %q", lang, code, other, reason)
			}
			seen[reason] = code
		}
		failed := &media.StageError{Stage: media.StageTranslate, Source: "DeepL", Err: plugins.NewTranslateError("teapot", "I'm a teapot")}
		if reason, section := Hint(m, m.QueueStatusFailed(failed.Error()), failed); reason != m.PrecheckFailedSource("DeepL", "I'm a teapot") || section != "" {
			t.Errorf("%s: an unknown code: %q %q", lang, reason, section)
		}
	}
}

// A translation source offline mode refused, as it is not on this computer, is told so in every interface language, with 隐私 to turn offline mode off, rather than as the AI service's or with the error's own words.
func TestSourceHintOffline(t *testing.T) {
	tools := media.Tools{Engine: checkless{}, EngineName: "DeepL", Offline: true}
	_, err := tools.Translate(t.Context(), []core.Segment{{Text: "hi"}}, "en")
	if !errors.Is(err, media.ErrOffline) {
		t.Fatalf("not refused offline: %v", err)
	}
	failed := &media.StageError{Stage: media.StageTranslate, Source: "DeepL", Err: err}
	for _, lang := range locales.Interface {
		m := locales.In(lang)
		if reason, section := Hint(m, m.QueueStatusFailed(failed.Error()), failed); reason != m.PrecheckSourceOffline("DeepL") || section != sectionid.Privacy {
			t.Errorf("%s: %q %q", lang, reason, section)
		}
	}
}

// AI tells the failures of the AI service and of a translation source from the others, and an account they refused from a request: a 402, or a 429 that says the quota ran out, wherever it came; a source's quota code; anything else of the translation step; and nothing of a download, a canceled job or one that did not fail.
func TestAI(t *testing.T) {
	for _, tc := range []struct {
		name            string
		job             core.Job
		ai, unavailable bool
	}{
		{"out of credit", core.Job{Err: media.ReadStageError("translate: translation API returned 402 Payment Required: top up")}, true, true},
		{"over the quota", core.Job{Err: media.ReadStageError("translate: translation API returned 429 Too Many Requests: insufficient_quota")}, true, true},
		{"too fast", core.Job{Err: media.ReadStageError("translate: translation API returned 429 Too Many Requests: slow down")}, true, false},
		{"a key rejected", core.Job{Err: media.ReadStageError("translate: translation API returned 401 Unauthorized: bad key")}, true, false},
		{"a reply out of shape", core.Job{Err: media.ReadStageError("translate: not a JSON array of strings")}, true, false},
		{"a proofread out of credit", core.Job{Err: media.ReadStageError("transcribe: translation API returned 402 Payment Required: top up")}, true, true},
		{"a source over its quota", core.Job{Err: &media.StageError{Stage: media.StageTranslate, Source: "DeepL", Err: errors.New(plugins.TranslateQuota + ": used up")}}, true, true},
		{"a source down", core.Job{Err: &media.StageError{Stage: media.StageTranslate, Source: "DeepL", Err: errors.New(plugins.TranslateUnavailable)}}, true, false},
		{"a download", core.Job{Err: media.ReadStageError("download: ERROR: Video unavailable")}, false, false},
		{"no step named", core.Job{Err: errors.New("disk full")}, false, false},
	} {
		tc.job.Stage = core.StageFailed
		if ai, unavailable := AI(tc.job); ai != tc.ai || unavailable != tc.unavailable {
			t.Errorf("%s: AI %v, unavailable %v; want %v, %v", tc.name, ai, unavailable, tc.ai, tc.unavailable)
		}
	}
	if ai, _ := AI(core.Job{Stage: core.StageComplete, Err: media.ReadStageError("translate: translation API returned 402 Payment Required: x")}); ai {
		t.Error("a job that did not fail is the AI service's failure")
	}
}
