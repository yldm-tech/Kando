package main

import (
	"fmt"
	"path/filepath"

	ailogo "github.com/yldm-tech/ai-logo/mygo"
)

// The page draws brand marks from AI Logo (github.com/yldm-tech/ai-logo), the collection Kando's own apps share: the websites of the works-with strip, the AI services of the AI card and Claude Code in the MCP tab. Each mark is written into the built site as an SVG file, so the page still loads nothing from elsewhere.

// siteMarks maps a featured site's domain to its mark's ID in AI Logo. The map is explicit rather than guessed from the domain, as a domain's name can be another brand's mark there: qq.com is Tencent Video in the catalog, while AI Logo's qq is the messenger. A featured site without a mark here keeps the logo the app's catalog has for it.
var siteMarks = map[string]string{
	"youtube.com":     "youtube",
	"bilibili.com":    "bilibili",
	"tiktok.com":      "tiktok",
	"twitter.com":     "x",
	"instagram.com":   "instagram",
	"facebook.com":    "facebook",
	"vimeo.com":       "vimeo",
	"twitch.tv":       "twitch",
	"nicovideo.jp":    "niconico",
	"weibo.com":       "weibo",
	"xiaohongshu.com": "xiaohongshu",
	"reddit.com":      "reddit",
	"dailymotion.com": "dailymotion",
	"soundcloud.com":  "soundcloud",
	"bandcamp.com":    "bandcamp",
	"kick.com":        "kick",
	"rumble.com":      "rumble",
	"vk.com":          "vk",
	"naver.com":       "naver",
}

// brand is a mark the page shows with a name.
type brand struct {
	Name string
	// ID is the mark's ID in AI Logo.
	ID string
	// Src is where the built site has the mark.
	Src string
}

// aiServices are the AI services of the AI card, among the presets of the app's AI Service settings, in the order the card names them.
var aiServices = []brand{
	{Name: "EveryAPI", ID: "everyapi"},
	{Name: "OpenAI", ID: "openai"},
	{Name: "DeepSeek", ID: "deepseek"},
	{Name: "Qwen", ID: "qwen"},
	{Name: "Kimi", ID: "kimi"},
	{Name: "Ollama", ID: "ollama"},
	{Name: "LM Studio", ID: "lmstudio"},
}

// lightOnDark are the marks AI Logo draws in color for a dark background, white where the others are dark, which the page's white tiles would hide: they are shown in one color.
var lightOnDark = map[string]bool{"kimi": true}

// claudeCode is the MCP tab's mark: the example adds Kando to Claude Code.
var claudeCode = brand{Name: "Claude Code", ID: "claudecode"}

// marks writes AI Logo's marks into the built site, each once.
type marks struct {
	dir     string
	written map[string]string
}

func newMarks(out string) *marks {
	return &marks{dir: filepath.Join(out, "marks"), written: map[string]string{}}
}

// src writes the mark id into the site, in color where AI Logo draws it in color for a light background and in one color otherwise, and returns the path the page links it by.
func (m *marks) src(id string) (string, error) {
	if src, ok := m.written[id]; ok {
		return src, nil
	}
	variants := []ailogo.Variant{ailogo.Color, ailogo.Mono}
	if lightOnDark[id] {
		variants = variants[1:]
	}
	for _, variant := range variants {
		svg, err := ailogo.Read(id, variant)
		if err != nil {
			continue
		}
		name, err := ailogo.Filename(id, variant)
		if err != nil {
			return "", err
		}
		if err := writeFile(filepath.Join(m.dir, filepath.Base(name)), svg); err != nil {
			return "", err
		}
		src := "/marks/" + filepath.Base(name)
		m.written[id] = src
		return src, nil
	}
	return "", fmt.Errorf("AI Logo has no mark %q", id)
}

// brands are bs with their marks written into the site.
func (m *marks) brands(bs ...brand) ([]brand, error) {
	out := make([]brand, len(bs))
	for i, b := range bs {
		src, err := m.src(b.ID)
		if err != nil {
			return nil, err
		}
		b.Src = src
		out[i] = b
	}
	return out, nil
}
