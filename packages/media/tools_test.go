package media

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"kando/packages/core"
)

func TestToolsCheckListsEveryProblem(t *testing.T) {
	err := Tools{}.Check()
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"yt-dlp", "ffmpeg", "whisper-cli", "KANDO_WHISPER_MODEL", "KANDO_LLM_MODEL"} {
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
	t.Setenv("KANDO_LLM_BASE_URL", baseURL)
	t.Setenv("KANDO_LLM_API_KEY", apiKey)
	t.Setenv("KANDO_LLM_MODEL", model)
	t.Setenv("EVERYAPI_API_KEY", everyAPIKey)
}

func TestToolsFromEnv(t *testing.T) {
	t.Setenv("KANDO_OUTPUT_DIR", "/out")
	t.Setenv("KANDO_WHISPER_MODEL", "/models/small.bin")
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
			apiKey:      "sk-kando",
			model:       "doubao-seed-2.0-mini",
			everyAPIKey: "sk-everyapi",
			want:        ChatTranslator{BaseURL: EveryAPIBaseURL, APIKey: "sk-kando", Model: "doubao-seed-2.0-mini"},
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
			got := ToolsFromEnv().Translator
			if got.Credential != nil || got.BaseURL != tc.want.BaseURL || got.APIKey != tc.want.APIKey || got.Model != tc.want.Model {
				t.Fatalf("translator = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestToolsCheckRequiresEveryAPIKey(t *testing.T) {
	tools := Tools{Translator: ChatTranslator{BaseURL: EveryAPIBaseURL, Model: EveryAPIModel}}
	if err := tools.Check(); err == nil || !strings.Contains(err.Error(), "everyapi auth login") {
		t.Fatalf("err = %v", err)
	}
	tools.Translator = ChatTranslator{BaseURL: "http://localhost:11434/v1", Model: "qwen3"}
	if err := tools.Check(); err != nil && strings.Contains(err.Error(), "everyapi auth login") {
		t.Fatalf("custom provider asked for an EveryAPI login: %v", err)
	}
}

func writeFakeEveryAPI(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "everyapi")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

func TestToolsFromEnvUsesSignedInCLI(t *testing.T) {
	writeFakeEveryAPI(t, `#!/bin/sh
if [ "$1" = auth ] && [ "$2" = credential ]; then
  printf '%s\n' '{"version":1,"base_url":"https://api.everyapi.ai/v1","api_key":"from-login"}'
  exit 0
fi
exit 1
`)
	setLLMEnv(t, "", "", "", "")
	tools := ToolsFromEnv()
	if tools.Translator.APIKey != "" || tools.Translator.Credential == nil {
		t.Fatalf("translator = %#v", tools.Translator)
	}
	got, err := tools.Translator.Credential(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if got != (Endpoint{BaseURL: EveryAPIBaseURL, APIKey: "from-login"}) {
		t.Fatalf("endpoint = %#v", got)
	}
	if err := tools.Check(); err != nil && strings.Contains(err.Error(), "EveryAPI") {
		t.Fatal(err)
	}
}

func TestToolsFromEnvReportsSignedOutCLI(t *testing.T) {
	writeFakeEveryAPI(t, `#!/bin/sh
echo 'EVERYAPI_CREDENTIAL_ERROR:not_logged_in: not logged in' >&2
exit 1
`)
	setLLMEnv(t, "", "", "", "")
	tools := ToolsFromEnv()
	_, err := tools.Translator.Credential(context.Background(), false)
	if err == nil || !strings.Contains(err.Error(), "everyapi auth login") {
		t.Fatalf("err = %v", err)
	}
}

func TestToolsTranscribeRejectsEmptyVideoPath(t *testing.T) {
	_, err := Tools{FFmpeg: "binary-that-does-not-exist", Whisper: "binary-that-does-not-exist"}.Transcribe(context.Background(), " ")
	if err == nil || !strings.Contains(err.Error(), "video path is empty") {
		t.Fatalf("err = %v", err)
	}
}

func TestToolsDownloadReturnsLocalFileWithoutYTDLP(t *testing.T) {
	video := filepath.Join(t.TempDir(), "talk.mp4")
	if err := os.WriteFile(video, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	url := "file://" + video
	got, err := (Tools{}).Download(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	if got != video {
		t.Fatalf("path = %q, want %q", got, video)
	}
}

func TestToolsDownloadUnescapesLocalFileURL(t *testing.T) {
	video := filepath.Join(t.TempDir(), "访谈 01.mp4")
	if err := os.WriteFile(video, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := (Tools{}).Download(context.Background(), (&url.URL{Scheme: "file", Path: video}).String())
	if err != nil {
		t.Fatal(err)
	}
	if got != video {
		t.Fatalf("path = %q, want %q", got, video)
	}
}

func TestLocalVideoPathRejectsRemoteFileHost(t *testing.T) {
	if path, ok := localVideoPath("file://other-machine/video.mp4"); ok || path != "" {
		t.Fatalf("path = %q, ok = %v", path, ok)
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

// fakeYTDLP writes a yt-dlp that prints the captured stdout and stderr of a real run, then exits with code.
func fakeYTDLP(t *testing.T, stdout, stderr string, code int) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "yt-dlp")
	script := "#!/bin/sh\ncat " + shellQuote(stdout) + "\ncat " + shellQuote(stderr) + " >&2\nexit " + strconv.Itoa(code) + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func shellQuote(path string) string {
	if path == "" {
		return "/dev/null"
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		panic(err)
	}
	return "'" + strings.ReplaceAll(abs, "'", `'\''`) + "'"
}

func TestDownloadFollowsTheRealOutput(t *testing.T) {
	tools := Tools{YTDLP: fakeYTDLP(t, "testdata/ytdlp/download-youtube.stdout", "", 0), OutputDir: t.TempDir()}
	var steps []core.Progress
	var info Info
	ctx := WithTrace(context.Background(), &Trace{Progress: func(p core.Progress) { steps = append(steps, p) }, Info: func(i Info) { info = i }})
	path, err := tools.Download(ctx, "https://www.youtube.com/watch?v=jNQXAC9IVRw")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/library/Me at the zoo [jNQXAC9IVRw]/video.mp4" || info.Title != "Me at the zoo" || len(steps) == 0 || steps[len(steps)-1].Fraction != 1 {
		t.Fatalf("path %q, info %+v, %d steps", path, info, len(steps))
	}
}

func TestDownloadNamesAFailedYouTubeVideoFromOEmbed(t *testing.T) {
	var asked string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.Query().Get("url")
		w.Write([]byte(`{"title":"Me at the zoo","thumbnail_url":"https://i.ytimg.com/vi/jNQXAC9IVRw/hqdefault.jpg"}`))
	}))
	t.Cleanup(srv.Close)
	tools := Tools{YTDLP: fakeYTDLP(t, "", "testdata/ytdlp/errors/outdated-youtube.stderr", 1), OutputDir: t.TempDir(), OEmbed: srv.URL}
	var info Info
	ctx := WithTrace(context.Background(), &Trace{Info: func(i Info) { info = i }})
	_, err := tools.Download(ctx, "https://youtu.be/jNQXAC9IVRw")
	if err == nil || !strings.Contains(err.Error(), "No video formats found") {
		t.Fatalf("err = %v", err)
	}
	if asked != "https://www.youtube.com/watch?v=jNQXAC9IVRw" || info.Title != "Me at the zoo" || info.Thumbnail == "" {
		t.Fatalf("asked %q, info %+v", asked, info)
	}
}

func TestDownloadDoesNotAskOEmbedForOtherSites(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("asked oEmbed for %s", r.URL)
	}))
	t.Cleanup(srv.Close)
	tools := Tools{YTDLP: fakeYTDLP(t, "", "testdata/ytdlp/errors/unsupported.stderr", 1), OutputDir: t.TempDir(), OEmbed: srv.URL}
	if _, err := tools.Download(context.Background(), "https://example.com/"); err == nil {
		t.Fatal("expected an error")
	}
}
