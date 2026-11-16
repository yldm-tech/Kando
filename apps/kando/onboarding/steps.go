package onboarding

import (
	"slices"
	"strings"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// cardShadow is the soft shadow under a card or a tile.
var cardShadow = []core.BoxShadow{{Offset: core.Point{Y: 1}, Blur: 2, Color: core.RGBA(0x000000, 0.04)}}

// card is a card in the theme's card color with the prototype's hairline and soft shadow, its rows stacked.
func card(children ...core.Node) core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		k := theme.Of(cx)
		return core.View{Style: core.Style{AlignItems: core.AlignStretch, CornerRadius: 12, BorderWidth: 0.5, Shadows: cardShadow},
			StyleFunc: func() core.Style { return core.Style{Background: k.Card(), BorderColor: k.Shade(0.1)} }, Children: children}
	})
}

// rule is the hairline between a card's rows.
func rule() core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		k := theme.Of(cx)
		return core.View{Style: core.Style{Height: core.Pt(0.5), FlexShrink: 0}, StyleFunc: func() core.Style { return core.Style{Background: k.Shade(0.08)} }}
	})
}

// sectionLabel is the 12 point label over a group of the step's body.
func sectionLabel(text func() string) core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		return core.Text{ContentFunc: text, Style: core.Style{FontSize: 12, FontWeight: core.FontWeightSemibold}, StyleFunc: ui.Colored(ui.ColorsOf(cx).Muted)}
	})
}

// note is the 11 point gray line under a group.
func note(text func() string) core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		return core.Text{ContentFunc: text, Style: core.Style{FontSize: 11, LineHeight: 16}, StyleFunc: ui.Colored(theme.Of(cx).Placeholder)}
	})
}

// rowText is a row's name over its second line, which takes the color sub gives with it.
func rowText(name func() string, sub func() (string, core.Color)) core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 1}, Children: []core.Node{
			core.Text{ContentFunc: name, MaxLines: 1, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 13}, StyleFunc: ui.Colored(ui.ColorsOf(cx).Text)},
			core.Text{ContentFunc: func() string { s, _ := sub(); return s }, Style: core.Style{FontSize: 11},
				StyleFunc: func() core.Style { _, c := sub(); return core.Style{Color: c} }},
		}}
	})
}

// rowStyle is the look of a card's row of a program or a model: its name and second line, then what can be done with it.
var rowStyle = core.Style{Direction: core.Row, Padding: core.Symmetric(5, 14), Gap: 10, AlignItems: core.AlignCenter}

// smallButton is a row's 24-point button, its words following text.
func smallButton(cx *core.Scope, id string, text func() string, b ui.Button) core.Node {
	b.TestID, b.Size, b.Variant = id, ui.SizeSmall, ui.ButtonSecondary
	return core.View{Style: core.Style{FlexShrink: 0}, Children: []core.Node{button(cx, text, b)}}
}

// progress is a row's bar of a download, 72 wide, at value from 0 to 1, or moving while it is negative, its size not known yet.
func progress(cx *core.Scope, value func() float64) core.Node {
	// The bar's accessible name is set as it is built, so it is built again in another language; it holds no focus to lose.
	name := core.NewMemo(cx, func() string { return locales.OnboardingDownloadProgress(cx) })
	return core.Dynamic(func(*core.Scope) core.Node {
		return ui.Progress{Label: name.Get(), Style: core.Style{Width: core.Pt(72), FlexShrink: 0},
			Value:         func() float32 { return float32(max(value(), 0) * 100) },
			Indeterminate: func() bool { return value() < 0 }}
	})
}

// languagesStep is the first step: the language subtitles are translated into, and tiles for the languages of the videos the user watches.
type languagesStep struct{ a App }

