package settings

import (
	"strings"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/locales"
	"kando/apps/kando/plugins"
	kcore "kando/packages/core"
)

// PluginDetail is what pack adds, in the words of m, as the 插件 page and the install confirmation say it (PluginRow.Detail, PluginInstall.Detail): a glossary's number of terms and its languages, an AI service's host, which the user's key goes to, a dictionary's languages, a word list's language and number of levels, a site pack's number of sites, whose downloads borrow no sign-in.
func PluginDetail(m locales.Messages, pack plugins.Pack) string {
	switch {
	case pack.Glossary != nil:
		return m.SettingsPluginsGlossaryDetail(len(pack.Glossary.Terms), pluginLanguageName(m, pack.Glossary.From), pluginLanguageName(m, pack.Glossary.To))
	case pack.Service != nil:
		return m.SettingsPluginsServiceDetail(Host(pack.Service.BaseURL))
	case pack.Dictionary != nil:
		return m.SettingsPluginsDictionaryDetail(pluginLanguageNames(m, pack.Dictionary.From), pluginLanguageNames(m, pack.Dictionary.To))
	case pack.Wordlist != nil:
		return m.SettingsPluginsWordlistDetail(pluginLanguageName(m, pack.Wordlist.Language), len(pack.Wordlist.Tiers))
	case pack.Site != nil:
		return m.SettingsPluginsSiteDetail(len(pack.Site.Sites))
	}
	return ""
}

// pluginLanguageName names a pack's BCP 47 language tag in the words of m, as the settings name the languages Kando translates into and hears: 简体中文 for zh-Hans, 日语 for ja. A tag Kando does not name is shown as the settings show an unknown spoken language.
func pluginLanguageName(m locales.Messages, tag string) string {
	switch strings.ToLower(tag) {
	case "zh-hans":
		return subtitle.TargetLanguageNameIn(m, "简体中文")
	case "zh-hant":
		return subtitle.TargetLanguageNameIn(m, "繁体中文")
	}
	return subtitle.SpokenLanguageNameIn(m, kcore.BaseLanguage(tag))
}

// pluginLanguageNames names each of tags as pluginLanguageName does, joined with commas.
func pluginLanguageNames(m locales.Messages, tags []string) string {
	names := make([]string, len(tags))
	for i, tag := range tags {
		names[i] = pluginLanguageName(m, tag)
	}
	return strings.Join(names, ", ")
}
