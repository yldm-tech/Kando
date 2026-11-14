// Package onboarding draws the first-run guide: a sheet of three steps over the window that picks the languages, gets the programs Kando runs and a speech model onto this Mac and takes the first link. It draws and hands the clicks back; the settings it changes, the download and the library it opens are the app's.
package onboarding

import (
	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Steps is how many steps the guide has.
const Steps = 3

// onMac reports whether the app under cx runs on macOS, where the guide calls the machine whisper runs on 这台 Mac; elsewhere it is 这台电脑.
func onMac(cx *core.Scope) bool { return cx.Env().OS() == core.OSMacOS }

// ModelState is where a speech model is.
type ModelState int

const (
	// ModelMissing is a model that is not on this Mac.
	ModelMissing ModelState = iota
	// ModelDownloading is the model being downloaded now.
	ModelDownloading
	// ModelReady is on this Mac but another model is in use.
	ModelReady
	// ModelInUse is the model speech recognition uses.
	ModelInUse
)

// Model is a row of the second step: a whisper model the guide offers.
type Model struct {
	ID   string
	Name string
	// Size is the download's size as the catalog writes it, such as "574 MB".
	Size  string
	Blurb string
	State ModelState
	// Progress is how much of a download has arrived, from 0 to 1, or negative while its size is not known yet.
	Progress float64
	// Err is why the last download of this model failed.
	Err string
}

// ToolState is where a program Kando runs is.
type ToolState int

const (
	// ToolMissing is a program that is not there, which Kando can download.
	ToolMissing ToolState = iota
	// ToolDownloading is a program Kando is downloading now.
	ToolDownloading
	// ToolReady is a program that is there, installed or downloaded.
	ToolReady
	// ToolUnavailable is a program that is not there, which Kando cannot download on this system.
	ToolUnavailable
)

// Tool is a row of the second step's card of programs: yt-dlp, ffmpeg or whisper-cli.
type Tool struct {
	Name string
	// Use is what the program is for.
	Use   string
	State ToolState
	// Progress is how much of a download has arrived, from 0 to 1, or negative while its size is not known yet.
	Progress float64
	// Err is why the last download of this program failed.
	Err string
}

// AI is what the second step says about the translation service: a line saying which model translates or what is missing, whether it is ready, and whether something is missing. Neither means it is not known yet, as the EveryAPI sign-in before it was checked.
type AI struct {
	Text    string
	Ready   bool
	Missing bool
}

// App is the state the guide reads and the clicks it reports. The rows and lines that hold words take the scope they are shown under, so they say them in the language it shows.
type App interface {
	live.State
	// OnboardingStep is the step shown, from 1 to Steps; 0 while the guide is closed.
	OnboardingStep() int
	// Language is the language subtitles are translated into, as the settings keep it, and LanguageChoices the ones the settings offer.
	Language() *string
	LanguageChoices() []string
	// OnboardingTargetName is how the interface names the target language value, one the settings keep such as 简体中文, as every screen that names it does.
	OnboardingTargetName(cx *core.Scope, value string) string
	// OnboardingSave writes the settings after the guide changed one of them.
	OnboardingSave()
	// OnboardingLearning is the languages of the videos the user watches, as codes ("en", "ja", …); OnboardingToggle adds or removes one.
	OnboardingLearning() []string
	OnboardingToggle(code string)
	// OnboardingTools is the programs Kando runs, which it downloads by itself when they are missing; OnboardingDownloadTool starts one again after its download failed.
	OnboardingTools(cx *core.Scope) []Tool
	OnboardingDownloadTool(name string)
	// OnboardingModels is the speech models the second step offers for the languages chosen.
	OnboardingModels(cx *core.Scope) []Model
	// OnboardingDownload starts downloading a model, OnboardingCancelDownload stops the download running, and OnboardingUseModel makes a downloaded model the one in use. OnboardingDownloading reports whether a download runs, as only one runs at a time.
	OnboardingDownload(id string)
	OnboardingCancelDownload()
	OnboardingUseModel(id string)
	OnboardingDownloading() bool
	// OnboardingDownloadsOnNext reports whether 继续 on the second step starts downloading the recommended model, so the button says so.
	OnboardingDownloadsOnNext() bool
	// OnboardingAI is the translation service's state.
	OnboardingAI(cx *core.Scope) AI
	// OpenSettings shows the settings at section, a settings/sectionid value.
	OpenSettings(section string)
	// OnboardingLink is the link typed into the third step, and OnboardingLinkError why the last one could not be added.
	OnboardingLink() *string
	OnboardingLinkError() string
	// OnboardingBack, OnboardingNext and OnboardingSkip are 上一步, the primary button, and 跳过 or Escape.
	OnboardingBack()
	OnboardingNext()
	OnboardingSkip()
}

// The sheet's measures, as the prototype draws it: 820 by 520, the art panel 340 wide, the content padded 26 30 20.
const (
	sheetWidth  = 820
	sheetHeight = 520
	artWidth    = 340
)

var (
	// sheetFill is the sheet's face, ink its text, and the grays its secondary text, as the prototype colors them.
	sheetFill = core.RGB(0xF6F6F8)
	ink       = core.RGB(0x1D1D1F)
	gray      = core.RGB(0x8A8A90)
	muted     = core.RGB(0x6A6A70)
	blue      = core.RGB(0x0A66D8)
	// cardRing is the hairline around a white card, and rowRule the line between a card's rows.
	cardRing = core.RGBA(0x000000, 0.1)
	rowRule  = core.RGBA(0x000000, 0.08)
	// The colors of a row's second line: progress, a failure, and what Kando cannot do, and of a link the field reads as one.
	progressBlue = core.RGB(0x0052B0)
	failedRed    = core.RGB(0xC4291C)
	warnOrange   = core.RGB(0xB25000)
	okGreen      = core.RGB(0x1B6B30)
)

// View is the guide over the window while App.OnboardingStep is not 0: the art on the left, the step on the right, and its buttons at the foot. Escape skips it, as 跳过 does; a click beside it does nothing, so what was typed is not lost. It is the prototype's own 820-wide sheet on every platform, held inside the window.
type View struct{ App App }

// Render implements core.Node.
func (v View) Render(cx *core.Scope) core.Node {
	a := v.App
	return ui.Dialog{
		TestID: "onboarding",
		Open:   live.Func(a, func() bool { return a.OnboardingStep() > 0 }),
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				a.OnboardingSkip()
				a.Changed()
			}
		},
		NoCloseOnBackdropPress: true,
		Bare:                   true,
		// TODO(nsgo): give base.Dialog (and ui.Dialog) a LabelFunc twin, so the sheet's accessible name follows the interface language as it changes; it is read once as the sheet opens, and a Dynamic would build the sheet again and take the link field's focus with it
		Label: locales.OnboardingWelcome(cx),
		Style: core.Style{Width: core.Pt(sheetWidth), Height: core.Pt(sheetHeight), CornerRadius: 16, Background: sheetFill,
			BorderWidth: 0.5, BorderColor: core.RGBA(0x000000, 0.22),
			Shadows: []core.BoxShadow{{Offset: core.Point{Y: 24}, Blur: 70, Color: core.RGBA(0x000000, 0.35)}}},
		Children: []core.Node{sheet{a}},
	}
}

