package settings

import (
	"sync"

	"github.com/egoist/mygo/ui"
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

// providerMarks holds each preset's parsed mark, so the page parses it once and not on every frame.
var providerMarks sync.Map // preset id -> *ui.SVG, nil for none

// providerMark is the color mark when the collection has one, otherwise the mono mark. Nil means this preset has no brand. The marks come from AI Logo's NSGo module, and MyGo parses them while this page still draws with MyGo.
func providerMark(id string) *ui.SVG {
	if mark, ok := providerMarks.Load(id); ok {
		return mark.(*ui.SVG)
	}
	mark := parseProviderMark(id)
	providerMarks.Store(id, mark)
	return mark
}

func parseProviderMark(id string) *ui.SVG {
	logo, ok := providerLogos[id]
	if !ok {
		return nil
	}
	for _, variant := range []ailogo.Variant{ailogo.Color, ailogo.Mono} {
		data, err := ailogo.Read(logo, variant)
		if err != nil {
			continue
		}
		if icon, err := ui.ParseSVG(data); err == nil {
			return icon
		}
	}
	return nil
}