// Render implements core.Node.
func (s languagesStep) Render(cx *core.Scope) core.Node {
	a := s.a
	target := live.Memo(cx, a, func() string { return *a.Language() })
	codes := core.NewMemoFunc(cx, func() []string { return tiles(target()) }, slices.Equal[[]string])
	chosen := core.NewMemoFunc(cx, live.Func(a, func() []string { return Effective(a.OnboardingLearning(), target()) }), slices.Equal[[]string])
	grid := core.View{Role: core.RoleGroup, LabelFunc: func() string { return locales.OnboardingWatchLanguagesLabel(cx) },
		Style: core.Style{Display: core.DisplayGrid, GridTemplateColumns: "repeat(3, 1fr)", Gap: 8},
		Children: []core.Node{core.Each(codes.Get, func(code string) string { return code }, func(cx *core.Scope, code func() string, _ func() int) core.Node {
			return tile{a: a, code: code(), on: func() bool { return slices.Contains(chosen.Get(), code()) }}
		})}}
	return core.View{Style: core.Style{Gap: 16, AlignItems: core.AlignStretch}, Children: []core.Node{
		card(core.View{Style: core.Style{Direction: core.Row, Height: core.Pt(44), Padding: core.Edges{Left: 14, Right: 8}, Gap: 12, AlignItems: core.AlignCenter},
			Children: []core.Node{
				core.Text{ContentFunc: func() string { return locales.OnboardingTranslateInto(cx) }, Style: core.Style{FontSize: 13, FlexGrow: 1, FlexShrink: 1}},
				languageSelect(cx, a),
			}}),
		core.View{Style: core.Style{Gap: 8, AlignItems: core.AlignStretch}, Children: []core.Node{
			sectionLabel(func() string { return locales.OnboardingWatchLanguages(cx) }),
			grid,
		}},
		note(func() string { return locales.OnboardingLanguagesNote(cx) }),
	}}
}

// selectFace is what the target language's field is built from: its accessible name and the languages it offers.
type selectFace struct {
	label   string
	choices string
}

// languageSelect is the field of the language subtitles are translated into, listing the values the settings keep, as they do.
func languageSelect(cx *core.Scope, a App) core.Node {
	// ui.Select takes its name and options as it is built, so it is built again when the language shown or the choices change, which happens while the settings, not this field, have the keyboard.
	face := live.Memo(cx, a, func() selectFace {
		return selectFace{label: locales.OnboardingTranslateInto(cx), choices: strings.Join(a.LanguageChoices(), "\x00")}
	})
	return core.Dynamic(func(*core.Scope) core.Node {
		f := face()
		var options []ui.Option[string]
		for _, choice := range strings.Split(f.choices, "\x00") {
			options = append(options, ui.Option[string]{Value: choice, Label: choice})
		}
		return ui.Select[string]{TestID: "onboarding-language", Label: f.label, Options: options, Style: core.Style{Width: core.Pt(160), FlexShrink: 0, AlignSelf: core.AlignCenter},
			Value: live.Func(a, func() string { return *a.Language() }),
			OnChange: live.On(a, func(v string) {
				*a.Language() = v
				a.OnboardingSave()
			})}
	})
}

// tile is a language's tile: its own name over its name in the interface's language, and a check circle at its top right, blue and ringed in blue while chosen. It is a checkbox for assistive technology.
type tile struct {
	a    App
	code string
	on   func() bool
}

