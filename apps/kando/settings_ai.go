package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/locales"
	"kando/apps/kando/settings"
	"kando/packages/media"
)

// Settings › AI 服务: the translation model's provider, endpoint and key.

// savedLLM is the AI service's part of settings.json.
type savedLLM struct {
	LLMProvider string `json:"llmProvider,omitempty"`
	LLMBaseURL  string `json:"llmBaseURL,omitempty"`
	LLMModel    string `json:"llmModel,omitempty"`
	LLMAPIKey   string `json:"llmAPIKey,omitempty"`
}

func (s *savedLLM) clean() {
	s.LLMBaseURL = strings.TrimSpace(s.LLMBaseURL)
	s.LLMModel = strings.TrimSpace(s.LLMModel)
	s.LLMAPIKey = strings.TrimSpace(s.LLMAPIKey)
	s.LLMProvider = strings.TrimSpace(s.LLMProvider)
}

func (a *app) saveLLM(saved *savedSettings) {
	saved.LLMProvider = a.prefs.llmProvider
	if tools, ok := a.pipeline.(media.Tools); ok {
		saved.LLMBaseURL = tools.Translator.BaseURL
		saved.LLMModel = tools.Translator.Model
		saved.LLMAPIKey = tools.Translator.APIKey
	}
}

// restoreLLM uses the AI service saved last time for what the environment does not name. The provider is read from the endpoint, not from the saved llmProvider.
func (a *app) restoreLLM(saved savedSettings) {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	if os.Getenv("KANDO_LLM_BASE_URL") == "" && saved.LLMBaseURL != "" {
		tools.Translator.BaseURL = saved.LLMBaseURL
	}
	if os.Getenv("KANDO_LLM_MODEL") == "" && saved.LLMModel != "" {
		tools.Translator.Model = saved.LLMModel
	}
	// A key goes only to the endpoint it belongs to: KANDO_LLM_API_KEY to whichever endpoint is in use, EVERYAPI_API_KEY to EveryAPI only, the saved key to the host it was saved with. ToolsFromEnv may have put EVERYAPI_API_KEY in for its EveryAPI default, which the saved endpoint can replace.
	if os.Getenv("KANDO_LLM_API_KEY") == "" {
		everyAPIKey := os.Getenv("EVERYAPI_API_KEY")
		switch base := tools.Translator.BaseURL; {
		case base == media.EveryAPIBaseURL && everyAPIKey != "":
			tools.Translator.APIKey = everyAPIKey
		case sameLLMHost(base, saved.LLMBaseURL):
			tools.Translator.APIKey = saved.LLMAPIKey
		default:
			tools.Translator.APIKey = ""
		}
	}
	credential := tools.Translator.Credential
	if tools.Translator.BaseURL != media.EveryAPIBaseURL || tools.Translator.APIKey != "" {
		tools.Translator.Credential = nil
	}
	a.syncLLM(tools)
	a.prefs.llmCredential = credential
}

// llmPreset is a service the AI 服务 page offers. Name and Blurb are in the interface's language, read when llmPresets is called; Group is one of the settings package's Group values.
type llmPreset struct {
	ID, Name, Blurb, BaseURL, Model, Group string
}