// sheet is what the dialog holds, built as it opens: the art and the step side by side.
type sheet struct{ a App }

// Render implements core.Node.
func (s sheet) Render(cx *core.Scope) core.Node {
	a := s.a
	// While the sheet closes, it keeps showing the step it was on rather than the third step's field.
	last := 1
	step := live.Memo(cx, a, func() int {
		if n := a.OnboardingStep(); n > 0 {
			last = n
		}
		return last
	})
	return core.View{Style: core.Style{Direction: core.Row, FlexGrow: 1, MinHeight: core.Pt(0), AlignItems: core.AlignStretch},
		Children: []core.Node{art{a: a, step: step}, content{a: a, step: step}}}
}

// content is the right side: the step's number and dots and 跳过, its title and description, its body, and the buttons.
type content struct {
	a    App
	step func() int
}

// Render implements core.Node.
func (c content) Render(cx *core.Scope) core.Node {
	a, step := c.a, c.step
	skip := func() string {
		if step() == Steps {
			return locales.OnboardingLookAround(cx)
		}
		return locales.OnboardingSkip(cx)
	}
	head := core.View{Style: core.Style{Direction: core.Row, Gap: 10, AlignItems: core.AlignCenter}, Children: []core.Node{
		core.Text{ContentFunc: func() string { return locales.OnboardingStepCounter(cx, step(), Steps) },
			Style: core.Style{FontSize: 12, FontWeight: core.FontWeightSemibold, Color: gray, FontVariant: core.FontTabularNums}},
		dots{step},
		core.View{Style: core.Style{FlexGrow: 1}},
		flatButton(cx, "onboarding-skip", skip, live.Do(a, a.OnboardingSkip)),
	}}
	title := func() string { title, _ := heading(cx, step()); return title }
	description := func() string { _, description := heading(cx, step()); return description }
	titleText := core.View{Role: core.RoleHeader, AccessibilityLevel: func() int { return 1 }, Accessible: true, LabelFunc: title,
		Style:    core.Style{Margin: core.Edges{Top: 14}},
		Children: []core.Node{core.Text{ContentFunc: title, Style: core.Style{FontSize: 22, FontWeight: core.FontWeightBold, LetterSpacing: -0.22}}}}
	descriptionText := core.Text{ContentFunc: description, Style: core.Style{FontSize: 13, LineHeight: 20, Color: muted, Margin: core.Edges{Top: 6}}}
	// The step's body is built again for each step; its parts follow the app's state through their bindings.
	body := core.Dynamic(func(cx *core.Scope) core.Node {
		switch step() {
		case 1:
			return languagesStep{a}
		case 2:
			return modelsStep{a}
		}
		return linkStep{a}
	})
	scroll := core.Scroll{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0), Margin: core.Edges{Top: 15, Left: -4, Right: -4}},
		Children: []core.Node{core.View{Style: core.Style{Padding: core.Symmetric(3, 4), Gap: 16, AlignItems: core.AlignStretch}, Children: []core.Node{body}}}}
	return core.View{
		Style:    core.Style{FlexGrow: 1, MinWidth: core.Pt(0), Padding: core.Edges{Top: 26, Right: 30, Bottom: 20, Left: 30}, AlignItems: core.AlignStretch, Color: ink},
		Children: []core.Node{head, titleText, descriptionText, scroll, footer{a: a, step: step}},
	}
}