// Render implements core.Node.
func (t tile) Render(cx *core.Scope) core.Node {
	a, code := t.a, t.code
	colors, k := ui.ColorsOf(cx), theme.Of(cx)
	own := autonyms[code]
	look := func() core.Style {
		if t.on() {
			return core.Style{Background: theme.Accent.Faded(0.07), Shadows: []core.BoxShadow{{Spread: 1.5, Color: theme.Accent, Inset: true}}}
		}
		return core.Style{Background: k.Card(), BorderWidth: 0.5, BorderColor: k.Shade(0.1), Shadows: cardShadow, Hover: &core.Style{Background: k.CardHover()}}
	}
	return core.View{
		TestID: "onboarding-tile-" + code, Role: core.RoleCheckbox, Label: own, Checked: t.on,
		OnPress: live.On(a, func(core.PressEvent) { a.OnboardingToggle(code) }),
		Style: core.Style{Height: core.Pt(52), Padding: core.Symmetric(0, 12), Gap: 1, CornerRadius: 10, JustifyContent: core.JustifyCenter, AlignItems: core.AlignStart,
			FocusVisible: ui.FocusRing(cx, 2)},
		StyleFunc: look,
		Children: []core.Node{
			// TODO(nsgo#207): give core.Text the language it is written in, as MyGo's Text.Lang did, so 日本語 and 中文 are drawn with their own glyph forms https://github.com/nsgo-dev/nsgo/issues/207
			core.Text{Content: own, MaxLines: 1, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 13, FontWeight: core.FontWeightSemibold}, StyleFunc: ui.Colored(colors.Text)},
			core.Text{ContentFunc: func() string { return languageName(cx, code) }, MaxLines: 1, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 11}, StyleFunc: ui.Colored(k.Placeholder)},
			// The empty ring is drawn in the shade, which checkOff takes as its current color.
			core.Icon{SVGFunc: func() *core.SVG {
				if t.on() {
					return checkOn
				}
				return checkOff
			}, Style: core.Style{Position: core.Absolute, Top: core.Pt(9), Right: core.Pt(9), Width: core.Pt(16), Height: core.Pt(16)}, StyleFunc: ui.Colored(func() core.Color { return k.Shade(1) })},
		},
	}
}

// modelsStep is the second step: the programs Kando runs, the speech models recommended for the languages chosen, each with its download, and the translation service, set up in place (aiCard).
type modelsStep struct{ a App }

// Render implements core.Node.
func (s modelsStep) Render(cx *core.Scope) core.Node {
	a := s.a
	colors := ui.ColorsOf(cx)
	models := core.NewMemoFunc(cx, live.Func(a, func() []Model { return a.OnboardingModels(cx) }), slices.Equal[[]Model])
	busy := live.Memo(cx, a, a.OnboardingDownloading)
	ai := live.Memo(cx, a, func() AI { return a.OnboardingAI(cx) })
	modelRows := core.Each(models.Get, func(m Model) string { return m.ID }, func(cx *core.Scope, m func() Model, _ func() int) core.Node {
		return core.Group{rule(), modelRow{a: a, m: m, busy: busy}}
	})
	return core.View{Style: core.Style{Gap: 16, AlignItems: core.AlignStretch}, Children: []core.Node{
		toolsCard{a},
		card(
			core.View{Style: core.Style{Direction: core.Row, Height: core.Pt(38), Padding: core.Symmetric(0, 14), AlignItems: core.AlignCenter}, Children: []core.Node{
				core.Text{ContentFunc: func() string { return modelsHead(cx, models.Get()) }, Style: core.Style{FontSize: 12, FlexGrow: 1, FlexShrink: 1}, StyleFunc: ui.Colored(colors.Muted)},
			}},
			modelRows,
		),
		note(func() string { return modelsNote(cx, models.Get()) }),
		aiCard{a: a, ai: ai},
	}}
}

// toolsCard is the programs Kando runs: a head saying whether they are ready, and a row for each that is not, with its download. Once all are there it is the head alone.
type toolsCard struct{ a App }