func llmPresets() []llmPreset {
	return []llmPreset{
		{ID: "everyapi", Name: "EveryAPI", Blurb: locales.SettingsAIBlurbEveryAPI(), BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel, Group: settings.GroupCloud},
		{ID: "openai", Name: "OpenAI", Blurb: locales.SettingsAIBlurbOpenAI(), BaseURL: "https://api.openai.com/v1", Group: settings.GroupCloud},
		{ID: "openrouter", Name: "OpenRouter", Blurb: locales.SettingsAIBlurbOpenRouter(), BaseURL: "https://openrouter.ai/api/v1", Group: settings.GroupCloud},
		{ID: "groq", Name: "Groq", Blurb: locales.SettingsAIBlurbGroq(), BaseURL: "https://api.groq.com/openai/v1", Group: settings.GroupCloud},
		{ID: "deepseek", Name: "DeepSeek", Blurb: locales.SettingsAIBlurbDeepSeek(), BaseURL: "https://api.deepseek.com/v1", Group: settings.GroupCloud},
		{ID: "xai", Name: "xAI", Blurb: locales.SettingsAIBlurbXAI(), BaseURL: "https://api.x.ai/v1", Group: settings.GroupCloud},
		{ID: "mistral", Name: "Mistral", Blurb: locales.SettingsAIBlurbMistral(), BaseURL: "https://api.mistral.ai/v1", Group: settings.GroupCloud},
		{ID: "dashscope", Name: locales.SettingsAINameDashScope(), Blurb: locales.SettingsAIBlurbDashScope(), BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", Group: settings.GroupCloud},
		{ID: "moonshot", Name: "Kimi", Blurb: locales.SettingsAIBlurbMoonshot(), BaseURL: "https://api.moonshot.cn/v1", Group: settings.GroupCloud},
		{ID: "zhipu", Name: locales.SettingsAINameZhipu(), Blurb: locales.SettingsAIBlurbZhipu(), BaseURL: "https://open.bigmodel.cn/api/paas/v4", Group: settings.GroupCloud},
		{ID: "siliconflow", Name: locales.SettingsAINameSiliconFlow(), Blurb: locales.SettingsAIBlurbSiliconFlow(), BaseURL: "https://api.siliconflow.cn/v1", Group: settings.GroupCloud},
		{ID: "ollama", Name: "Ollama", Blurb: locales.SettingsAIBlurbOllama(), BaseURL: "http://127.0.0.1:11434/v1", Group: settings.GroupLocal},
		{ID: "lmstudio", Name: "LM Studio", Blurb: locales.SettingsAIBlurbLMStudio(), BaseURL: "http://127.0.0.1:1234/v1", Group: settings.GroupLocal},
		{ID: "custom", Name: locales.SettingsAINameCustom(), Blurb: locales.SettingsAIBlurbCustom(), Group: settings.GroupOther},
	}
}

func llmPresetByID(id string) (llmPreset, bool) {
	for _, preset := range llmPresets() {
		if preset.ID == id {
			return preset, true
		}
	}
	return llmPreset{}, false
}

func llmProviderID(base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	for _, preset := range llmPresets() {
		if preset.ID == "custom" || preset.BaseURL == "" {
			continue
		}
		if strings.TrimRight(preset.BaseURL, "/") == base {
			return preset.ID
		}
	}
	return "custom"
}

// sameLLMHost reports whether two endpoints are on the same scheme and host, so that a key for one may go to the other.
func sameLLMHost(a, b string) bool {
	return llmHost(a) == llmHost(b)
}

func llmHost(base string) string {
	parsed, err := url.Parse(strings.TrimSpace(base))
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host)
}

func (a *app) syncLLM(tools media.Tools) {
	a.pipeline = tools
	a.prefs.llmBase = tools.Translator.BaseURL
	a.prefs.llmModel = tools.Translator.Model
	a.prefs.llmKey = tools.Translator.APIKey
	a.prefs.llmCredential = tools.Translator.Credential
	a.prefs.llmProvider = llmProviderID(tools.Translator.BaseURL)
}

func (a *app) selectLLM(id string) {
	if a.prefs.llmProvider == id {
		return
	}
	preset, ok := llmPresetByID(id)
	if !ok {
		return
	}
	a.prefs.llmProvider = id
	if preset.BaseURL != "" {
		a.prefs.llmBase = preset.BaseURL
	}
	if preset.Model != "" {
		a.prefs.llmModel = preset.Model
	}
	a.applyLLM()
}

func (a *app) applyLLM() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	base := strings.TrimSpace(a.prefs.llmBase)
	key := strings.TrimSpace(a.prefs.llmKey)
	// The key in the field belongs to the endpoint it was in use with. When the service or Base URL moves to another host and the key was not retyped, it is dropped rather than sent there.
	if key != "" && key == tools.Translator.APIKey && !sameLLMHost(base, tools.Translator.BaseURL) {
		key = ""
		a.prefs.llmKey = ""
	}
	// Without a key EveryAPI uses EVERYAPI_API_KEY, as at startup (media.ToolsFromEnv), and otherwise the signed-in everyapi CLI.
	if base == media.EveryAPIBaseURL && key == "" {
		key = os.Getenv("EVERYAPI_API_KEY")
		a.prefs.llmKey = key
	}
	if base != tools.Translator.BaseURL || key != tools.Translator.APIKey {
		a.prefs.llmEdited = time.Now()
	}
	tools.Translator.BaseURL = base
	tools.Translator.Model = strings.TrimSpace(a.prefs.llmModel)
	tools.Translator.APIKey = key
	tools.Translator.Credential = nil
	if base == media.EveryAPIBaseURL && key == "" {
		tools.Translator.Credential = a.prefs.llmCredential
	}
	a.pipeline = tools
	a.areasLLMChanged()
	if err := a.saveSettings(); err != nil {
		a.saveErr = locales.SettingsAISaveFailed(err.Error())
	}
}

