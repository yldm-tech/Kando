package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kando/packages/core"
)

func TestToolsCheckListsEveryProblem(t *testing.T) {
	err := Tools{}.Check()
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"yt-dlp", "ffmpeg", "whisper-cli", "KANDO_WHISPER_MODEL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
	// A job without a translator still makes the original-language subtitles; Run checks the translator before translating.
	if strings.Contains(err.Error(), "KANDO_LLM_MODEL") {
		t.Errorf("error %q asks for the translator", err)
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

// Translating a done film again needs only the translator: CheckTranslate passes without the download and speech tools, and still says when the translator has no model.
func TestToolsCheckTranslateNeedsOnlyTheTranslator(t *testing.T) {
	if err := (Tools{Translator: ChatTranslator{Model: "m"}}).CheckTranslate(); err != nil {
		t.Fatal(err)
	}
	err := Tools{}.CheckTranslate()
	if err == nil || !strings.Contains(err.Error(), "KANDO_LLM_MODEL") {
		t.Fatalf("err %v", err)
	}
	for _, tool := range []string{"yt-dlp", "ffmpeg", "whisper"} {
		if strings.Contains(err.Error(), tool) {
			t.Errorf("error %q mentions %s", err, tool)
		}
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

func TestToolsCheckTranslateRequiresEveryAPIKey(t *testing.T) {
	tools := Tools{Translator: ChatTranslator{BaseURL: EveryAPIBaseURL, Model: EveryAPIModel}}
	if err := tools.CheckTranslate(); !errors.Is(err, ErrEveryAPILoginSignedOut) || strings.Contains(err.Error(), "everyapi auth") {
		t.Fatalf("err = %v, want %v", err, ErrEveryAPILoginSignedOut)
	}
	tools.Translator = ChatTranslator{BaseURL: "http://localhost:11434/v1", Model: "qwen3"}
	if err := tools.CheckTranslate(); errors.Is(err, ErrEveryAPIMissing) || errors.Is(err, ErrEveryAPISignedOut) {
		t.Fatalf("custom provider asked for the everyapi program: %v", err)
	}
}

// noPackageManagerDirs keeps lookTool to PATH for the test, so an everyapi that Homebrew installed on the machine running it does not count.
func noPackageManagerDirs(t *testing.T) {
	t.Helper()
	saved := extraToolDirs
	extraToolDirs = nil
	t.Cleanup(func() { extraToolDirs = saved })
}

// signedInEveryAPI is an everyapi someone signed in to: `everyapi auth credential` prints the relay key.
var signedInEveryAPI = fakeTool{Want: []string{"auth", "credential"}, Stdout: `{"version":1,"base_url":"https://api.everyapi.ai/v1","api_key":"from-login"}` + "\n"}

// TestRelookFindsTheEveryAPICLIInstalledLater checks that without the everyapi program, and without Kando's own sign-in, CheckTranslate says EveryAPI is not signed in, in words that name no program, and that once it is installed Relook signs the translator in through it without Kando starting again.
func TestRelookFindsTheEveryAPICLIInstalledLater(t *testing.T) {
	noPackageManagerDirs(t)
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	setLLMEnv(t, "", "", "", "")
	tools := ToolsFromEnv()
	if tools.EveryAPI != "" || tools.Translator.Credential != nil {
		t.Fatalf("found an everyapi in an empty PATH: %q", tools.EveryAPI)
	}
	if err := tools.CheckTranslate(); !errors.Is(err, ErrEveryAPILoginSignedOut) || errors.Is(err, ErrEveryAPISignedOut) || strings.Contains(err.Error(), "command-line") {
		t.Fatalf("CheckTranslate = %v, want %v", err, ErrEveryAPILoginSignedOut)
	}

	cli := writeFakeTool(t, dir, "everyapi", signedInEveryAPI)
	tools = tools.Relook()
	if tools.EveryAPI != cli || tools.Translator.Credential == nil {
		t.Fatalf("Relook: everyapi %q, credential %v", tools.EveryAPI, tools.Translator.Credential != nil)
	}
	got, err := tools.Translator.Credential(context.Background(), false)
	if err != nil || got.APIKey != "from-login" {
		t.Fatalf("credential = %#v, %v", got, err)
	}

	// Once it is uninstalled again, Relook takes the sign-in away.
	if err := os.Remove(cli); err != nil {
		t.Fatal(err)
	}
	if tools = tools.Relook(); tools.EveryAPI != "" || tools.Translator.Credential != nil {
		t.Fatalf("Relook kept a removed everyapi: %q", tools.EveryAPI)
	}
}

// TestUseEveryAPICLIOnlyWithoutAKey checks that the everyapi sign-in stands in for EveryAPI's key only: a key, or another service, takes it away.
func TestUseEveryAPICLIOnlyWithoutAKey(t *testing.T) {
	tools := Tools{EveryAPI: "/bin/everyapi", Translator: ChatTranslator{BaseURL: EveryAPIBaseURL, Model: EveryAPIModel}}
	if tools.UseEveryAPICLI().Translator.Credential == nil {
		t.Fatal("EveryAPI without a key did not sign in through everyapi")
	}
	tools.Translator.APIKey = "k"
	if tools.UseEveryAPICLI().Translator.Credential != nil {
		t.Fatal("EveryAPI with a key still signs in through everyapi")
	}
	tools.Translator = ChatTranslator{BaseURL: "https://api.openai.com/v1", Model: "gpt"}
	if tools.UseEveryAPICLI().Translator.Credential != nil {
		t.Fatal("another service signs in through everyapi")
	}
}

// TestEveryAPICredentialPrecedence checks the order a key or sign-in is used in for EveryAPI: a key first (KANDO_LLM_API_KEY, EVERYAPI_API_KEY and the saved key all reach the translator as its APIKey), then Kando's own sign-in, then the everyapi program's; that finding the program again keeps Kando's sign-in; and that signing out of Kando falls back to the program's.
func TestEveryAPICredentialPrecedence(t *testing.T) {
	var asked []*Endpoint
	login := EveryAPILogin(func(_ context.Context, rejected *Endpoint) (Endpoint, error) {
		asked = append(asked, rejected)
		return Endpoint{BaseURL: EveryAPIBaseURL, APIKey: "sk-login"}, nil
	})
	tools := Tools{EveryAPI: "/bin/everyapi", Translator: ChatTranslator{BaseURL: EveryAPIBaseURL, Model: EveryAPIModel}}.UseEveryAPICLI()
	if tools.Translator.CredentialSource != CredentialCLI {
		t.Fatalf("source = %v, want the program's", tools.Translator.CredentialSource)
	}
	tools = tools.UseEveryAPILogin(login)
	if tools.Translator.CredentialSource != CredentialLogin || tools.Translator.Renew == nil {
		t.Fatalf("source = %v, want Kando's own", tools.Translator.CredentialSource)
	}
	if got, err := tools.Translator.Credential(context.Background(), false); err != nil || got.APIKey != "sk-login" || len(asked) != 1 || asked[0] != nil {
		t.Fatalf("credential = %#v, %v, asked %v", got, err, asked)
	}
	// Finding the programs again, or changing the endpoint back, keeps Kando's sign-in.
	if again := tools.UseEveryAPICLI(); again.Translator.CredentialSource != CredentialLogin {
		t.Fatalf("UseEveryAPICLI dropped Kando's sign-in: %v", again.Translator.CredentialSource)
	}
	// A key comes first.
	keyed := tools
	keyed.Translator.APIKey = "sk-key"
	if keyed = keyed.UseEveryAPICLI(); keyed.Translator.Credential != nil || keyed.Translator.CredentialSource != CredentialNone {
		t.Fatalf("a key still signs in: %v", keyed.Translator.CredentialSource)
	}
	// Another service takes no sign-in, and choosing EveryAPI again brings Kando's back.
	other := tools
	other.Translator.BaseURL = "https://api.openai.com/v1"
	if other = other.UseEveryAPICLI(); other.Translator.Credential != nil {
		t.Fatal("another service signs in through EveryAPI")
	}
	other.Translator.BaseURL = EveryAPIBaseURL
	if other = other.UseEveryAPICLI(); other.Translator.CredentialSource != CredentialLogin {
		t.Fatal("EveryAPI chosen again lost Kando's sign-in")
	}
	// Signing out of Kando falls back to the program's sign-in, and without the program to none.
	if out := tools.UseEveryAPILogin(nil); out.Translator.CredentialSource != CredentialCLI || out.EveryAPILogin != nil {
		t.Fatalf("signed out: source %v", out.Translator.CredentialSource)
	}
	tools.EveryAPI = ""
	if out := tools.UseEveryAPILogin(nil); !out.Translator.EveryAPIUnsigned() || !errors.Is(out.CheckTranslate(), ErrEveryAPILoginSignedOut) {
		t.Fatalf("signed out without the program: source %v", out.Translator.CredentialSource)
	}
}

// TestEveryAPILoginRenewsTheRejectedKey checks that a request the API refused asks Kando's sign-in for a key in place of the one it sent, naming it, and is sent again with the new one.
func TestEveryAPILoginRenewsTheRejectedKey(t *testing.T) {
	var rejected []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-new" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer server.Close()
	login := EveryAPILogin(func(_ context.Context, old *Endpoint) (Endpoint, error) {
		if old == nil {
			return Endpoint{BaseURL: server.URL, APIKey: "sk-old"}, nil
		}
		rejected = append(rejected, old.APIKey)
		return Endpoint{BaseURL: server.URL, APIKey: "sk-new"}, nil
	})
	tools := Tools{Translator: ChatTranslator{BaseURL: EveryAPIBaseURL, Model: "m", Client: server.Client()}}.UseEveryAPILogin(login)
	reply, err := tools.Translator.Chat(context.Background(), "system", "hi")
	if err != nil || reply != "ok" {
		t.Fatalf("Chat = %q, %v", reply, err)
	}
	if len(rejected) != 1 || rejected[0] != "sk-old" {
		t.Fatalf("renewed for %v, want the key sent", rejected)
	}
}

// everyAPIRefusal is EveryAPI's 403 for a request refused over money, with its quota_reason.
func everyAPIRefusal(reason string) string {
	return `{"error":{"message":"quota refused","type":"everyapi_error","code":"insufficient_user_quota","quota_reason":"` + reason + `"}}`
}

// TestEveryAPIQuotaReasons checks how EveryAPI's 403 over money reads: an empty wallet and a spent plan are the account out of credit, which no retry mends, while a key at its own cap is not, as a new key resets it; and the other services' answers read as before.
func TestEveryAPIQuotaReasons(t *testing.T) {
	for _, tc := range []struct {
		body string
		want TranslateFail
	}{
		{everyAPIRefusal(QuotaBalance), TranslateQuota},
		{everyAPIRefusal(QuotaPlan), TranslateQuota},
		{`{"error":{"code":"pre_consume_token_quota_failed","message":"pre-consume failed"}}`, TranslateQuota},
		{everyAPIRefusal(QuotaKeyCap), TranslateAuth},
		{`{"error":{"code":"insufficient_quota","message":"You exceeded your current quota"}}`, TranslateQuota},
		{`{"error":{"message":"forbidden"}}`, TranslateAuth},
	} {
		msg := (&statusError{code: http.StatusForbidden, status: "403 Forbidden", detail: tc.body}).Error()
		if got := ClassifyTranslate(msg); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.body, got, tc.want)
		}
	}
	if EveryAPIQuotaReason(everyAPIRefusal(QuotaPlan)) != QuotaPlan || EveryAPIQuotaReason(`{"error":{}}`) != "" {
		t.Fatal("quota_reason misread")
	}
}

// TestEveryAPILoginRenewsAKeyAtItsCap checks that a request EveryAPI refused because the key's own cap is reached renews the key once, naming the one it sent, and goes through with the new one, while an empty wallet is not renewed for.
func TestEveryAPILoginRenewsAKeyAtItsCap(t *testing.T) {
	var reason atomic.Value
	reason.Store(QuotaKeyCap)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer sk-new" && reason.Load() == QuotaKeyCap {
			_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, everyAPIRefusal(reason.Load().(string)))
	}))
	defer server.Close()
	var rejected []string
	login := EveryAPILogin(func(_ context.Context, old *Endpoint) (Endpoint, error) {
		if old == nil {
			return Endpoint{BaseURL: server.URL, APIKey: "sk-old"}, nil
		}
		rejected = append(rejected, old.APIKey)
		return Endpoint{BaseURL: server.URL, APIKey: "sk-new"}, nil
	})
	tools := Tools{Translator: ChatTranslator{BaseURL: EveryAPIBaseURL, Model: "m", Client: server.Client(), Attempts: 1}}.UseEveryAPILogin(login)
	if reply, err := tools.Translator.Chat(context.Background(), "system", "hi"); err != nil || reply != "ok" || len(rejected) != 1 || rejected[0] != "sk-old" {
		t.Fatalf("key at its cap: %q, %v, renewed for %v", reply, err, rejected)
	}
	rejected = nil
	reason.Store(QuotaBalance)
	_, err := tools.Translator.Chat(context.Background(), "system", "hi")
	if ClassifyTranslate(fmt.Sprint(err)) != TranslateQuota || len(rejected) != 0 {
		t.Fatalf("empty wallet: %v, renewed for %v", err, rejected)
	}
}