// Render implements core.Node.
func (t toolsCard) Render(cx *core.Scope) core.Node {
	a := t.a
	tools := core.NewMemoFunc(cx, live.Func(a, func() []Tool { return a.OnboardingTools(cx) }), slices.Equal[[]Tool])
	ready := func() bool { _, ready, _ := toolsHead(cx, tools.Get()); return ready }
	unavailable := func() bool {
		return slices.ContainsFunc(tools.Get(), func(t Tool) bool { return t.State == ToolUnavailable })
	}
	missing := func() []Tool {
		return slices.DeleteFunc(slices.Clone(tools.Get()), func(t Tool) bool { return t.State == ToolReady })
	}
	return core.View{Style: core.Style{Gap: 16, AlignItems: core.AlignStretch}, Children: []core.Node{
		card(
			core.View{Style: core.Style{Direction: core.Row, Height: core.Pt(38), Padding: core.Symmetric(0, 14), Gap: 10, AlignItems: core.AlignCenter}, Children: []core.Node{
				core.Text{ContentFunc: func() string { head, _, _ := toolsHead(cx, tools.Get()); return head }, Style: core.Style{FontSize: 12, FlexGrow: 1, FlexShrink: 1}, StyleFunc: ui.Colored(ui.ColorsOf(cx).Muted)},
				core.Show{When: ready, Then: func(cx *core.Scope) core.Node {
					return core.View{Role: core.RoleImage, Accessible: true, LabelFunc: func() string { head, _, _ := toolsHead(cx, tools.Get()); return head },
						Children: []core.Node{mark(readyMark)}}
				}},
				// A program Kando cannot download is the user's to install, after which Kando looks for it again.
				core.Show{When: unavailable, Then: func(cx *core.Scope) core.Node {
					return smallButton(cx, "onboarding-tools-redetect", func() string { return locales.SettingsGeneralRedetect(cx) }, ui.Button{OnPress: live.Do(a, a.Redetect)})
				}},
			}},
			core.Each(missing, func(t Tool) string { return t.Name }, func(cx *core.Scope, t func() Tool, _ func() int) core.Node {
				return core.Group{rule(), toolRow{a: a, t: t}}
			}),
		),
		core.Show{When: func() bool { _, _, downloading := toolsHead(cx, tools.Get()); return downloading }, Then: func(cx *core.Scope) core.Node {
			return note(func() string { return locales.OnboardingToolsNote(cx) })
		}},
	}}
}

// toolsHead is the head of the card of programs, whether all of them are there, and whether one is downloading.
func toolsHead(cx *core.Scope, tools []Tool) (head string, ready, downloading bool) {
	ready = true
	for _, t := range tools {
		ready = ready && t.State == ToolReady
		downloading = downloading || t.State == ToolDownloading
	}
	switch {
	case ready:
		return locales.OnboardingToolsReady(cx), true, false
	case downloading:
		return locales.OnboardingToolsDownloading(cx), false, true
	}
	return locales.OnboardingToolsMissing(cx), false, false
}

// toolRow is a program that is not there yet: its name over what it is for, its download's progress or why it failed, and a progress bar, 下载 or 重试.
type toolRow struct {
	a App
	t func() Tool
}

// Render implements core.Node.
func (r toolRow) Render(cx *core.Scope) core.Node {
	a, t := r.a, r.t
	name := t().Name
	state := core.NewMemo(cx, func() ToolState { return t().State })
	control := core.Dynamic(func(cx *core.Scope) core.Node {
		switch state.Get() {
		case ToolDownloading:
			return progress(cx, func() float64 { return t().Progress })
		case ToolMissing:
			return smallButton(cx, "onboarding-tool-"+name, func() string {
				if t().Err != "" {
					return locales.OnboardingRetry(cx)
				}
				return locales.OnboardingDownload(cx)
			}, ui.Button{OnPress: live.Do(a, func() { a.OnboardingDownloadTool(name) })})
		case ToolUnavailable:
			if page := t().Page; page != "" {
				return ui.Link{Text: locales.SettingsGeneralToolPage(cx), Label: locales.SettingsGeneralToolPageLabel(cx, name), Href: page, TestID: "onboarding-tool-page-" + name,
					Style: core.Style{AlignSelf: core.AlignCenter, FlexShrink: 0}}
			}
		}
		return core.Group(nil)
	})
	return core.View{Role: core.RoleGroup, Label: name, Style: rowStyle.Merge(core.Style{MinHeight: core.Pt(42)}), Children: []core.Node{
		rowText(func() string { return t().Name }, func() (string, core.Color) { return toolSub(cx, t()) }),
		control,
	}}
}

// toolSub is the line under a program's name and its color: the download's progress in blue while it runs, why it failed in red, how to install it where Kando cannot, otherwise what it is for.
func toolSub(cx *core.Scope, t Tool) (string, core.Color) {
	k := theme.Of(cx)
	switch {
	case t.State == ToolDownloading && t.Progress < 0:
		return locales.OnboardingToolConnecting(cx), k.AccentInk()
	case t.State == ToolDownloading:
		return locales.OnboardingToolProgress(cx, int(t.Progress*100)), k.AccentInk()
	case t.State == ToolUnavailable && t.Install != "":
		return locales.OnboardingToolUnavailableCommand(cx, t.Install), k.WarnInk()
	case t.State == ToolUnavailable:
		return locales.OnboardingToolUnavailable(cx), k.WarnInk()
	case t.Err != "":
		return locales.OnboardingModelFailed(cx, t.Err), k.DangerInk()
	}
	return t.Use, ui.ColorsOf(cx).Muted()
}

