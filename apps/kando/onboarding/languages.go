package onboarding

import (
	"slices"
	"strings"

	"kando/apps/kando/films/sitename"
	"kando/apps/kando/locales"
	"kando/packages/core"

	ns "github.com/nsgo-dev/nsgo/core"
)

// tileCodes are the languages the first step offers as tiles, in the prototype's order. The one subtitles are translated into is left out, and six are shown.
var tileCodes = []string{"en", "ja", "ko", "fr", "es", "de", "zh"}

// autonyms are each language's own name, as the tiles show it over languageName.
var autonyms = map[string]string{
	"en": "English",
	"ja": "日本語",
	"ko": "한국어",
	"fr": "Français",
	"es": "Español",
	"de": "Deutsch",
	"zh": "中文",
}

// languageName is the name of the language code in the interface's language under cx, as the tiles show it under its own name.
func languageName(cx *ns.Scope, code string) string {
	switch code {
	case "en":
		return locales.OnboardingLanguageEn(cx)
	case "ja":
		return locales.OnboardingLanguageJa(cx)
	case "ko":
		return locales.OnboardingLanguageKo(cx)
	case "fr":
		return locales.OnboardingLanguageFr(cx)
	case "es":
		return locales.OnboardingLanguageEs(cx)
	case "de":
		return locales.OnboardingLanguageDe(cx)
	case "zh":
		return locales.OnboardingLanguageZh(cx)
	}
	return code
}

// DefaultLearning is what the first step has chosen before the user changes it, as the prototype starts: English and Japanese.
var DefaultLearning = []string{"en", "ja"}

// Known reports whether code is one of the languages the first step offers.
func Known(code string) bool { return slices.Contains(tileCodes, code) }

// LanguageCode is the language the target language target is written in, "" for one Kando does not know.
func LanguageCode(target string) string { return core.BaseLanguage(core.TargetTag(target)) }

// targetTag is the BCP 47 tag of the target language the settings keep, as zh-Hans for 简体中文, "" for one Kando does not know.
func targetTag(target string) string { return core.TargetTag(target) }

// Effective is the languages chosen in learning that the first step shows as chosen: those whose tiles it shows for target, in the tiles' order.
func Effective(learning []string, target string) []string {
	var out []string
	for _, code := range tiles(target) {
		if slices.Contains(learning, code) {
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

// demoLines are the art panel's example line in each language: those of the tiles, and those of the target languages, as the line's translation.
var demoLines = map[string]string{
	"en": "Every river begins as a trickle high in the mountains.",
	"ja": "どの川も、山の上の小さな流れから始まる。",
	"ko": "모든 강은 높은 산의 작은 물줄기에서 시작된다.",
	"fr": "Chaque rivière naît d’un mince filet d’eau en montagne.",
	"es": "Todo río nace como un hilo de agua en la montaña.",
	"de": "Jeder Fluss beginnt als Rinnsal hoch in den Bergen.",
	"zh": "每条河都始于高山上的一股细流。",
	"pt": "Todo rio nasce como um fio de água no alto das montanhas.",
	"it": "Ogni fiume nasce come un rivolo d’acqua in alta montagna.",
	"ru": "Каждая река начинается с ручейка высоко в горах.",
	"nl": "Elke rivier begint als een klein stroompje hoog in de bergen.",
	"pl": "Każda rzeka zaczyna się jako mały strumyk wysoko w górach.",
	"uk": "Кожна річка починається з маленького струмка високо в горах.",
	"tr": "Her nehir, dağların yükseklerinde küçük bir dere olarak başlar.",
	"ar": "كل نهر يبدأ جدولًا صغيرًا في أعالي الجبال.",
	"hi": "हर नदी ऊँचे पहाड़ों में एक छोटी धारा के रूप में शुरू होती है।",
	"id": "Setiap sungai berawal dari aliran kecil di pegunungan tinggi.",
	"vi": "Mọi dòng sông đều bắt đầu từ một dòng suối nhỏ trên núi cao.",
	"th": "แม่น้ำทุกสายเริ่มต้นจากลำธารเล็ก ๆ บนภูเขาสูง",
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

// LinkHint is what the line under the third step's field says about raw, in the interface's language under cx: nothing for an empty field, what the link points at when ParseLink reads it, or why it cannot be added. ok is false for a link that cannot be added: one that does not read as a link to a video on the web. A channel or a list that names no single video can be: the guide's button opens the dialog that lists its videos, as the library's link field does.
func LinkHint(cx *ns.Scope, raw string) (hint string, ok bool) { return linkHint(locales.For(cx), raw) }

// Addable is LinkHint's ok alone, for the app's handler of the third step's button, which needs no words.
func Addable(raw string) bool {
	_, ok := linkHint(locales.In(locales.Source), raw)
	return ok
}

// linkHint is LinkHint in the language of m.
func linkHint(m locales.Messages, raw string) (hint string, ok bool) {
	if strings.TrimSpace(raw) == "" {
		return "", true
	}
	link, parsed := core.ParseLink(raw)
	// A site's home page, its search results, a feed or a hashtag list videos without being one.
	if !parsed || link.Platform == core.PlatformLocal || link.Kind == core.LinkPage {
		return m.OnboardingLinkNotVideo(), false
	}
	site := sitename.In(m, link.Site)
	switch {
	case link.Kind == core.LinkChannel && site == "":
		return m.OnboardingLinkChannelUnnamed(), true
	case link.Kind == core.LinkChannel:
		return m.OnboardingLinkChannel(site), true
	case !link.OneVideo() && site == "":
		return m.OnboardingLinkListUnnamed(), true
	case !link.OneVideo():
		return m.OnboardingLinkList(site), true
	case link.Kind == core.LinkPlaylist && site == "":
		return m.OnboardingLinkPlaylistVideoUnnamed(), true
	case link.Kind == core.LinkPlaylist:
		return m.OnboardingLinkPlaylistVideo(site), true
	}
	if site == "" {
		return m.OnboardingLinkWebVideo(), true
	}
	return m.OnboardingLinkVideo(site), true
}