// heading is a step's title and the line under it, in the language shown under cx.
func heading(cx *core.Scope, step int) (title, description string) {
	switch step {
	case 1:
		return locales.OnboardingWelcome(cx), locales.OnboardingWelcomeDescription(cx)
	case 2:
		if onMac(cx) {
			return locales.OnboardingModelsTitle(cx), locales.OnboardingModelsDescriptionMac(cx)
		}
		return locales.OnboardingModelsTitle(cx), locales.OnboardingModelsDescriptionComputer(cx)
	}
	return locales.OnboardingLinkTitle(cx), locales.OnboardingLinkDescription(cx)
}

// dots are the step dots: 4 high, the current one 16 wide and the others 4, blue up to the current one.
type dots struct{ step func() int }

// Render implements core.Node.
func (d dots) Render(cx *core.Scope) core.Node {
	var row []core.Node
	for i := 1; i <= Steps; i++ {
		row = append(row, core.View{StyleFunc: func() core.Style {
			w, fill := core.Points(4), core.RGBA(0x000000, 0.14)
			if i == d.step() {
				w = 16
			}
			if i <= d.step() {
				fill = blue
			}
			return core.Style{Width: core.Pt(w), Height: core.Pt(4), CornerRadius: 2, Background: fill}
		}})
	}
	return core.View{Role: core.RoleGroup, Accessible: true, LabelFunc: func() string { return locales.OnboardingStepOf(cx, d.step(), Steps) },
		Style: core.Style{Direction: core.Row, Gap: 4, AlignItems: core.AlignCenter}, Children: row}
}

