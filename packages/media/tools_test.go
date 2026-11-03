package media

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToolsCheckListsEveryProblem(t *testing.T) {
	err := Tools{}.Check()
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"yt-dlp", "ffmpeg", "whisper-cli", "LINGOCUT_WHISPER_MODEL", "LINGOCUT_LLM_MODEL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

func TestToolsCheckPassesWhenConfigured(t *testing.T) {
	model := filepath.Join(t.TempDir(), "ggml-small.bin")
	if err := os.WriteFile(model, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	tools := Tools{YTDLP: "/bin/yt-dlp", FFmpeg: "/bin/ffmpeg", Whisper: "/bin/whisper-cli", WhisperModel: model, Translator: ChatTranslator{Model: "m"}}
	if err := tools.Check(); err != nil {
		t.Fatal(err)
	}
}

func setLLMEnv(t *testing.T, baseURL, apiKey, model, everyAPIKey string) {
	t.Setenv("LINGOCUT_LLM_BASE_URL", baseURL)
	t.Setenv("LINGOCUT_LLM_API_KEY", apiKey)
	t.Setenv("LINGOCUT_LLM_MODEL", model)
	t.Setenv("EVERYAPI_API_KEY", everyAPIKey)
}

func TestToolsFromEnv(t *testing.T) {
	t.Setenv("LINGOCUT_OUTPUT_DIR", "/out")
	t.Setenv("LINGOCUT_WHISPER_MODEL", "/models/small.bin")
	setLLMEnv(t, "", "", "", "")
	tools := ToolsFromEnv()
	if tools.OutputDir != "/out" || tools.WhisperModel != "/models/small.bin" {
		t.Fatalf("unexpected tools: %#v", tools)
	}
}

func TestToolsFromEnvTranslator(t *testing.T) {
	for _, tc := range []struct {
		name                                string
		baseURL, apiKey, model, everyAPIKey string
		want                                ChatTranslator
	}{
		{
			name:        "EveryAPI by default",
			everyAPIKey: "sk-everyapi",
			want:        ChatTranslator{BaseURL: EveryAPIBaseURL, APIKey: "sk-everyapi", Model: EveryAPIModel},
		},
		{
			name:        "EveryAPI with chosen key and model",
			apiKey:      "sk-lingocut",
			model:       "doubao-seed-2.0-mini",
			everyAPIKey: "sk-everyapi",
			want:        ChatTranslator{BaseURL: EveryAPIBaseURL, APIKey: "sk-lingocut", Model: "doubao-seed-2.0-mini"},
		},
		{
			name:        "custom provider ignores EveryAPI defaults",
			baseURL:     "http://localhost:11434/v1",
			everyAPIKey: "sk-everyapi",
			want:        ChatTranslator{BaseURL: "http://localhost:11434/v1"},
		},
		{
			name:    "custom provider",
			baseURL: "https://llm.example.com/v1",
			apiKey:  "key",
			model:   "m",
			want:    ChatTranslator{BaseURL: "https://llm.example.com/v1", APIKey: "key", Model: "m"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setLLMEnv(t, tc.baseURL, tc.apiKey, tc.model, tc.everyAPIKey)
			if got := ToolsFromEnv().Translator; got != tc.want {
				t.Fatalf("translator = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestToolsCheckRequiresEveryAPIKey(t *testing.T) {
	tools := Tools{Translator: ChatTranslator{BaseURL: EveryAPIBaseURL, Model: EveryAPIModel}}
	if err := tools.Check(); err == nil || !strings.Contains(err.Error(), "EVERYAPI_API_KEY") {
		t.Fatalf("err = %v", err)
	}
	tools.Translator = ChatTranslator{BaseURL: "http://localhost:11434/v1", Model: "qwen3"}
	if err := tools.Check(); err != nil && strings.Contains(err.Error(), "EVERYAPI_API_KEY") {
		t.Fatalf("custom provider asked for an EveryAPI key: %v", err)
	}
}

func TestLastLineAndTail(t *testing.T) {
	if got := lastLine("[download] 100%\n/videos/a.mp4\n\n"); got != "/videos/a.mp4" {
		t.Fatalf("lastLine = %q", got)
	}
	if got := tail("abcdef", 3); got != "...def" {
		t.Fatalf("tail = %q", got)
	}
	if got := tail(" short ", 10); got != "short" {
		t.Fatalf("tail = %q", got)
	}
}
