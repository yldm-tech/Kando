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

// A failure a translation source of its own gave is named with what the source said, before any reason the AI service's words would give: a refused key, a missing program, offline mode.
func TestHintNamesTheSourceFirst(t *testing.T) {
	for _, said := range []string{"translation API returned 401 Unauthorized: invalid key", "deepl is not installed", media.ErrOffline.Error()} {
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