// TestEveryAPICredentialGoneSaysMissing checks that an everyapi removed after Kando found it is reported as missing, not as signed out.
func TestEveryAPICredentialGoneSaysMissing(t *testing.T) {
	cli := writeFakeTool(t, t.TempDir(), "everyapi", signedInEveryAPI)
	if err := os.Remove(cli); err != nil {
		t.Fatal(err)
	}
	_, err := everyAPICredential(cli)(context.Background(), false)
	if !errors.Is(err, ErrEveryAPIMissing) {
		t.Fatalf("err = %v, want %v", err, ErrEveryAPIMissing)
	}
}

// writeFakeEveryAPI puts a fake everyapi that does what fake says alone on PATH.
func writeFakeEveryAPI(t *testing.T, fake fakeTool) {
	t.Helper()
	dir := t.TempDir()
	writeFakeTool(t, dir, "everyapi", fake)
	t.Setenv("PATH", dir)
}

func TestToolsFromEnvUsesSignedInCLI(t *testing.T) {
	writeFakeEveryAPI(t, signedInEveryAPI)
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
	if err := tools.CheckTranslate(); err != nil && strings.Contains(err.Error(), "EveryAPI") {
		t.Fatal(err)
	}
}

func TestToolsFromEnvReportsSignedOutCLI(t *testing.T) {
	writeFakeEveryAPI(t, fakeTool{Stderr: "EVERYAPI_CREDENTIAL_ERROR:not_logged_in: not logged in\n", Exit: 1})
	setLLMEnv(t, "", "", "", "")
	tools := ToolsFromEnv()
	_, err := tools.Translator.Credential(context.Background(), false)
	if !errors.Is(err, ErrEveryAPISignedOut) || !strings.Contains(err.Error(), "everyapi auth login") {
		t.Fatalf("err = %v, want %v", err, ErrEveryAPISignedOut)
	}
}

