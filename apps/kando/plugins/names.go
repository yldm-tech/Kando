package plugins

import (
	"strings"
	"unicode"

	"golang.org/x/text/width"
)

// nameMax is the most characters a pack's name, or a label it supplies, may have; textMax a description's or an attribution's.
const (
	nameMax = 40
	textMax = 500
)

// ReservedNames are the names no pack may take, compared as reserved does (case, full and half width and spaces aside): Kando's own, in every language its interface has, and the name of every built-in AI service in every language (settings.Presets), so that a pack cannot pass itself off as Kando or as a service the user already trusts. The list is kept here rather than read from the settings because this package imports no interface package; a test of the app's (plugins_test.go) checks that every name settings.Presets gives, in every language, is in it.
var ReservedNames = []string{
	"Kando", "看懂", "Official", "官方",
	"EveryAPI", "OpenAI", "OpenRouter", "Groq", "DeepSeek", "xAI", "Mistral", "Kimi", "Ollama", "LM Studio",
	"Qwen", "通义千问", "通義千問",
	"Zhipu GLM", "智谱 GLM", "智譜 GLM",
	"SiliconFlow", "硅基流动", "矽基流動",
	"OpenAI Compatible", "OpenAI 兼容", "OpenAI 相容", "OpenAI 互換", "OpenAI 호환",
}

// reserved reports whether name reads as one of ReservedNames.
func reserved(name string) bool {
	folded := foldName(name)
	for _, r := range ReservedNames {
		if folded == foldName(r) {
			return true
		}
	}
	return false
}

// foldName is name without the differences a reader does not see: letter case, full and half width, and spaces.
func foldName(name string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, width.Fold.String(name))
}

// cleanText is s with what a reader cannot see or that would rearrange the text around it taken out: bidirectional controls, zero-width characters and every other control or format character, the line break aside when lines is set; and the spaces around it trimmed. A pack's texts are shown as plain text, so this is what keeps one from looking like another.
func cleanText(s string, lines bool) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		switch {
		case r == '\n' && lines:
			return r
		case r == '\t':
			return ' '
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r), unicode.Is(unicode.Bidi_Control, r):
			return -1
		}
		return r
	}, s))
}
