package settings

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"kando/apps/kando/locales/testlocale"
	"kando/packages/media"
)

// rejected is an API answer with an HTTP status, as media's errors carry one.
type rejected int

func (r rejected) Error() string   { return "rejected " + strconv.Itoa(int(r)) }
func (r rejected) StatusCode() int { return int(r) }

// TestKitsErrorSaysWhy checks the words for why a program's download failed: a stall, an HTTP status and a dropped connection each in plain words of the interface's language, anything else as it is.
func TestKitsErrorSaysWhy(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{nil, ""},
		{media.ErrStalled, "服务器一直没有发来数据"},
		{rejected(404), "服务器返回 HTTP 404"},
		{fmt.Errorf("%w: read: connection reset by peer", media.ErrDisconnected), "连接断了"},
		{errors.New("download ffmpeg: checksum 00, want 11"), "download ffmpeg: checksum 00, want 11"},
	} {
		if got := KitsError(testlocale.Messages(), tc.err); got != tc.want {
			t.Errorf("KitsError(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
	testlocale.Use(t, "en")
	if got := KitsError(testlocale.Messages(), media.ErrStalled); got != "the server stopped sending data" {
		t.Errorf("in English, a stall is %q", got)
	}
}

// TestSpeechDownloadErrorSaysWhy checks the words for why a speech model's download failed, in the interface's language: a stall and a dropped connection say that downloading again continues, an HTTP status names it, Hugging Face's suggesting hf-mirror.com, and anything else shows as it is.
func TestSpeechDownloadErrorSaysWhy(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{nil, ""},
		{ErrNoModelsDir, "还没有模型目录"},
		{media.ErrStalled, "下载停住了，一直收不到数据。重新下载会从断开的地方接着下。"},
		{rejected(503), "服务器返回 HTTP 503，请稍后再试。"},
		{fmt.Errorf("%w: unexpected EOF", media.ErrDisconnected), "连接断了。检查网络后再下载一次。"},
		{errors.New("no space left on device"), "no space left on device"},
		{DownloadError{Source: media.SpeechSourceMirror, Err: fmt.Errorf("%w: unexpected EOF", media.ErrDisconnected)}, "连接断了。检查网络后再下载一次。"},
		{DownloadError{Source: media.SpeechSourceMirror, Err: media.ErrStalled}, "下载停住了，一直收不到数据。重新下载会从断开的地方接着下。"},
		{DownloadError{Err: rejected(503)}, "Hugging Face 返回 HTTP 503。请稍后再试，或从 hf-mirror.com 下载。"},
		{DownloadError{Source: media.SpeechSourceMirror, Err: rejected(503)}, "服务器返回 HTTP 503，请稍后再试。"},
	} {
		if got := SpeechDownloadError(testlocale.Messages(), tc.err); got != tc.want {
			t.Errorf("SpeechDownloadError(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
	testlocale.Use(t, "en")
	if got := SpeechDownloadError(testlocale.Messages(), rejected(404)); got != "The server answered HTTP 404. Try again later." {
		t.Errorf("in English, HTTP 404 is %q", got)
	}
}

// TestSpeechDownloadErrorSaysWhyTheTrashRefused checks the words for a model the Trash did not take: the one in use, and one the system refused. Under NSGo desktop.Trash fails with a fixed English sentinel and logs the system's reason, so the sentence names the model and leaves the error's text out.
func TestSpeechDownloadErrorSaysWhyTheTrashRefused(t *testing.T) {
	msgs := testlocale.Messages()
	if got, want := SpeechDownloadError(msgs, TrashError{ID: "large-v3"}), msgs.SettingsSpeechTrashInUse("Large v3"); got != want {
		t.Errorf("in use: %q, want %q", got, want)
	}
	if got, want := SpeechDownloadError(msgs, TrashError{ID: "large-v3", Err: errors.New("permission denied")}), msgs.SettingsSpeechTrashFailed("Large v3"); got != want || strings.Contains(got, "permission denied") {
		t.Errorf("refused: %q, want %q", got, want)
	}
}

// TestSpeechDownloadErrorSaysWhyTheModelFileWasNotUsed checks the words for a file chosen with 选择模型文件… that is not used: one that is not a whisper.cpp model, and one Kando could not read, whose system reason goes to the log rather than into the sentence.
func TestSpeechDownloadErrorSaysWhyTheModelFileWasNotUsed(t *testing.T) {
	msgs := testlocale.Messages()
	if got, want := SpeechDownloadError(msgs, ModelFileError{Path: "/models/model.gguf", Err: media.ErrNotSpeechModel}), msgs.SettingsSpeechNotModelFile("model.gguf"); got != want {
		t.Errorf("not a model: %q, want %q", got, want)
	}
	if got, want := SpeechDownloadError(msgs, ModelFileError{Path: "/models/ggml-base.bin", Err: errors.New("permission denied")}), msgs.SettingsSpeechModelFileUnreadable("ggml-base.bin"); got != want || strings.Contains(got, "permission denied") {
		t.Errorf("unreadable: %q, want %q", got, want)
	}
}

// TestProviderIDFindsThePreset checks that an endpoint finds its preset whatever its trailing slash and spaces, and that any other is custom.
func TestProviderIDFindsThePreset(t *testing.T) {
	for base, want := range map[string]string{
		media.EveryAPIBaseURL:          "everyapi",
		" https://api.openai.com/v1/ ": "openai",
		"http://127.0.0.1:11434/v1":    "ollama",
		"https://llm.example.com/v1":   "custom",
		"":                             "custom",
	} {
		if got := ProviderID(Presets(testlocale.Messages()), base); got != want {
			t.Errorf("ProviderID(%q) = %q, want %q", base, got, want)
		}
	}
	if !SameHost("https://api.openai.com/v1", "HTTPS://API.OPENAI.COM/other") || SameHost("https://api.openai.com/v1", "http://api.openai.com/v1") {
		t.Error("SameHost compares the scheme and host alone")
	}
}

// TestPresetResolutionReadsTheGivenList checks that PresetByID and ProviderID resolve against the list they are given alone: a preset only in that list is found by its id and its endpoint, and a built-in preset left out of it is not.
func TestPresetResolutionReadsTheGivenList(t *testing.T) {
	extra := Preset{ID: "extra", Name: "Extra", BaseURL: "https://llm.example.com/v1", Group: GroupCloud}
	list := append(Presets(testlocale.Messages()), extra)
	if got, ok := PresetByID(list, "extra"); !ok || got != extra {
		t.Errorf("PresetByID(list, extra) = %+v, %v, want %+v, true", got, ok, extra)
	}
	if got := ProviderID(list, "https://llm.example.com/v1/"); got != "extra" {
		t.Errorf("ProviderID(list, extra's endpoint) = %q, want extra", got)
	}
	only := []Preset{extra}
	if _, ok := PresetByID(only, "openai"); ok {
		t.Error("PresetByID found openai in a list without it")
	}
	if got := ProviderID(only, "https://api.openai.com/v1"); got != "custom" {
		t.Errorf("ProviderID(list without openai, openai's endpoint) = %q, want custom", got)
	}
	if _, ok := PresetByID(Presets(testlocale.Messages()), "extra"); ok {
		t.Error("PresetByID found extra among the built-in presets")
	}
}

// TestLLMTestErrorSaysWhy checks the words for why 测试连接 failed: a missing model or Base URL, a key the service refuses, an account out of credit, a refused request, a busy service, a timeout, an unreachable service and a reply that is not a chat answer each in plain words of the interface's language, never the API's English text.
func TestLLMTestErrorSaysWhy(t *testing.T) {
	msgs := testlocale.Messages()
	for _, tc := range []struct {
		err  error
		want string
	}{
		{nil, ""},
		{media.ErrNoModel, msgs.SettingsAITestNoModel()},
		{media.ErrNoBaseURL, msgs.SettingsAITestNoBaseURL()},
		{errors.New(`translation API returned 401 Unauthorized: {"error":{"message":"Incorrect API key provided"}}`), msgs.SettingsAITestUnauthorized()},
		{errors.New(`translation API returned 402 Payment Required: {"error":{"message":"Insufficient Balance"}}`), msgs.SettingsAITestQuota()},
		{errors.New(`translation API returned 429 Too Many Requests: {"error":{"type":"insufficient_quota"}}`), msgs.SettingsAITestQuota()},
		{errors.New(`translation API returned 404 Not Found: {"error":{"message":"The model gpt-x does not exist"}}`), msgs.SettingsAITestRejected()},
		{errors.New(`translation API returned 429 Too Many Requests: slow down`), msgs.SettingsAITestBusy()},
		{context.DeadlineExceeded, msgs.SettingsAITestTimeout()},
		{errors.New(`Post "https://llm.example.com/v1/chat/completions": dial tcp: lookup llm.example.com: no such host`), msgs.SettingsAITestNetwork()},
		{errors.New("decode translation response: invalid character '<' looking for beginning of value"), msgs.SettingsAITestMalformed()},
		{media.ErrEveryAPISignedOut, msgs.PrecheckEveryAPISignedOut()},
		{errors.New("something nobody has seen"), msgs.SettingsAITestFailed()},
	} {
		got := LLMTestError(msgs, tc.err)
		if got != tc.want {
			t.Errorf("LLMTestError(%v) = %q, want %q", tc.err, got, tc.want)
		}
		if tc.err != nil && strings.Contains(got, "translation API") {
			t.Errorf("LLMTestError(%v) shows the API's text: %q", tc.err, got)
		}
	}
}

// TestDefaultModelPrefersChatModels checks the model chosen for a service that has none yet: the preset's own when the service lists it, otherwise the first that looks like a chat model rather than an embedding, speech or image model, and the first listed only when none does.
func TestDefaultModelPrefersChatModels(t *testing.T) {
	for _, tc := range []struct {
		preset Preset
		models []string
		want   string
	}{
		{Preset{Model: "deepseek-chat"}, []string{"deepseek-chat", "deepseek-reasoner"}, "deepseek-chat"},
		{Preset{Model: "gone"}, []string{"babbage-002", "dall-e-3", "gpt-4.1-mini", "text-embedding-3-small"}, "gpt-4.1-mini"},
		{Preset{}, []string{"distil-whisper-large-v3-en", "llama-3.3-70b-versatile", "whisper-large-v3"}, "llama-3.3-70b-versatile"},
		{Preset{}, []string{"text-embedding-v3", "tts-1"}, "text-embedding-v3"},
		{Preset{Model: "m"}, nil, ""},
		// SiliconFlow's and Groq's lists, sorted as media's Models returns them, start with embedding, speech, image and guard models.
		{Preset{}, []string{"BAAI/bge-large-zh-v1.5", "BAAI/bge-m3", "BAAI/bge-reranker-v2-m3", "FunAudioLLM/CosyVoice2-0.5B", "FunAudioLLM/SenseVoiceSmall", "Kwai-Kolors/Kolors", "Qwen/Qwen2.5-7B-Instruct", "black-forest-labs/FLUX.1-schnell", "stabilityai/stable-diffusion-3-5-large"}, "Qwen/Qwen2.5-7B-Instruct"},
		{Preset{}, []string{"meta-llama/llama-guard-4-12b", "meta-llama/llama-prompt-guard-2-86m", "playai-tts", "qwen/qwen3-32b"}, "qwen/qwen3-32b"},
	} {
		if got := DefaultModel(tc.preset, tc.models); got != tc.want {
			t.Errorf("DefaultModel(%q, %q) = %q, want %q", tc.preset.Model, tc.models, got, tc.want)
		}
	}
}

// TestCloudPresetsNameAChatModel checks that every service in the cloud fills in a chat model of its own when it is chosen, rather than leaving the choice to the first id its list happens to sort first, which on several services is an embedding, image or guard model.
func TestCloudPresetsNameAChatModel(t *testing.T) {
	for _, preset := range Presets(testlocale.Messages()) {
		if preset.Group != GroupCloud {
			continue
		}
		if preset.Model == "" {
			t.Errorf("%s names no model", preset.ID)
			continue
		}
		if got := DefaultModel(Preset{}, []string{preset.Model}); got != preset.Model {
			t.Errorf("%s's model %q looks like one that does not chat", preset.ID, preset.Model)
		}
	}
}

// TestSpeechDownloadErrorSuggestsTheMirror checks that a download from Hugging Face whose connection failed or stalled, or that Hugging Face refused, suggests hf-mirror.com, offering to download the model from there (MirrorHelps), and that one which failed the checksum, from either source, says the file was deleted.
func TestSpeechDownloadErrorSuggestsTheMirror(t *testing.T) {
	m := testlocale.Messages()
	for _, tc := range []struct {
		err  error
		want string
	}{
		{DownloadError{Err: fmt.Errorf("%w: dial tcp: i/o timeout", media.ErrDisconnected)}, m.SettingsSpeechDownloadDisconnectedMirror()},
		{DownloadError{Err: media.ErrStalled}, m.SettingsSpeechDownloadStalledMirror()},
		{DownloadError{Err: fmt.Errorf("%w: ggml-tiny.bin has checksum 00, want ff", media.ErrSpeechModelMismatch)}, m.SettingsSpeechDownloadMismatch()},
		{DownloadError{Source: media.SpeechSourceMirror, Err: fmt.Errorf("%w: ggml-tiny.bin is 5 bytes, want 9", media.ErrSpeechModelMismatch)}, m.SettingsSpeechDownloadMismatch()},
		{DownloadError{Err: rejected(451)}, m.SettingsSpeechDownloadFailedStatusMirror(451)},
		{DownloadError{Source: media.SpeechSourceMirror, Err: rejected(503)}, m.SettingsSpeechDownloadFailedStatus(503)},
	} {
		if got := SpeechDownloadError(m, tc.err); got != tc.want {
			t.Errorf("SpeechDownloadError(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
	for _, tc := range []struct {
		err  error
		id   string
		want bool
	}{
		{DownloadError{ID: "tiny", Err: fmt.Errorf("%w: dial tcp: i/o timeout", media.ErrDisconnected)}, "tiny", true},
		{DownloadError{ID: "base", Err: media.ErrStalled}, "base", true},
		{DownloadError{ID: "small", Err: rejected(403)}, "small", true},
		{DownloadError{ID: "tiny", Source: media.SpeechSourceMirror, Err: media.ErrStalled}, "", false},
		{DownloadError{ID: "tiny", Err: fmt.Errorf("%w: checksum", media.ErrSpeechModelMismatch)}, "", false},
		{TrashError{ID: "tiny"}, "", false},
		{nil, "", false},
	} {
		if id, ok := MirrorHelps(tc.err); id != tc.id || ok != tc.want {
			t.Errorf("MirrorHelps(%v) = %q, %v, want %q, %v", tc.err, id, ok, tc.id, tc.want)
		}
	}
	if got := SpeechDownloadError(m, DownloadError{Err: media.ErrStalled}); !strings.Contains(got, "hf-mirror.com") {
		t.Errorf("a stall from Hugging Face says %q, which does not name hf-mirror.com", got)
	}
}
