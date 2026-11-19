package settings

import (
	"strings"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	kcore "kando/packages/core"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// MinContextLines is the fewest earlier lines the 翻译 page lets go with a batch while context is on; turning context off sends none.
const MinContextLines = 10

// translate is the 翻译 page: what translates the subtitles, with sources, the 翻译来源 row the window is given (Sources), at the top of its first card when it is, the language subtitles are translated into, whether each batch takes the sentences before it, the final proofread and which of its changes apply by themselves, and the prompt, with styles, the 风格预设 row the window is given (Styles), over it when it is. The prototype's page starts 6 points lower than one that opens with a section label.
type translate struct {
	s       screen
	styles  core.Node
	sources core.Node
}

// Render implements core.Node.
func (p translate) Render(cx *core.Scope) core.Node {
	a := p.s.a
	// choices is the target languages offered, joined, so the drop-down is built again only when a language typed into an older version leaves the list.
	choices := core.NewMemo(cx, live.Func(a, func() string { return strings.Join(LanguageChoices(*a.Language()), "\n") }))
	target := core.Dynamic(func(cx *core.Scope) core.Node {
		// TODO(nsgo#145): give ui.Select LabelFunc twins for its name and its options' labels so they follow the interface language without building the drop-down again, then drop this Dynamic https://github.com/nsgo-dev/nsgo/issues/145
		values := strings.Split(choices.Get(), "\n")
		options := make([]ui.Option[string], len(values))
		for i, value := range values {
			options[i] = ui.Option[string]{Value: value, Label: LanguageName(cx, value)}
		}
		return ui.Select[string]{Options: options, Label: locales.SettingsTranslateTarget(cx), TestID: "settings-translate-language",
			Searchable: true, SearchPlaceholder: locales.SettingsLanguagesSearch(cx),
			Value: live.Func(a, func() string { return *a.Language() }),
			OnChange: live.On(a, func(value string) {
				*a.Language() = value
				a.Save()
			}),
			Style: core.Style{Width: core.Pt(160)}}
	})
	context := form.SwitchRow{Title: locales.SettingsTranslateContext(cx), Subtitle: locales.SettingsTranslateContextHint(cx), TestID: "settings-translate-context",
		On: live.Func(a, func() bool { return *a.TranslateContext() }),
		OnChange: live.On(a, func(on bool) {
			*a.TranslateContext() = on
			a.Save()
		})}
	lines := ui.NumberInput{TestID: "settings-translate-context-lines", Label: locales.SettingsTranslateContextLines(cx), Min: MinContextLines, Max: media.MaxContextLines, Step: 10,
		Disabled: live.Func(a, func() bool { return !*a.TranslateContext() }),
		Value:    live.Func(a, func() float64 { return float64(a.TranslateContextLines()) }),
		OnChange: live.On(a, func(v float64) {
			a.SetTranslateContextLines(int(v))
			a.Save()
		}),
		Style: core.Style{Width: core.Pt(96)}}
	suggest := form.SwitchRow{Title: locales.SettingsSuggestTerms(cx), Subtitle: locales.SettingsSuggestTermsHint(cx), TestID: "settings-suggest-terms",
		On: live.Func(a, func() bool { return *a.SuggestTerms() }),
		OnChange: live.On(a, func(on bool) {
			*a.SuggestTerms() = on
			a.Save()
		})}
	card := form.Card(p.sourcesRow(), form.Row{Title: locales.SettingsTranslateTarget(cx), NoGroup: true, Trailing: []core.Node{target}}, form.Separator(true), context,
		form.Separator(true), form.Row{Title: locales.SettingsTranslateContextLines(cx), Subtitle: locales.SettingsTranslateContextLinesHint(cx), Trailing: []core.Node{lines}},
		form.Separator(true), suggest)
	card.Style.Margin = core.Edges{Top: 6}
	review := form.SwitchRow{Title: locales.SettingsReviewOn(cx), Subtitle: locales.SettingsReviewOnHint(cx), TestID: "settings-review",
		On: live.Func(a, func() bool { return *a.Review() }),
		OnChange: live.On(a, func(on bool) {
			*a.Review() = on
			a.Save()
		})}
	autos := []ui.Option[string]{
		{Value: string(kcore.ProofAutoConsistency), Label: locales.SettingsReviewAutoConsistency(cx)},
		{Value: string(kcore.ProofAutoAll), Label: locales.SettingsReviewAutoAll(cx)},
		{Value: string(kcore.ProofAutoNone), Label: locales.SettingsReviewAutoNone(cx)},
	}
	auto := choice{a: a, id: "settings-review-auto", title: locales.SettingsReviewAuto(cx), subtitle: locales.SettingsReviewAutoHint(cx), options: autos,
		get: a.ReviewAuto, set: a.SetReviewAuto}
	usingDefault := live.Func(a, func() bool {
		return strings.TrimSpace(*a.TranslatePrompt()) == strings.TrimSpace(media.DefaultSubtitlePrompt)
	})
	prompt := ui.Input{TestID: "settings-translate-prompt", Multiline: true, Label: locales.SettingsTranslatePromptLabel(cx),
		Value: live.Func(a, func() string { return *a.TranslatePrompt() }),
		OnChange: live.On(a, func(text string) {
			*a.TranslatePrompt() = text
			a.Save()
		}),
		Style: core.Style{Height: core.Pt(106), FontFamily: "monospace"}}
	state := core.Text{Style: core.Style{FontSize: 11, FlexGrow: 1, FlexShrink: 1}, StyleFunc: ui.Colored(ui.ColorsOf(cx).Muted),
		ContentFunc: live.Func(a, func() string {
			switch {
			case usingDefault():
				return locales.SettingsTranslatePromptDefault(cx)
			case !strings.Contains(*a.TranslatePrompt(), "{{language}}"):
				// The translation then names the target language after the prompt (media's subtitlePrompt), which the line says rather than leaving the language to look forgotten.
				return locales.SettingsTranslatePromptNoLanguage(cx)
			}
			return locales.SettingsTranslatePromptCustom(cx)
		})}
	restore := form.Button{Text: locales.SettingsTranslateRestore(cx), TestID: "settings-translate-reset", Disabled: usingDefault, OnPress: live.Do(a, func() {
		if strings.TrimSpace(*a.TranslatePrompt()) != strings.TrimSpace(media.DefaultSubtitlePrompt) {
			*a.TranslatePrompt() = media.DefaultSubtitlePrompt
			a.Save()
		}
	})}
	editor := core.View{Style: core.Style{Padding: core.Edges{Top: 10, Right: 14, Bottom: 12, Left: 14}, Gap: 8, AlignItems: core.AlignStretch}, Children: []core.Node{
		prompt,
		core.View{Style: core.Style{Direction: core.Row, Gap: 8, AlignItems: core.AlignCenter}, Children: []core.Node{state, restore}},
	}}
	return form.Page(
		card,
		form.Note(static(locales.SettingsTranslateTargetNote(cx)), 0),
		form.Label(locales.SettingsReview(cx), 12),
		form.Card(review, form.Separator(true), auto),
		form.Label(locales.SettingsTranslatePrompt(cx), 12),
		p.promptCard(editor),
		form.Note(static(locales.SettingsTranslatePromptNote(cx)), 0),
	)
}

// sourcesRow is the 翻译来源 row with its separator under it, which the window gives the page (Sources), or nothing when it gives none.
func (p translate) sourcesRow() core.Node {
	if p.sources == nil {
		return core.Group{}
	}
	return p.sources
}

// promptCard is the card of the prompt editor, with the 风格预设 row over it when the window was given one.
func (p translate) promptCard(editor core.Node) core.Node {
	if p.styles == nil {
		return form.Card(editor)
	}
	return form.Card(p.styles, editor)
}

// LanguageChoices is the target languages the settings offer, with current added when it is not one of them, as a language typed into an older version is kept. These are the values the settings, the jobs and the translation prompt keep, as 简体中文; LanguageName is what the interface shows for each.
func LanguageChoices(current string) []string {
	choices := subtitle.TargetLanguages()
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

// LanguageName is how the interface names the target language value, one of LanguageChoices, in the language the app shows under cx (subtitle.TargetLanguageName).
func LanguageName(cx *core.Scope, value string) string { return subtitle.TargetLanguageName(cx, value) }
