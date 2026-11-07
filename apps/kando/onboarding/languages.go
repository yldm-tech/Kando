package onboarding

import (
	"slices"
	"strings"

	"kando/packages/core"
)

// tileCodes are the languages the first step offers as tiles, in the prototype's order. The one subtitles are translated into is left out, and six are shown.
var tileCodes = []string{"en", "ja", "ko", "fr", "es", "de", "zh"}

// names are each language's own name and its name in Chinese, as the tiles show them.
var names = map[string][2]string{
	"en": {"English", "英语"},
	"ja": {"日本語", "日语"},
	"ko": {"한국어", "韩语"},
	"fr": {"Français", "法语"},
	"es": {"Español", "西班牙语"},
	"de": {"Deutsch", "德语"},
	"zh": {"中文", "普通话"},
}

// targetCodes maps the settings' target languages (settings › 翻译 › 默认目标语言) to the language they are written in.
var targetCodes = map[string]string{
	"简体中文": "zh", "繁体中文": "zh", "繁體中文": "zh", "English": "en", "日本語": "ja", "한국어": "ko", "Español": "es", "Français": "fr", "Deutsch": "de",
}

// DefaultLearning is what the first step has chosen before the user changes it, as the prototype starts: English and Japanese.
var DefaultLearning = []string{"en", "ja"}

// Known reports whether code is one of the languages the first step offers.
func Known(code string) bool { return slices.Contains(tileCodes, code) }

// LanguageCode is the language the target language target is written in, "zh" for one Kando does not know.
func LanguageCode(target string) string {
	if code, ok := targetCodes[target]; ok {
		return code
	}
	return "zh"
}

// Effective is the languages chosen in learning that the first step shows as chosen: those it offers, other than the one subtitles are translated into, in the tiles' order.
func Effective(learning []string, target string) []string {
	own := LanguageCode(target)
	var out []string
	for _, code := range tileCodes {
		if code != own && slices.Contains(learning, code) {
			out = append(out, code)
		}
	}
	return out
}

// tiles is the codes of the tiles the first step shows for target.
func tiles(target string) []string {
	own := LanguageCode(target)
	var out []string
	for _, code := range tileCodes {
		if code != own {
			out = append(out, code)
		}
	}
	return out[:min(6, len(out))]
}

// demoLines are the art panel's example line in each language.
var demoLines = map[string]string{
	"en": "Every river begins as a trickle high in the mountains.",
	"ja": "どの川も、山の上の小さな流れから始まる。",
	"ko": "모든 강은 높은 산의 작은 물줄기에서 시작된다.",
	"fr": "Chaque rivière naît d’un mince filet d’eau en montagne.",
	"es": "Todo río nace como un hilo de agua en la montaña.",
	"de": "Jeder Fluss beginnt als Rinnsal hoch in den Bergen.",
	"zh": "每条河都始于高山上的一股细流。",
}

// demoTranslations is the example line in the target languages whose wording differs from demoLines.
var demoTranslations = map[string]string{
	"简体中文": "每条河都始于高山上的一股细流。",
	"繁体中文": "每條河都始於高山上的一股細流。",
	"繁體中文": "每條河都始於高山上的一股細流。",
}

// demo is the art panel's example: the language of the original line, the line, and its translation into target.
func demo(learning []string, target string) (code, original, translation string) {
	chosen := Effective(learning, target)
	code = "en"
	if len(chosen) > 0 {
		code = chosen[0]
	} else if LanguageCode(target) == "en" {
		code = "ja"
	}
	translation, ok := demoTranslations[target]
	if !ok {
		translation = demoLines[LanguageCode(target)]
	}
	return code, demoLines[code], translation
}

// LinkHint is what the line under the third step's field says about raw: nothing for an empty field, what the link points at when ParseLink reads it, or why it cannot be added. ok is false for a link that cannot be added: one that does not read as a link to a video on the web, and a channel or a list that names no single video, because a job downloads one video (yt-dlp runs with --no-playlist).
func LinkHint(raw string) (hint string, ok bool) {
	if strings.TrimSpace(raw) == "" {
		return "", true
	}
	link, parsed := core.ParseLink(raw)
	if !parsed || link.Platform == core.PlatformLocal {
		return "这不像一个视频链接", false
	}
	site := link.SiteName
	if site == "" {
		site = "这个网站"
	}
	switch {
	case link.Kind == core.LinkChannel:
		return "这是" + site + "频道的链接。现在一次加入一个视频，请粘贴其中一个视频的链接", false
	case link.Kind == core.LinkPlaylist && link.ID == "":
		return "这是" + site + "列表的链接。现在一次加入一个视频，请粘贴其中一个视频的链接", false
	case link.Kind == core.LinkPlaylist:
		return "识别为 " + site + " 视频，只加入这一个，不加入整个列表", true
	}
	if link.SiteName == "" {
		return "识别为网页视频", true
	}
	return "识别为 " + site + " 视频", true
}