func (a *app) testLLM() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok || a.prefs.testingLLM {
		return
	}
	a.prefs.testingLLM = true
	a.prefs.llmTestOK, a.prefs.llmTestErr = false, ""
	translator := tools.Translator
	credential := translator.Credential != nil
	update := a.update
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := translator.Ping(ctx)
		update(func() {
			a.prefs.testingLLM = false
			a.prefs.llmTestOK = err == nil
			if err != nil {
				a.prefs.llmTestErr = err.Error()
			}
			a.areasLLMTested(err, credential)
		})
	}()
}

// llmTypingPause is how long the Base URL and the key must rest before the models are fetched for them, so typing an address asks the service once, not at every key.
const llmTypingPause = 700 * time.Millisecond

// llmModelsKey names the service the model list was fetched for: the Base URL, the key, and whether the EveryAPI CLI's login stands in for the key.
func llmModelsKey(t media.ChatTranslator) string {
	login := ""
	if t.Credential != nil {
		login = "login"
	}
	return strings.TrimSpace(t.BaseURL) + "\n" + t.APIKey + "\n" + login
}

// loadLLMModels fetches the AI service's model list while Settings › AI 服务 shows, once per service, Base URL and key, and again on retry. It waits for typing in the Base URL or the key to pause.
func (a *app) loadLLMModels() {
	if !a.prefs.open || a.SettingsSection() != settings.SectionAI {
		return
	}
	a.fetchLLMModels(false)
}

func (a *app) fetchLLMModels(force bool) {
	tools, ok := a.pipeline.(media.Tools)
	if !ok || a.prefs.listModels == nil || strings.TrimSpace(tools.Translator.BaseURL) == "" {
		return
	}
	key := llmModelsKey(tools.Translator)
	if !force && key == a.prefs.llmModelsFor {
		return
	}
	if a.prefs.llmModelsLoading && key == a.prefs.llmModelsFor {
		return
	}
	if wait := llmTypingPause - time.Since(a.prefs.llmEdited); wait > 0 {
		if !a.prefs.llmWaiting {
			a.prefs.llmWaiting = true
			update := a.update
			time.AfterFunc(wait, func() {
				update(func() {
					a.prefs.llmWaiting = false
					a.loadLLMModels()
				})
			})
		}
		return
	}
	a.prefs.llmModelsFor = key
	a.prefs.llmModelsLoading = true
	a.prefs.llmModels = nil
	a.prefs.llmModelsErr = nil
	translator, list, update := tools.Translator, a.prefs.listModels, a.update
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		models, err := list(ctx, translator)
		update(func() {
			if a.prefs.llmModelsFor != key {
				return
			}
			a.prefs.llmModelsLoading = false
			if err != nil {
				a.prefs.llmModels = nil
				a.prefs.llmModelsErr = err
				return
			}
			a.prefs.llmModels = models
			a.adoptLLMModel()
		})
	}()
}

// adoptLLMModel picks a model when none is chosen yet: the service's own default when it offers it, else the first it lists. A model already chosen is kept even when the list lacks it; the drop-down shows it on top.
func (a *app) adoptLLMModel() {
	if strings.TrimSpace(a.prefs.llmModel) != "" || len(a.prefs.llmModels) == 0 {
		return
	}
	model := a.prefs.llmModels[0]
	if preset, ok := llmPresetByID(a.prefs.llmProvider); ok && slices.Contains(a.prefs.llmModels, preset.Model) {
		model = preset.Model
	}
	a.prefs.llmModel = model
	a.applyLLM()
}

// llmModelsError says in plain words, in the interface's language, why the model list could not be fetched.
func llmModelsError(err error) string {
	var status interface{ StatusCode() int }
	if errors.As(err, &status) {
		switch status.StatusCode() {
		case http.StatusUnauthorized, http.StatusForbidden:
			return locales.SettingsAIModelsUnauthorized()
		case http.StatusNotFound:
			return locales.SettingsAIModelsNotFound()
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return locales.SettingsAIModelsTimeout()
	}
	return locales.SettingsAIModelsFailed(err.Error())
}
