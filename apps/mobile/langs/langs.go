// Package langs is the languages the phone asks about: those a user watches films in, by whisper's codes, and those the computer translates subtitles into, by BCP 47 tags, each in its own name, as a language list names them for anyone to find their own. The names are names, not words of the interface.
package langs

import "strings"

// Lang is a language and its name in itself.
type Lang struct {
	Code, Name string
}

// Spoken are the languages the introduction offers to pick among those the user watches.
var Spoken = []Lang{
	{"en", "English"}, {"ja", "日本語"}, {"ko", "한국어"}, {"fr", "Français"}, {"es", "Español"}, {"de", "Deutsch"},
}

// Targets are the languages the subtitles can be translated into: the desktop app's interface languages.
var Targets = []Lang{
	{"zh-Hans", "简体中文"}, {"zh-Hant", "繁體中文"}, {"en", "English"}, {"ja", "日本語"}, {"ko", "한국어"},
}

// TargetName is the name of target tag, "" for one not among Targets.
func TargetName(tag string) string {
	for _, l := range Targets {
		if l.Code == tag {
			return l.Name
		}
	}
	return ""
}

// DefaultTarget is the target for a phone in locale (a BCP 47 tag, as "zh-TW" or "ja-JP"): the phone's own language where it is a target, else English.
func DefaultTarget(locale string) string {
	parts := strings.Split(strings.ReplaceAll(locale, "_", "-"), "-")
	switch strings.ToLower(parts[0]) {
	case "zh":
		for _, p := range parts[1:] {
			switch strings.ToLower(p) {
			case "hant", "tw", "hk", "mo":
				return "zh-Hant"
			}
		}
		return "zh-Hans"
	case "ja":
		return "ja"
	case "ko":
		return "ko"
	}
	return "en"
}