func TestToolsHearRejectsEmptyVideoPath(t *testing.T) {
	_, err := Tools{FFmpeg: "binary-that-does-not-exist", Whisper: "binary-that-does-not-exist"}.Hear(context.Background(), " ", 0, 0, func(core.Segment) {})
	if err == nil || !strings.Contains(err.Error(), "video path is empty") {
		t.Fatalf("err = %v", err)
	}
}

// transcribeTools writes an ffmpeg that does nothing and a whisper-cli that saves its arguments to a file, one a line, prints the one line it hears, and writes a transcript of it where -of asks, as the real one does with -np and -oj.
func transcribeTools(t *testing.T) (tools Tools, args string) {
	t.Helper()
	dir := t.TempDir()
	args = filepath.Join(dir, "args")
	ffmpeg := writeFakeTool(t, dir, "ffmpeg", fakeTool{})
	whisper := writeFakeTool(t, dir, "whisper-cli", fakeTool{Args: args, Act: "transcribe", Stdout: "[00:00:00.000 --> 00:00:01.500]   Hello\n", Transcript: `{"result":{"language":"en"},"transcription":[{"offsets":{"from":0,"to":1500},"text":" Hello"}]}`})
	return Tools{FFmpeg: ffmpeg, Whisper: whisper, WhisperModel: filepath.Join(dir, "ggml-base.bin")}, args
}

