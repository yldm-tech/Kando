package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"kando/apps/kando/locales"
	"kando/apps/kando/settings"
	"kando/packages/media"

	"github.com/egoist/mygo/ui"
)

// llmEnv sets the AI service's environment for one test: HOME and the library go to temporary directories, so ToolsFromEnv never reads the user's own settings.
func llmEnv(t *testing.T, baseURL, apiKey, everyAPIKey string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("KANDO_OUTPUT_DIR", t.TempDir())
	t.Setenv("KANDO_LLM_BASE_URL", baseURL)
	t.Setenv("KANDO_LLM_API_KEY", apiKey)
	t.Setenv("KANDO_LLM_MODEL", "")
	t.Setenv("EVERYAPI_API_KEY", everyAPIKey)
}

// bearerServer is an OpenAI-compatible provider that records the bearer tokens it receives.
func bearerServer(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var got []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, r.Header.Get("Authorization"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"pong"}}]}`))
	}))
	t.Cleanup(server.Close)
	return server, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), got...)
	}
}

func translator(t *testing.T, a *app) media.ChatTranslator {
	t.Helper()
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		t.Fatalf("pipeline %#v", a.pipeline)
	}
	return tools.Translator
}

// TestRestoreLLMSendsTheEveryAPIKeyOnlyToEveryAPI checks that EVERYAPI_API_KEY stays with EveryAPI when settings.json names another provider: the provider gets its own saved key, or none, never the EveryAPI one.
func TestRestoreLLMSendsTheEveryAPIKeyOnlyToEveryAPI(t *testing.T) {
	llmEnv(t, "", "", "ea-env-secret")
	server, received := bearerServer(t)

	for _, tc := range []struct {
		name, savedKey, want string
	}{
		{name: "saved key", savedKey: "sk-custom", want: "Bearer sk-custom"},
		{name: "no saved key", want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newApp(media.ToolsFromEnv(), func(func()) {})
			a.restoreLLM(savedSettings{savedLLM: savedLLM{LLMBaseURL: server.URL, LLMModel: "gpt-test", LLMAPIKey: tc.savedKey}})
			got := translator(t, a)
			if got.BaseURL != server.URL || got.APIKey != tc.savedKey || got.Credential != nil {
				t.Fatalf("translator base %q key %q credential %v, want base %q key %q", got.BaseURL, got.APIKey, got.Credential != nil, server.URL, tc.savedKey)
			}
			if a.prefs.llmKey != tc.savedKey {
				t.Fatalf("the settings show key %q, want %q", a.prefs.llmKey, tc.savedKey)
			}
			before := len(received())
			if err := got.Ping(context.Background()); err != nil {
				t.Fatal(err)
			}
			if auth := received()[before]; auth != tc.want {
				t.Fatalf("the custom provider received Authorization %q, want %q", auth, tc.want)
			}
		})
	}
}

// TestRestoreLLMKeys checks which key restoreLLM uses: KANDO_LLM_API_KEY for whatever endpoint is in use, EVERYAPI_API_KEY for EveryAPI only, and the saved key only for the endpoint it was saved with.
func TestRestoreLLMKeys(t *testing.T) {
	const openAI = "https://api.openai.com/v1"
	for _, tc := range []struct {
		name                      string
		envBase, envKey, everyKey string
		saved                     savedLLM
		wantBase, wantKey         string
	}{
		{name: "EveryAPI key with saved EveryAPI", everyKey: "ea-env", saved: savedLLM{LLMBaseURL: media.EveryAPIBaseURL, LLMAPIKey: "ea-saved"}, wantBase: media.EveryAPIBaseURL, wantKey: "ea-env"},
		{name: "saved EveryAPI key without the environment", saved: savedLLM{LLMBaseURL: media.EveryAPIBaseURL, LLMAPIKey: "ea-saved"}, wantBase: media.EveryAPIBaseURL, wantKey: "ea-saved"},
		{name: "EveryAPI key with saved custom provider", everyKey: "ea-env", saved: savedLLM{LLMBaseURL: openAI, LLMAPIKey: "sk-openai"}, wantBase: openAI, wantKey: "sk-openai"},
		{name: "EveryAPI key with saved keyless provider", everyKey: "ea-env", saved: savedLLM{LLMBaseURL: "http://127.0.0.1:11434/v1"}, wantBase: "http://127.0.0.1:11434/v1", wantKey: ""},
		{name: "KANDO_LLM_API_KEY wins", envKey: "sk-env", everyKey: "ea-env", saved: savedLLM{LLMBaseURL: openAI, LLMAPIKey: "sk-openai"}, wantBase: openAI, wantKey: "sk-env"},
		{name: "saved key of another endpoint than KANDO_LLM_BASE_URL", envBase: "https://api.groq.com/openai/v1", saved: savedLLM{LLMBaseURL: openAI, LLMAPIKey: "sk-openai"}, wantBase: "https://api.groq.com/openai/v1", wantKey: ""},
		{name: "saved key of KANDO_LLM_BASE_URL", envBase: openAI, everyKey: "ea-env", saved: savedLLM{LLMBaseURL: openAI, LLMAPIKey: "sk-openai"}, wantBase: openAI, wantKey: "sk-openai"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			llmEnv(t, tc.envBase, tc.envKey, tc.everyKey)
			a := newApp(media.ToolsFromEnv(), func(func()) {})
			a.restoreLLM(savedSettings{savedLLM: tc.saved})
			got := translator(t, a)
			if got.BaseURL != tc.wantBase || got.APIKey != tc.wantKey {
				t.Fatalf("base %q key %q, want base %q key %q", got.BaseURL, got.APIKey, tc.wantBase, tc.wantKey)
			}
		})
	}
}

// TestSwitchingTheLLMEndpointDropsTheKey checks that the key in use does not follow the settings to another service, whether chosen from the list or typed into Base URL, and that EveryAPI gets EVERYAPI_API_KEY back.
func TestSwitchingTheLLMEndpointDropsTheKey(t *testing.T) {
	llmEnv(t, "", "", "ea-env-secret")
	a := newApp(media.ToolsFromEnv(), func(func()) {})
	a.restoreLLM(savedSettings{})
	if got := translator(t, a); got.BaseURL != media.EveryAPIBaseURL || got.APIKey != "ea-env-secret" {
		t.Fatalf("start: base %q key %q", got.BaseURL, got.APIKey)
	}

	a.selectLLM("openai")
	if got := translator(t, a); got.BaseURL != "https://api.openai.com/v1" || got.APIKey != "" || a.prefs.llmKey != "" {
		t.Fatalf("after choosing OpenAI: base %q key %q field %q", got.BaseURL, got.APIKey, a.prefs.llmKey)
	}

	a.prefs.llmKey = "sk-openai"
	a.applyLLM()
	if got := translator(t, a); got.APIKey != "sk-openai" {
		t.Fatalf("typed key %q", got.APIKey)
	}
	a.prefs.llmBase = "https://api.openai.com/v1/"
	a.applyLLM()
	if got := translator(t, a); got.APIKey != "sk-openai" {
		t.Fatalf("editing the path on the same host dropped the key: %q", got.APIKey)
	}

	a.selectLLM("groq")
	if got := translator(t, a); got.APIKey != "" || a.prefs.llmKey != "" {
		t.Fatalf("the OpenAI key followed to Groq: key %q field %q", got.APIKey, a.prefs.llmKey)
	}

	a.selectLLM("everyapi")
	if got := translator(t, a); got.BaseURL != media.EveryAPIBaseURL || got.APIKey != "ea-env-secret" {
		t.Fatalf("back on EveryAPI: base %q key %q", got.BaseURL, got.APIKey)
	}

	a.selectLLM("custom")
	a.prefs.llmBase = "https://proxy.example.com/v1"
	a.applyLLM()
	if got := translator(t, a); got.BaseURL != "https://proxy.example.com/v1" || got.APIKey != "" {
		t.Fatalf("typing a custom Base URL kept the EveryAPI key: base %q key %q", got.BaseURL, got.APIKey)
	}
}

// TestEditingTheBaseURLDropsASavedEveryAPIKey checks the same for an EveryAPI key that came from settings.json rather than the environment.
func TestEditingTheBaseURLDropsASavedEveryAPIKey(t *testing.T) {
	llmEnv(t, "", "", "")
	a := newApp(media.Tools{Translator: media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel}}, func(func()) {})
	a.restoreLLM(savedSettings{savedLLM: savedLLM{LLMBaseURL: media.EveryAPIBaseURL, LLMAPIKey: "ea-saved"}})
	if got := translator(t, a); got.APIKey != "ea-saved" {
		t.Fatalf("start key %q", got.APIKey)
	}
	a.prefs.llmBase = "https://proxy.example.com/v1"
	a.applyLLM()
	if got := translator(t, a); got.APIKey != "" || a.prefs.llmKey != "" {
		t.Fatalf("the saved EveryAPI key followed to another host: key %q field %q", got.APIKey, a.prefs.llmKey)
	}
}

// everyAPIApp is an app whose tools are all in a bin folder that is the whole PATH, with a speech model, and whose AI service is EveryAPI without a key on a computer that had no everyapi program when Kando started. It returns the folder, where a test installs a fake everyapi (writeFakeEveryAPI) to see Kando find it. The tools are shell scripts that only tell their version, which is all 重新检测 asks of them.
func everyAPIApp(t *testing.T) (a *app, tt *ui.Tester, updates chan func(), bin string) {
	t.Helper()
	llmEnv(t, "", "", "")
	saved := media.OwnToolsDir
	media.OwnToolsDir = ""
	t.Cleanup(func() { media.OwnToolsDir = saved })
	bin = filepath.Join(t.TempDir(), "bin")
	t.Setenv("PATH", bin)
	model := filepath.Join(t.TempDir(), "ggml-base.bin")
	if err := os.WriteFile(model, []byte("model"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := func(name string) string { return writeFakeScript(t, bin, name, "echo "+name+" version 1") }
	tools := media.Tools{
		YTDLP:        tool("yt-dlp"),
		FFmpeg:       tool("ffmpeg"),
		Whisper:      tool("whisper-cli"),
		WhisperModel: model,
		Translator:   media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel},
	}
	a, tt, updates = newTestApp(tools.UseEveryAPICLI())
	a.restoreLLM(savedSettings{})
	a.checkReadiness(true)
	tt.Frame()
	return a, tt, updates, bin
}

// TestEveryAPICLIInstalledLaterIsFoundByDetectAgain checks that without the everyapi program the sidebar and Settings › AI 服务 say it is missing rather than signed out, with 获取 EveryAPI, and that once it is installed 重新检测 on that page finds it and signs in through it without Kando starting again.
func TestEveryAPICLIInstalledLaterIsFoundByDetectAgain(t *testing.T) {
	a, tt, updates, bin := everyAPIApp(t)
	if !tt.HasText(everyAPIMissing()) {
		t.Fatalf("sidebar: missing %q in %q", everyAPIMissing(), tt.Texts())
	}
	page := settingsOpenAt(a, tt, settings.SectionAI)
	for _, text := range []string{locales.SettingsAIEveryAPICLI(), locales.SettingsAIEveryAPIMissing(), locales.SettingsAIGetEveryAPI(), locales.SettingsAIRedetect()} {
		if !page.HasText(text) {
			t.Fatalf("AI page: missing %q in %q", text, page.Texts())
		}
	}

	writeFakeEveryAPI(t, bin, true)
	if err := page.Click(locales.SettingsAIRedetect()); err != nil {
		t.Fatal(err)
	}
	page.Frame()
	if got := translator(t, a); got.Credential == nil {
		t.Fatal("重新检测 did not sign in through the everyapi program it found")
	}
	if !page.HasText(checkingSignIn()) || page.HasText(locales.SettingsAIGetEveryAPI()) {
		t.Fatalf("AI page while the sign-in is asked: %q", page.Texts())
	}
	waitUntil(t, updates, func() bool { return a.side.ready.signInDone })
	tt.Frame()
	page.Frame()
	if !tt.HasText(readyMessage()) {
		t.Fatalf("sidebar: missing %q in %q", readyMessage(), tt.Texts())
	}
	if page.HasText(locales.SettingsAIRedetect()) {
		t.Fatalf("AI page still has the everyapi row once signed in: %q", page.Texts())
	}
	endpoint, err := translator(t, a).Credential(context.Background(), false)
	if err != nil || endpoint.APIKey != fakeEveryAPIKey {
		t.Fatalf("credential = %#v, %v", endpoint, err)
	}

	// A key typed and then cleared again leaves the sign-in to the program found, not to the one Kando started with.
	a.prefs.llmKey = "ea-typed"
	a.applyLLM()
	if translator(t, a).Credential != nil {
		t.Fatal("a typed key still signs in through everyapi")
	}
	a.prefs.llmKey = ""
	a.applyLLM()
	if translator(t, a).Credential == nil {
		t.Fatal("clearing the key went back to the everyapi Kando started without")
	}
}

// TestEveryAPICLISignedOutSaysHowToSignIn checks that an everyapi nobody signed in to is reported as signed out, with the command to run, in the sidebar and on Settings › AI 服务, where 获取 EveryAPI is not offered.
func TestEveryAPICLISignedOutSaysHowToSignIn(t *testing.T) {
	a, tt, updates, bin := everyAPIApp(t)
	writeFakeEveryAPI(t, bin, false)
	a.redetectTools()
	waitUntil(t, updates, func() bool { return a.side.ready.signInDone })
	tt.Frame()
	if !errors.Is(a.side.ready.signInErr, media.ErrEveryAPISignedOut) {
		t.Fatalf("sign-in error %v, want %v", a.side.ready.signInErr, media.ErrEveryAPISignedOut)
	}
	if !tt.HasText(everyAPISignedOut()) {
		t.Fatalf("sidebar: missing %q in %q", everyAPISignedOut(), tt.Texts())
	}
	page := settingsOpenAt(a, tt, settings.SectionAI)
	if !page.HasText(locales.SettingsAIEveryAPISignedOut()) || !page.HasText(locales.SettingsAIRedetect()) || page.HasText(locales.SettingsAIGetEveryAPI()) {
		t.Fatalf("AI page: %q", page.Texts())
	}
	if msg, section := a.precheck([]string{"https://youtu.be/TRWgBxVAb1M"}); msg != "" || section != "" {
		t.Fatalf("precheck = %q, %q; a sign-in only running the program can tell is left to the job", msg, section)
	}
}

// TestGetEveryAPIOpensItsWebsite checks that 获取 EveryAPI opens everyapi.ai in the browser, and that the page says why when it could not.
func TestGetEveryAPIOpensItsWebsite(t *testing.T) {
	var opened []string
	fail := error(nil)
	saved := everyAPIOpenURL
	everyAPIOpenURL = func(link string) error {
		opened = append(opened, link)
		return fail
	}
	t.Cleanup(func() { everyAPIOpenURL = saved })
	a, tt, updates, _ := everyAPIApp(t)
	page := settingsOpenAt(a, tt, settings.SectionAI)
	if err := page.Click(locales.SettingsAIGetEveryAPI()); err != nil {
		t.Fatal(err)
	}
	waitUpdate(t, updates)
	page.Frame()
	if !slices.Equal(opened, []string{"https://everyapi.ai"}) || a.prefs.everyAPIOpenErr != "" {
		t.Fatalf("opened %q, error %q", opened, a.prefs.everyAPIOpenErr)
	}

	fail = errors.New("no browser")
	if err := page.Click(locales.SettingsAIGetEveryAPI()); err != nil {
		t.Fatal(err)
	}
	waitUpdate(t, updates)
	page.Frame()
	if want := locales.SettingsAIEveryAPIOpenFailed("no browser"); !page.HasText(want) {
		t.Fatalf("missing %q in %q", want, page.Texts())
	}
}
