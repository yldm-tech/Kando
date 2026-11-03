package settings

import (
	"github.com/egoist/mygo/ui"
	ailogo "github.com/yldm-tech/ai-logo/mygo"
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

// providerMark is the color mark when the collection has one, otherwise the mono mark. Nil means this preset has no brand.
func providerMark(id string) *ui.SVG {
	logo, ok := providerLogos[id]
	if !ok {
		return nil
	}
	if icon, err := ailogo.Parse(logo, ailogo.Color); err == nil {
		return icon
	}
	icon, err := ailogo.Parse(logo, ailogo.Mono)
	if err != nil {
		return nil
	}
	return icon
}