// whisperLanguage is the value of -l in the arguments file transcribeTools' whisper-cli wrote.
func whisperLanguage(t *testing.T, args string) string {
	t.Helper()
	data, err := os.ReadFile(args)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for i, line := range lines {
		if line == "-l" && i+1 < len(lines) {
			return lines[i+1]
		}
	}
	t.Fatalf("no -l in %q", lines)
	return ""
}

func TestHearNamesTheSpokenLanguage(t *testing.T) {
	tools, args := transcribeTools(t)
	video := filepath.Join(t.TempDir(), "talk.mp4")
	for _, tc := range []struct{ language, want string }{{"", "auto"}, {"ja", "ja"}, {" zh ", "zh"}} {
		tools.SpeechLanguage = tc.language
		var segments []core.Segment
		heard, err := tools.Hear(context.Background(), video, 0, 0, func(s core.Segment) { segments = append(segments, s) })
		if err != nil {
			t.Fatal(err)
		}
		// The language whisper's output says it heard is the one Hear returns.
		if heard != "en" {
			t.Fatalf("language %q: heard %q, want en", tc.language, heard)
		}
		if len(segments) != 1 || segments[0].Text != "Hello" {
			t.Fatalf("segments %+v", segments)
		}
		if got := whisperLanguage(t, args); got != tc.want {
			t.Fatalf("language %q: -l %q, want %q", tc.language, got, tc.want)
		}
	}
}

// whisperPipeline is a pipeline whose speech recognition is Tools', so a test reads the -l whisper was given.
type whisperPipeline struct {
	fakePipeline
	tools Tools
}

func (p *whisperPipeline) Hear(ctx context.Context, video string, from, to time.Duration, heard func(core.Segment)) (string, error) {
	return p.tools.Hear(ctx, video, from, to, heard)
}

// TestMakeHearsTheLanguageTheJobNames checks that a job whose film the user said is spoken in a language of its own is heard in that language rather than the one the settings name for every video, that a job naming none is transcribed in the settings', and that the job keeps what it named.
func TestMakeHearsTheLanguageTheJobNames(t *testing.T) {
	tools, args := transcribeTools(t)
	tools.SpeechLanguage = "ja"
	for _, tc := range []struct{ job, want string }{{"", "ja"}, {"en", "en"}, {" ko ", "ko"}} {
		job, _ := core.NewJob("https://example.com/video")
		job.SpeechLanguage = tc.job
		final := Make(context.Background(), &whisperPipeline{fakePipeline: fakePipeline{dir: t.TempDir()}, tools: tools}, job, "简体中文", "mac", nil, func(core.Job) {})
		if final.Stage != core.StageComplete || final.SpeechLanguage != tc.job {
			t.Fatalf("job naming %q: %+v", tc.job, final)
		}
		if got := whisperLanguage(t, args); got != tc.want {
			t.Fatalf("job naming %q: -l %q, want %q", tc.job, got, tc.want)
		}
	}
}

// A video with no sound track makes ffmpeg write no stream; Hear returns ErrNoAudio rather than ffmpeg's words.
func TestHearSaysTheVideoHasNoSound(t *testing.T) {
	dir := t.TempDir()
	ffmpeg := writeFakeTool(t, dir, "ffmpeg", fakeTool{Stderr: "[out#0/wav @ 0x1] Output file does not contain any stream\nError opening output files: Invalid argument\n", Exit: 234})
	tools := Tools{FFmpeg: ffmpeg, Whisper: filepath.Join(dir, "whisper-cli"), WhisperModel: filepath.Join(dir, "model.bin")}
	_, err := tools.Hear(context.Background(), filepath.Join(dir, "silent.mp4"), 0, 0, func(core.Segment) {})
	if !errors.Is(err, ErrNoAudio) {
		t.Fatalf("err %v, want ErrNoAudio", err)
	}
}

func TestSpeechLanguagesAreKnown(t *testing.T) {
	for _, code := range SpeechLanguages() {
		if !KnownSpeechLanguage(code) {
			t.Fatalf("%q is not known", code)
		}
	}
	for _, code := range []string{"", "auto", "xx", "EN"} {
		if KnownSpeechLanguage(code) {
			t.Fatalf("%q is known", code)
		}
	}
}

func TestWhisperThreadCountUsesAvailableCPU(t *testing.T) {
	want := runtime.NumCPU()
	if want < 1 {
		want = 1
	}
	if want > 8 {
		want = 8
	}
	if got := whisperThreadCount(); got != want {
		t.Fatalf("threads = %d, want %d", got, want)
	}
}