// footer is the line about where the work runs, 上一步 after the first step, and the primary button.
type footer struct {
	a    App
	step func() int
}

// Render implements core.Node.
func (f footer) Render(cx *core.Scope) core.Node {
	a, step := f.a, f.step
	privacy := core.View{Style: core.Style{Direction: core.Row, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 6, AlignItems: core.AlignCenter, Color: gray},
		Children: []core.Node{
			core.Icon{SVG: lockIcon, Style: core.Style{Width: core.Pt(11), Height: core.Pt(11), FlexShrink: 0}},
			core.Text{ContentFunc: func() string { return locales.OnboardingPrivacy(cx) }, MaxLines: 1, EllipsizeMode: core.EllipsizeTail,
				Style: core.Style{FontSize: 11, FlexShrink: 1}},
		}}
	back := core.Show{When: func() bool { return step() > 1 }, Then: func(cx *core.Scope) core.Node {
		return button(cx, func() string { return locales.OnboardingBack(cx) },
			ui.Button{Variant: ui.ButtonSecondary, TestID: "onboarding-back", OnPress: live.Do(a, a.OnboardingBack)})
	}}
	label := live.Func(a, func() string {
		switch step() {
		case 2:
			if a.OnboardingDownloadsOnNext() {
				return locales.OnboardingDownloadAndContinue(cx)
			}
		case Steps:
			if *a.OnboardingLink() != "" {
				return locales.OnboardingAddAndStart(cx)
			}
			return locales.OnboardingDone(cx)
		}
		return locales.OnboardingContinue(cx)
	})
	// A link the third step cannot add keeps the button off; its line says why.
	disabled := live.Func(a, func() bool { return step() == Steps && *a.OnboardingLink() != "" && !Addable(*a.OnboardingLink()) })
	next := button(cx, label, ui.Button{TestID: "onboarding-next", Disabled: disabled, OnPress: live.Do(a, a.OnboardingNext)})
	return core.View{Style: core.Style{Direction: core.Row, Gap: 8, AlignItems: core.AlignCenter, Margin: core.Edges{Top: 16}},
		Children: []core.Node{privacy, back, next}}
}

// button is b with the words text returns, which follow the step and the language: ui.Button takes its words as it is built, so it is built again, only when they change.
func button(cx *core.Scope, text func() string, b ui.Button) core.Node {
	words := core.NewMemo(cx, text)
	return core.Dynamic(func(*core.Scope) core.Node {
		b.Text = words.Get()
		return b
	})
}

// flatButton is the prototype's quiet gray button of the step's head, 跳过: its words alone, a fill under the pointer.
func flatButton(cx *core.Scope, id string, text func() string, press func()) core.Node {
	return base.Button{TestID: id, LabelFunc: text, OnPress: func(core.PressEvent) { press() },
		Style: core.Style{Height: core.Pt(24), Padding: core.Symmetric(0, 8), CornerRadius: 6, JustifyContent: core.JustifyCenter, FlexShrink: 0,
			Hover: &core.Style{Background: core.RGBA(0x000000, 0.05)}, Active: &core.Style{Background: core.RGBA(0x000000, 0.09)}},
		StyleFunc: func() core.Style { return core.Style{FocusVisible: ui.FocusRing(cx, 2)} },
		Children:  []core.Node{core.Text{ContentFunc: text, Style: core.Style{FontSize: 12, Color: gray}}}}
}
