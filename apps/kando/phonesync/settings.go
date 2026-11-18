package phonesync

import "strings"

// targetTags are the languages the desktop translates into, as its settings keep them (subtitle.TargetLanguages), by the BCP 47 tags protocol v2's settings carry (psync.Settings.Target).
var targetTags = []struct{ value, tag string }{
	{"简体中文", "zh-Hans"}, {"繁体中文", "zh-Hant"}, {"English", "en"}, {"日本語", "ja"}, {"한국어", "ko"},
	{"Español", "es"}, {"Français", "fr"}, {"Deutsch", "de"}, {"Português", "pt"}, {"Italiano", "it"},
	{"Русский", "ru"}, {"Nederlands", "nl"}, {"Polski", "pl"}, {"Українська", "uk"}, {"Türkçe", "tr"},
	{"العربية", "ar"}, {"हिन्दी", "hi"}, {"Bahasa Indonesia", "id"}, {"Tiếng Việt", "vi"}, {"ไทย", "th"},
}

// TargetTag is the tag of value, the language the settings translate into, "" for one no tag names, as one typed into an older version.
func TargetTag(value string) string {
	value = strings.TrimSpace(value)
	for _, t := range targetTags {
		if t.value == value {
			return t.tag
		}
	}
	return ""
}

// TargetValue is the language tag names as the settings keep it, false for one the desktop does not translate into. Tags are compared without regard to case.
func TargetValue(tag string) (string, bool) {
	for _, t := range targetTags {
		if strings.EqualFold(t.tag, strings.TrimSpace(tag)) {
			return t.value, true
		}
	}
	return "", false
}