// modelRow is a speech model: its name and size, and what can be done with it: 下载, a progress bar and 取消 while it downloads, 使用 once it is here, and a green check while it is in use.
type modelRow struct {
	a    App
	m    func() Model
	busy func() bool
}

// Render implements core.Node.
func (r modelRow) Render(cx *core.Scope) core.Node {
	a, m := r.a, r.m
	id := m().ID
	state := core.NewMemo(cx, func() ModelState { return m().State })
	control := core.Dynamic(func(cx *core.Scope) core.Node {
		switch state.Get() {
		case ModelInUse:
			return core.View{Role: core.RoleImage, Accessible: true, LabelFunc: func() string { return locales.OnboardingInUse(cx) }, Children: []core.Node{mark(readyMark)}}
		case ModelReady:
			return smallButton(cx, "onboarding-use-"+id, func() string { return locales.OnboardingUse(cx) },
				ui.Button{OnPress: live.Do(a, func() { a.OnboardingUseModel(id) })})
		case ModelDownloading:
			return core.View{Style: core.Style{Direction: core.Row, Gap: 10, AlignItems: core.AlignCenter, FlexShrink: 0}, Children: []core.Node{
				progress(cx, func() float64 { return m().Progress }),
				smallButton(cx, "onboarding-cancel-"+id, func() string { return locales.OnboardingCancel(cx) },
					ui.Button{OnPress: live.Do(a, a.OnboardingCancelDownload)}),
			}}
		}
		download := smallButton(cx, "onboarding-download-"+id, func() string {
			if m().Err != "" {
				return locales.OnboardingRetry(cx)
			}
			return locales.OnboardingDownload(cx)
		}, ui.Button{Disabled: r.busy, OnPress: live.Do(a, func() { a.OnboardingDownload(id) })})
		return ui.Tooltip{TextFunc: func() string { return locales.OnboardingDownloadTooltip(cx, m().Name, m().Size) }, Trigger: download,
			Style: core.Style{FlexShrink: 0}}
	})
	return core.View{Role: core.RoleGroup, LabelFunc: func() string { return m().Name }, Style: rowStyle.Merge(core.Style{MinHeight: core.Pt(46)}), Children: []core.Node{
		rowText(func() string { return locales.OnboardingSpeechModel(cx, m().Name) }, func() (string, core.Color) { return modelSub(cx, m()) }),
		control,
	}}
}

// modelSub is the line under a model's name and its color: the download's progress in blue while it runs, why it failed in red, otherwise its size and what it suits.
func modelSub(cx *core.Scope, m Model) (string, core.Color) {
	k, muted := theme.Of(cx), ui.ColorsOf(cx).Muted()
	switch m.State {
	case ModelInUse:
		return locales.OnboardingModelInUse(cx), muted
	case ModelReady:
		return locales.OnboardingModelDownloaded(cx, m.Blurb), muted
	case ModelDownloading:
		if m.Progress < 0 {
			return locales.OnboardingModelConnecting(cx, m.Size), k.AccentInk()
		}
		return locales.OnboardingModelProgress(cx, int(m.Progress*100), m.Size), k.AccentInk()
	}
	if m.Err != "" {
		return locales.OnboardingModelFailed(cx, m.Err), k.DangerInk()
	}
	return locales.OnboardingModelAbout(cx, m.Size, m.Blurb), muted
}

// modelsHead is the card's head: what is in use, or what is missing.
func modelsHead(cx *core.Scope, models []Model) string {
	for _, m := range models {
		if m.State == ModelInUse {
			return locales.OnboardingModelsReady(cx)
		}
	}
	for _, m := range models {
		if m.State == ModelDownloading {
			return locales.OnboardingModelsDownloading(cx, m.Name)
		}
	}
	return locales.OnboardingModelsMissing(cx)
}