func TestToolsDownloadReturnsLocalFileWithoutYTDLP(t *testing.T) {
	video := filepath.Join(t.TempDir(), "talk.mp4")
	if err := os.WriteFile(video, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	url := core.FileURL(video)
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
	got, err := (Tools{}).Download(context.Background(), core.FileURL(video))
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

// fakeYTDLP writes a yt-dlp that prints the captured stdout and stderr of a real run, from the files stdout and stderr ("" for nothing), then exits with code.
func fakeYTDLP(t *testing.T, stdout, stderr string, code int) string {
	t.Helper()
	return writeFakeTool(t, t.TempDir(), "yt-dlp", fakeTool{StdoutFile: stdout, StderrFile: stderr, Exit: code})
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

// Every download asks yt-dlp for one video at most, so a link to a whole list on a site Kando does not know lists of downloads its first video instead of every one into the library. yt-dlp exits with 101 once it has downloaded that video, as it does with a real download (checked against yt-dlp 2026.08.19), which is a download that worked.
func TestDownloadStopsAfterOneVideo(t *testing.T) {
	tool, args := argsYTDLP(t)
	tools := Tools{YTDLP: tool, OutputDir: t.TempDir()}
	tools.Download(context.Background(), "https://vimeo.com/showcase/1234")
	if data, _ := os.ReadFile(args); !strings.Contains(string(data), "--max-downloads\n1\n") {
		t.Fatalf("args %s", data)
	}

	tools.YTDLP = fakeYTDLP(t, "testdata/ytdlp/download-youtube.stdout", "", ytdlpMaxDownloadsExit)
	path, err := tools.Download(context.Background(), "https://www.youtube.com/watch?v=jNQXAC9IVRw")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/library/Me at the zoo [jNQXAC9IVRw]/video.mp4" {
		t.Fatalf("path %q", path)
	}

	// Without the video's path, or with another failure, the exit is an error as before.
	tools.YTDLP = fakeYTDLP(t, "", "", ytdlpMaxDownloadsExit)
	if _, err := tools.Download(context.Background(), "https://www.youtube.com/watch?v=jNQXAC9IVRw"); err == nil {
		t.Fatal("exit 101 without a downloaded video reported success")
	}
	tools.YTDLP = fakeYTDLP(t, "testdata/ytdlp/download-youtube.stdout", "", 1)
	if _, err := tools.Download(context.Background(), "https://www.youtube.com/watch?v=jNQXAC9IVRw"); err == nil {
		t.Fatal("exit 1 reported success")
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

// argsYTDLP writes a yt-dlp that saves its arguments to a file, one a line, and fails.
func argsYTDLP(t *testing.T) (tool, args string) {
	t.Helper()
	dir := t.TempDir()
	args = filepath.Join(dir, "args")
	return writeFakeTool(t, dir, "yt-dlp", fakeTool{Args: args, Exit: 1}), args
}

func TestDownloadBorrowsTheBrowsersLogin(t *testing.T) {
	tool, args := argsYTDLP(t)
	tools := Tools{YTDLP: tool, OutputDir: t.TempDir(), Cookies: "chrome"}
	tools.Download(context.Background(), "https://www.bilibili.com/video/BV1xx411c7mD")
	data, _ := os.ReadFile(args)
	if !strings.Contains(string(data), "--cookies-from-browser\nchrome\n") {
		t.Fatalf("args %s", data)
	}
	tools.Cookies = ""
	tools.Download(context.Background(), "https://www.bilibili.com/video/BV1xx411c7mD")
	data, _ = os.ReadFile(args)
	if strings.Contains(string(data), "--cookies-from-browser") {
		t.Fatalf("args without cookies %s", data)
	}
}

// An app opened from Finder has a PATH without Homebrew's folder, where yt-dlp would look for ffmpeg to merge the video with its sound; Download names the ffmpeg Kando found.
func TestDownloadTellsYTDLPWhereFFmpegIs(t *testing.T) {
	tool, args := argsYTDLP(t)
	tools := Tools{YTDLP: tool, FFmpeg: "/opt/homebrew/bin/ffmpeg", OutputDir: t.TempDir()}
	tools.Download(context.Background(), "https://www.youtube.com/watch?v=jNQXAC9IVRw")
	data, _ := os.ReadFile(args)
	if !strings.Contains(string(data), "--ffmpeg-location\n/opt/homebrew/bin/ffmpeg\n") {
		t.Fatalf("args %s", data)
	}
}

// yt-dlp looks for Deno, which solves YouTube's JavaScript challenges, only on PATH, which an app opened from Finder does not extend to Homebrew's folder, and Kando's own Deno is on neither; Download, the previews, the lists, the streams and the platform's subtitles name the Deno Kando found, and none when it found none, which leaves yt-dlp to look on PATH.
func TestYTDLPIsToldWhereDenoIs(t *testing.T) {
	tool, args := argsYTDLP(t)
	ctx := context.Background()
	source := "https://www.youtube.com/watch?v=jNQXAC9IVRw"
	for name, run := range map[string]func(Tools){
		"download":  func(tools Tools) { tools.Download(ctx, source) },
		"preview":   func(tools Tools) { tools.Preview(ctx, source) },
		"list":      func(tools Tools) { tools.ListVideos(ctx, "https://www.youtube.com/playlist?list=PL1", 0) },
		"stream":    func(tools Tools) { tools.StreamURL(ctx, source) },
		"subtitles": func(tools Tools) { tools.PlatformSubtitles(ctx, source, "en") },
	} {
		os.Remove(args)
		run(Tools{YTDLP: tool, Deno: "/opt/homebrew/bin/deno", OutputDir: t.TempDir()})
		data, _ := os.ReadFile(args)
		if !strings.Contains(string(data), "--js-runtimes\ndeno:/opt/homebrew/bin/deno\n") {
			t.Errorf("%s: args %s", name, data)
		}
		os.Remove(args)
		run(Tools{YTDLP: tool, OutputDir: t.TempDir()})
		data, _ = os.ReadFile(args)
		if len(data) == 0 || strings.Contains(string(data), "--js-runtimes") {
			t.Errorf("%s without Deno: args %s", name, data)
		}
	}
}

// What yt-dlp printed on 2026-11-28 for a signed-in YouTube download without a JavaScript runtime (2026.10.26, with no Deno on its PATH) and with solver scripts older than YouTube's player (Homebrew's 2026.08.19 with Deno): signed in, yt-dlp does not say it found no runtime, only that the challenge failed. Without a Deno from Kando the failure says it found none (ErrNoJSRuntime), which the job's card tells apart from an outdated yt-dlp (downloadFailKind); with one, it is an outdated yt-dlp. Signed out, yt-dlp says it found no runtime.
func TestDownloadSaysItFoundNoJSRuntime(t *testing.T) {
	const noRuntime = "WARNING: [youtube] No supported JavaScript runtime could be found. Only deno is enabled by default; to use another runtime add  --js-runtimes RUNTIME[:PATH]  to your command/config. YouTube extraction without a JS runtime has been deprecated, and some formats may be missing. See  https://github.com/yt-dlp/yt-dlp/wiki/EJS  for details on installing one\n"
	const challenge = "WARNING: [youtube] jNQXAC9IVRw: n challenge solving failed: Some formats may be missing. Ensure you have a supported JavaScript runtime and challenge solver script distribution installed. Review any warnings presented before this message. For more details, refer to  https://github.com/yt-dlp/yt-dlp/wiki/EJS\n"
	const reload = "ERROR: [youtube] jNQXAC9IVRw: The page needs to be reloaded.\n"
	const noFormats = "ERROR: [youtube] jNQXAC9IVRw: No video formats found!; please report this issue on  https://github.com/yt-dlp/yt-dlp/issues?q= , filling out the appropriate issue template. Confirm you are on the latest version using  yt-dlp -U\n"
	const private = "ERROR: [youtube] jNQXAC9IVRw: Private video. Sign in if you've been granted access to this video\n"
	for _, c := range []struct {
		stderr, deno string
		want         core.FailKind
	}{
		{challenge + noFormats, "", core.FailNoJSRuntime},
		{challenge + reload, "", core.FailNoJSRuntime},
		{noRuntime + noFormats, "", core.FailNoJSRuntime},
		{challenge + reload, "/opt/homebrew/bin/deno", core.FailOutdated},
		{challenge + noFormats, "/opt/homebrew/bin/deno", core.FailOutdated},
		// A reason yt-dlp could tell stays that reason.
		{noRuntime + private, "", core.FailPrivate},
		// Without any word of the challenge it is not the runtime.
		{reload, "", core.FailOutdated},
	} {
		dir := t.TempDir()
		tool := writeFakeTool(t, dir, "yt-dlp", fakeTool{Stderr: c.stderr, Exit: 1})
		_, err := Tools{YTDLP: tool, Deno: c.deno, OutputDir: t.TempDir()}.Download(context.Background(), "https://youtu.be/jNQXAC9IVRw")
		if err == nil {
			t.Fatal("download succeeded")
		}
		if got := downloadFailKind(core.Job{Cookies: "chrome"}, err); got != c.want {
			t.Errorf("%q with Deno %q: kind %q, want %q (%v)", c.stderr, c.deno, got, c.want, err)
		}
	}
}

// yt-dlp keeps a split-format video's separate tracks and its .part files in a folder of its own outside the library, so the library watcher only ever sees the merged video; the folder is the same for the same video, so a retry picks up where the last try stopped.
func TestDownloadKeepsPartialFilesOutsideTheLibrary(t *testing.T) {
	tool, args := argsYTDLP(t)
	library, cache := t.TempDir(), t.TempDir()
	tools := Tools{YTDLP: tool, OutputDir: library, DownloadTemp: cache}
	temps := map[string]string{}
	for _, source := range []string{"https://www.youtube.com/watch?v=jNQXAC9IVRw", "https://www.youtube.com/watch?v=jNQXAC9IVRw", "https://www.youtube.com/watch?v=dQw4w9WgXcQ"} {
		tools.Download(context.Background(), source)
		home, temp := ytdlpPaths(t, args)
		if home != library || !filepath.IsAbs(temp) || !pathInside(cache, temp) {
			t.Fatalf("home %q temp %q", home, temp)
		}
		if seen, ok := temps[source]; ok && seen != temp {
			t.Fatalf("%s: temp %q then %q", source, seen, temp)
		}
		temps[source] = temp
	}
	if temps["https://www.youtube.com/watch?v=jNQXAC9IVRw"] == temps["https://www.youtube.com/watch?v=dQw4w9WgXcQ"] {
		t.Fatalf("two videos share a temp folder: %v", temps)
	}
}

// ytdlpPaths reads the home path (-P) and the temp path (-P temp:) from the arguments argsYTDLP saved.
func ytdlpPaths(t *testing.T, args string) (home, temp string) {
	t.Helper()
	data, err := os.ReadFile(args)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")
	for i := 0; i+1 < len(lines); i++ {
		if lines[i] != "-P" {
			continue
		}
		if rest, ok := strings.CutPrefix(lines[i+1], "temp:"); ok {
			temp = rest
		} else {
			home = lines[i+1]
		}
	}
	return home, temp
}

// yt-dlp moves the merged video into the library with a rename only when its temp path is on the library's volume; across volumes it copies the video into video.mp4 in place, so a download stopped during the copy leaves a cut-off video.mp4 that the next try takes for a finished one. When DownloadTemp is not on the library's volume, or cannot be told to be, the partial files go to a hidden folder in the library, which the library never lists, and the folder is gone once the video is in.
func TestDownloadKeepsPartialFilesOnTheLibrarysVolume(t *testing.T) {
	notAFolder := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notAFolder, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	tool, args := argsYTDLP(t)
	library := t.TempDir()
	tools := Tools{YTDLP: tool, OutputDir: library, DownloadTemp: filepath.Join(notAFolder, "downloads")}
	tools.Download(context.Background(), "https://www.youtube.com/watch?v=jNQXAC9IVRw")
	if _, temp := ytdlpPaths(t, args); !pathInside(filepath.Join(library, ".kando-downloads"), temp) {
		t.Fatalf("temp %q", temp)
	}
	tools.YTDLP, _ = splitYTDLP(t)
	path, err := tools.Download(context.Background(), "https://www.youtube.com/watch?v=jNQXAC9IVRw")
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != "track 395track 251" {
		t.Fatalf("%s = %q", path, data)
	}
	if entries, _ := os.ReadDir(library); len(entries) != 1 || entries[0].Name() != filepath.Base(filepath.Dir(path)) {
		t.Fatalf("library holds %v", entries)
	}
}

// splitYTDLP writes a yt-dlp that downloads a split-format video as the real one does (fakeDownload). Each run appends a line to the returned file.
func splitYTDLP(t *testing.T) (tool, runs string) {
	t.Helper()
	dir := t.TempDir()
	runs = filepath.Join(dir, "runs")
	return writeFakeTool(t, dir, "yt-dlp", fakeTool{Runs: runs, Act: "download"}), runs
}

func TestDownloadLeavesNoTemporaryFolderBehind(t *testing.T) {
	tool, _ := splitYTDLP(t)
	library, cache := t.TempDir(), t.TempDir()
	tools := Tools{YTDLP: tool, OutputDir: library, DownloadTemp: cache}
	path, err := tools.Download(context.Background(), "https://www.youtube.com/watch?v=jNQXAC9IVRw")
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != "track 395track 251" {
		t.Fatalf("%s = %q", path, data)
	}
	if entries, _ := os.ReadDir(cache); len(entries) != 0 {
		t.Fatalf("temp folders left: %v", entries)
	}
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
		t.Fatalf("library folder holds %v", entries)
	}
}

// An earlier Kando renamed the video track of an unfinished download to video.mp4 and left the sound track beside it. yt-dlp takes that silent file for a finished download, so a retry would transcribe it forever; Download notices the leftover track, removes both and downloads the video again.
func TestDownloadReplacesAVideoLeftWithoutItsSound(t *testing.T) {
	tool, runs := splitYTDLP(t)
	library := t.TempDir()
	folder := filepath.Join(library, "Me at the zoo [jNQXAC9IVRw]")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{"video.mp4": "track 395", "video.f251.webm": "track 251"} {
		if err := os.WriteFile(filepath.Join(folder, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tools := Tools{YTDLP: tool, OutputDir: library, DownloadTemp: t.TempDir()}
	path, err := tools.Download(context.Background(), "https://www.youtube.com/watch?v=jNQXAC9IVRw")
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); path != filepath.Join(folder, "video.mp4") || string(data) != "track 395track 251" {
		t.Fatalf("%s = %q", path, data)
	}
	if _, err := os.Stat(filepath.Join(folder, "video.f251.webm")); !os.IsNotExist(err) {
		t.Fatalf("leftover track: %v", err)
	}
	if data, _ := os.ReadFile(runs); strings.Count(string(data), "run") != 2 {
		t.Fatalf("yt-dlp ran %q", data)
	}
}

// A video downloaded before, with nothing unfinished beside it, is used as it is.
func TestDownloadReusesAFinishedVideo(t *testing.T) {
	tool, runs := splitYTDLP(t)
	library := t.TempDir()
	folder := filepath.Join(library, "Me at the zoo [jNQXAC9IVRw]")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	video := filepath.Join(folder, "video.mp4")
	if err := os.WriteFile(video, []byte("finished"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "video.bilingual.srt"), []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	tools := Tools{YTDLP: tool, OutputDir: library, DownloadTemp: t.TempDir()}
	path, err := tools.Download(context.Background(), "https://www.youtube.com/watch?v=jNQXAC9IVRw")
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); path != video || string(data) != "finished" {
		t.Fatalf("%s = %q", path, data)
	}
	if data, _ := os.ReadFile(runs); strings.Count(string(data), "run") != 1 {
		t.Fatalf("yt-dlp ran %q", data)
	}
}

// TestLookToolFindsPackageManagerPrograms checks that a program in a package manager's folder that is not on PATH is found, as one winget or Scoop installed on Windows after Kando started, whose folder Kando's PATH does not have yet: there by its .exe.
func TestLookToolFindsPackageManagerPrograms(t *testing.T) {
	dir := t.TempDir()
	file := "ffmpeg"
	if runtime.GOOS == "windows" {
		file = "ffmpeg.exe"
	}
	if err := os.WriteFile(filepath.Join(dir, file), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	saved, savedOwn := extraToolDirs, OwnToolsDir
	extraToolDirs, OwnToolsDir = []string{filepath.Join(dir, "missing"), dir}, ""
	t.Cleanup(func() { extraToolDirs, OwnToolsDir = saved, savedOwn })
	if got := lookTool("ffmpeg"); got != filepath.Join(dir, file) {
		t.Fatalf("lookTool = %q, want %q", got, filepath.Join(dir, file))
	}
	if got := lookTool("whisper-cli"); got != "" {
		t.Fatalf("lookTool found a whisper-cli that is not there: %q", got)
	}
}

// TestPackageManagerDirsOnWindows checks the folders where winget, Scoop and Chocolatey put the programs they install, where the user's environment says they are, and where they are by default.
func TestPackageManagerDirsOnWindows(t *testing.T) {
	env := map[string]string{
		"LOCALAPPDATA": filepath.Join("C:", "Users", "me", "AppData", "Local"),
		"ProgramFiles": filepath.Join("C:", "Program Files"),
		"USERPROFILE":  filepath.Join("C:", "Users", "me"),
		"ProgramData":  filepath.Join("C:", "ProgramData"),
	}
	want := []string{
		filepath.Join(env["LOCALAPPDATA"], "Microsoft", "WinGet", "Links"),
		filepath.Join(env["ProgramFiles"], "WinGet", "Links"),
		filepath.Join(env["USERPROFILE"], "scoop", "shims"),
		filepath.Join(env["ProgramData"], "scoop", "shims"),
		filepath.Join(env["ProgramData"], "chocolatey", "bin"),
	}
	if got := packageManagerDirs("windows", func(key string) string { return env[key] }); !slices.Equal(got, want) {
		t.Errorf("by default: %q, want %q", got, want)
	}
	env["SCOOP"], env["SCOOP_GLOBAL"], env["ChocolateyInstall"] = filepath.Join("D:", "scoop"), filepath.Join("D:", "scoop-global"), filepath.Join("D:", "choco")
	want[2], want[3], want[4] = filepath.Join("D:", "scoop", "shims"), filepath.Join("D:", "scoop-global", "shims"), filepath.Join("D:", "choco", "bin")
	if got := packageManagerDirs("windows", func(key string) string { return env[key] }); !slices.Equal(got, want) {
		t.Errorf("moved: %q, want %q", got, want)
	}
	if got := packageManagerDirs("windows", func(string) string { return "" }); len(got) != 0 {
		t.Errorf("with no environment: %q", got)
	}
	if got := packageManagerDirs("linux", func(key string) string { return env[key] }); !slices.Contains(got, "/home/linuxbrew/.linuxbrew/bin") {
		t.Errorf("on Linux: %q", got)
	}
}

// TestMoviesDirIsTheSystemsVideosFolder checks that a new library goes in the videos folder the system names (VideosDir), as one Windows moved or a Linux desktop names in the user's language, and otherwise in the home folder's.
func TestMoviesDirIsTheSystemsVideosFolder(t *testing.T) {
	saved := VideosDir
	t.Cleanup(func() { VideosDir = saved })
	VideosDir = ""
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	want := filepath.Join(home, "Videos")
	if runtime.GOOS == "darwin" {
		want = filepath.Join(home, "Movies")
	}
	if got := MoviesDir(); got != want {
		t.Fatalf("MoviesDir = %q, want %q", got, want)
	}
	VideosDir = filepath.Join(t.TempDir(), "Vidéos")
	if got := MoviesDir(); got != VideosDir {
		t.Fatalf("MoviesDir = %q, want %q", got, VideosDir)
	}
	if got := DefaultLibraryDir(); filepath.Dir(got) != VideosDir {
		t.Fatalf("DefaultLibraryDir = %q, want it in %q", got, VideosDir)
	}
}

func TestWhisperArgsCarryThreadsAndHints(t *testing.T) {
	tools := Tools{WhisperModel: "m.bin", WhisperThreads: 4, WhisperHints: " Kando,\nwhisper.cpp\n\n、 Monterey "}
	got := strings.Join(tools.WhisperArgs("auto", "out", "audio.wav"), " ")
	if want := "-m m.bin -t 4 -l auto --prompt Kando, whisper.cpp, Monterey -np -pp -oj -of out -f audio.wav"; got != want {
		t.Fatalf("args = %q, want %q", got, want)
	}
	tools.WhisperThreads, tools.WhisperHints = 0, " \n "
	got = strings.Join(tools.WhisperArgs("en", "out", "audio.wav"), " ")
	if want := ("-m m.bin -t " + strconv.Itoa(whisperThreadCount()) + " -l en -np -pp -oj -of out -f audio.wav"); got != want {
		t.Fatalf("args = %q, want %q", got, want)
	}
}
