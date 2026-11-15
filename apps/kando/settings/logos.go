package settings

import (
	"github.com/nsgo-dev/nsgo/core"
	ailogo "github.com/yldm-tech/ai-logo/nsgo"
)

// providerLogos maps a translation preset to an AI Logo id. A preset is absent when it is not a brand.
var providerLogos = map[string]string{
	"everyapi":    "everyapi",
	"openai":      "openai",
	"openrouter":  "openrouter",
	"groq":        "groq",
	"deepseek":    "deepseek",
	"xai":         "xai",
	"mistral":     "mistral",
	"dashscope":   "qwen",
	"moonshot":    "kimi",
	"zhipu":       "zhipu",
	"siliconflow": "siliconcloud",
	"ollama":      "ollama",
	"lmstudio":    "lmstudio",
}

// providerMark is the color mark when the collection has one, otherwise the mono mark. Nil means this preset has no brand. AI Logo parses each mark once and keeps it, so the page asks for it whenever the service changes.
func providerMark(id string) *core.SVG {
	logo, ok := providerLogos[id]
	if !ok {
		return nil
	}
	for _, variant := range []ailogo.Variant{ailogo.Color, ailogo.Mono} {
		if icon, err := ailogo.Parse(logo, variant); err == nil {
			return icon
		}
	}
	return nil
}
