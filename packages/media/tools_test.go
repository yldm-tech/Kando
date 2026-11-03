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

func TestToolsFromEnv(t *testing.T) {
	t.Setenv("LINGOCUT_OUTPUT_DIR", "/out")
	t.Setenv("LINGOCUT_WHISPER_MODEL", "/models/small.bin")
	t.Setenv("LINGOCUT_LLM_BASE_URL", "")
	t.Setenv("LINGOCUT_LLM_API_KEY", "key")
	t.Setenv("LINGOCUT_LLM_MODEL", "gpt")
	tools := ToolsFromEnv()
	if tools.OutputDir != "/out" || tools.WhisperModel != "/models/small.bin" {
		t.Fatalf("unexpected tools: %#v", tools)
	}
	if tools.Translator != (ChatTranslator{BaseURL: "https://api.openai.com/v1", APIKey: "key", Model: "gpt"}) {
		t.Fatalf("unexpected translator: %#v", tools.Translator)
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
