package settings

import (
	"net/url"
	"strings"

	"kando/apps/kando/locales"
	"kando/packages/media"
)

// Preset is one translation service the AI page can select: its id, its name and what it is in the language of the messages Presets is given, its group, one of the Group values, and the endpoint and default model it fills in. Custom has neither.
type Preset struct {
	ID, Name, Blurb, Group string
	BaseURL, Model         string
}

// The groups of the presets: services in the cloud, services running on this Mac, which the drop-down names after their group, and any other endpoint.
const (
	GroupCloud = "cloud"
	GroupLocal = "local"
	GroupOther = "other"
)

// Presets are the services the AI 服务 page offers, their names and blurbs in the language of m.
func Presets(m locales.Messages) []Preset {
	return []Preset{
		{ID: "everyapi", Name: "EveryAPI", Blurb: m.SettingsAIBlurbEveryAPI(), BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel, Group: GroupCloud},
		{ID: "openai", Name: "OpenAI", Blurb: m.SettingsAIBlurbOpenAI(), BaseURL: "https://api.openai.com/v1", Group: GroupCloud},
		{ID: "openrouter", Name: "OpenRouter", Blurb: m.SettingsAIBlurbOpenRouter(), BaseURL: "https://openrouter.ai/api/v1", Group: GroupCloud},
		{ID: "groq", Name: "Groq", Blurb: m.SettingsAIBlurbGroq(), BaseURL: "https://api.groq.com/openai/v1", Group: GroupCloud},
		{ID: "deepseek", Name: "DeepSeek", Blurb: m.SettingsAIBlurbDeepSeek(), BaseURL: "https://api.deepseek.com/v1", Group: GroupCloud},
		{ID: "xai", Name: "xAI", Blurb: m.SettingsAIBlurbXAI(), BaseURL: "https://api.x.ai/v1", Group: GroupCloud},
		{ID: "mistral", Name: "Mistral", Blurb: m.SettingsAIBlurbMistral(), BaseURL: "https://api.mistral.ai/v1", Group: GroupCloud},
		{ID: "dashscope", Name: m.SettingsAINameDashScope(), Blurb: m.SettingsAIBlurbDashScope(), BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", Group: GroupCloud},
		{ID: "moonshot", Name: "Kimi", Blurb: m.SettingsAIBlurbMoonshot(), BaseURL: "https://api.moonshot.cn/v1", Group: GroupCloud},
		{ID: "zhipu", Name: m.SettingsAINameZhipu(), Blurb: m.SettingsAIBlurbZhipu(), BaseURL: "https://open.bigmodel.cn/api/paas/v4", Group: GroupCloud},
		{ID: "siliconflow", Name: m.SettingsAINameSiliconFlow(), Blurb: m.SettingsAIBlurbSiliconFlow(), BaseURL: "https://api.siliconflow.cn/v1", Group: GroupCloud},
		{ID: "ollama", Name: "Ollama", Blurb: m.SettingsAIBlurbOllama(), BaseURL: "http://127.0.0.1:11434/v1", Group: GroupLocal},
		{ID: "lmstudio", Name: "LM Studio", Blurb: m.SettingsAIBlurbLMStudio(), BaseURL: "http://127.0.0.1:1234/v1", Group: GroupLocal},
		{ID: "custom", Name: m.SettingsAINameCustom(), Blurb: m.SettingsAIBlurbCustom(), Group: GroupOther},
	}
}

// PresetByID is the preset id, its name and blurb in the language of m.
func PresetByID(m locales.Messages, id string) (Preset, bool) {
	for _, preset := range Presets(m) {
		if preset.ID == id {
			return preset, true
		}
	}
	return Preset{}, false
}

// ProviderID is the preset whose endpoint base is, "custom" for an endpoint none of them has. Only the presets' endpoints are read, so their names are left in the catalogs' source language.
func ProviderID(base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	for _, preset := range Presets(locales.In(locales.Source)) {
		if preset.ID == "custom" || preset.BaseURL == "" {
			continue
		}
		if strings.TrimRight(preset.BaseURL, "/") == base {
			return preset.ID
		}
	}
	return "custom"
}

// SameHost reports whether two endpoints are on the same scheme and host, so that a key for one may go to the other.
func SameHost(a, b string) bool {
	return host(a) == host(b)
}

func host(base string) string {
	parsed, err := url.Parse(strings.TrimSpace(base))
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host)
}