// modelsNote is the line under the models.
func modelsNote(cx *core.Scope, models []Model) string {
	for _, m := range models {
		if m.State == ModelDownloading {
			return locales.OnboardingModelsNoteDownloading(cx)
		}
	}
	for _, m := range models {
		if m.State == ModelInUse {
			return locales.OnboardingModelsNoteInUse(cx)
		}
	}
	return locales.OnboardingModelsNoteLater(cx)
}

// linkStep is the third step: the field for the first link and what it reads as. The field takes the keyboard as the step shows, and Return adds a link it can add, as the primary button does.
type linkStep struct{ a App }

// Render implements core.Node.
func (s linkStep) Render(cx *core.Scope) core.Node {
	a := s.a
	colors, k := ui.ColorsOf(cx), theme.Of(cx)
	link := live.Func(a, func() string { return *a.OnboardingLink() })
	focused := core.NewSignal(cx, false)
	// TODO(nsgo#145): give core.Input (base.Input) a LabelFunc twin, so the field's accessible name follows the interface language as it changes; it is read once here, and a Dynamic would take the focus and the typed text with it https://github.com/nsgo-dev/nsgo/issues/145
	input := base.Input{
		TestID: "onboarding-link", AutoFocus: true,
		Label:           locales.OnboardingLinkLabel(cx),
		Value:           link,
		PlaceholderFunc: func() string { return locales.OnboardingLinkPlaceholder(cx) },
		OnChange:        live.On(a, func(text string) { *a.OnboardingLink() = text }),
		OnSubmit: live.On(a, func(string) {
			if Addable(*a.OnboardingLink()) {
				a.OnboardingNext()
			}
		}),
		OnFocus: func() { focused.Set(true) },
		OnBlur:  func() { focused.Set(false) },
		Style:   core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), FontSize: 14, CaretColor: theme.Accent, SelectionColor: theme.Accent.Faded(0.22)},
		StyleFunc: func() core.Style {
			return core.Style{Color: colors.Text(), PlaceholderColor: k.Placeholder()}
		},
	}
	field := core.View{
		Style: core.Style{Direction: core.Row, Height: core.Pt(44), CornerRadius: 10, Padding: core.Edges{Left: 13, Right: 14}, Gap: 8, AlignItems: core.AlignCenter,
			BorderWidth: 0.5},
		StyleFunc: func() core.Style {
			s := core.Style{Background: k.Card(), BorderColor: k.Shade(0.14), Shadows: []core.BoxShadow{{Offset: core.Point{Y: 1}, Blur: 3, Color: core.RGBA(0x000000, 0.05)}}}
			if focused.Get() {
				s.Shadows = []core.BoxShadow{{Spread: 3, Color: theme.Accent.Faded(0.5)}}
			}
			return s
		},
		Children: []core.Node{
			core.Icon{SVG: linkIcon, Style: core.Style{Width: core.Pt(15), Height: core.Pt(15), FlexShrink: 0}, StyleFunc: ui.Colored(k.Placeholder)},
			input,
		},
	}
	hint := live.Memo(cx, a, func() hintLine {
		hint, ok := LinkHint(cx, *a.OnboardingLink())
		switch {
		case a.OnboardingLinkError() != "" && *a.OnboardingLink() != "":
			return hintLine{a.OnboardingLinkError(), k.DangerInk()}
		case hint == "":
			return hintLine{locales.OnboardingLinkHintEmpty(cx), k.Placeholder()}
		case !ok:
			return hintLine{hint, k.DangerInk()}
		}
		return hintLine{hint, k.SuccessInk()}
	})
	return core.View{Style: core.Style{Gap: 6, AlignItems: core.AlignStretch}, Children: []core.Node{
		field,
		core.Text{TestID: "onboarding-link-hint", ContentFunc: func() string { return hint().text }, Style: core.Style{FontSize: 12, MinHeight: core.Pt(16)},
			StyleFunc: func() core.Style { return core.Style{Color: hint().color} }},
	}}
}

// hintLine is the line under the link field and its color.
type hintLine struct {
	text  string
	color core.Color
}
