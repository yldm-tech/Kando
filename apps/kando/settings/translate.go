package settings

import (
	"slices"
	"strings"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	"kando/packages/media"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// translate is the 翻译 page: the language subtitles are translated into, whether each batch takes the sentences before it, and the prompt. The prototype's page starts 6 points lower than one that opens with a section label.
func (s screen) translate(c *ui.Context, t *ui.Theme) {
	form.Card(c, func() {
		form.Row(c, locales.SettingsTranslateTarget(), "", func() {
			c.Key("translate-language")
			values := LanguageChoices(*s.Language())
			labels := make([]string, len(values))
			for i, value := range values {
				labels[i] = LanguageName(value)
			}
			chosen := LanguageName(*s.Language())
			if form.Select(c, &chosen, labels).Label(locales.SettingsTranslateTarget()).Changed() {
				if i := slices.Index(labels, chosen); i >= 0 {
					*s.Language() = values[i]
				}
				s.Save()
			}
		})
		form.Separator(c, true)
		c.Key("translate-context")
		if form.SwitchRow(c, locales.SettingsTranslateContext(), locales.SettingsTranslateContextHint(), s.TranslateContext()) {
			s.Save()
		}
	}).Margin(6, 0, 0, 0)
	form.Note(c, locales.SettingsTranslateTargetNote(), 0)
	usingDefault := strings.TrimSpace(*s.TranslatePrompt()) == strings.TrimSpace(media.DefaultSubtitlePrompt)
	form.Label(c, locales.SettingsTranslatePrompt(), 12, nil)
	form.Card(c, func() {
		ui.Column(c).Padding(10, 14, 12, 14).Gap(8).AlignItems(ui.Stretch).Children(func() {
			c.Key("translate-prompt")
			if breeze.TextArea(c, s.TranslatePrompt()).Height(106).Font("monospace").FontSize(12).Label(locales.SettingsTranslatePromptLabel()).Changed() {
				s.Save()
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				label := locales.SettingsTranslatePromptCustom()
				if usingDefault {
					label = locales.SettingsTranslatePromptDefault()
				}
				ui.Text(c, label).FontSize(11).TextColor(form.Secondary).Grow(1).Wrap()
				c.Key("translate-reset")
				if form.Button(c, locales.SettingsTranslateRestore(), true).Disabled(usingDefault).Clicked() && !usingDefault {
					*s.TranslatePrompt() = media.DefaultSubtitlePrompt
					s.Save()
				}
			})
		})
	})
	form.Note(c, locales.SettingsTranslatePromptNote(), 0)
}

// LanguageChoices is the target languages the settings offer, with current added when it is not one of them, as a language typed into an older version is kept. These are the values the settings, the jobs and the translation prompt keep, as 简体中文; LanguageName is what the interface shows for each.
func LanguageChoices(current string) []string {
	choices := []string{"简体中文", "繁体中文", "English", "日本語", "한국어", "Español", "Français", "Deutsch"}
	for _, choice := range choices {
		if choice == current {
			return choices
		}
	}
	if strings.TrimSpace(current) == "" {
		return choices
	}
	return append([]string{current}, choices...)
}

// LanguageName is how the interface names the target language value, one of LanguageChoices (subtitle.TargetLanguageName).
func LanguageName(value string) string { return subtitle.TargetLanguageName(value) }
