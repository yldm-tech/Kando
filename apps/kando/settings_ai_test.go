package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"kando/packages/media"
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
