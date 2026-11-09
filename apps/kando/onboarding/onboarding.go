// Package onboarding draws the first-run guide: a sheet of three steps over the window that picks the languages, gets a speech model onto this Mac and takes the first link. It draws and hands the clicks back; the settings it changes, the download and the library it opens are the app's.
package onboarding

import (
	"runtime"

	"kando/apps/kando/locales"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// Steps is how many steps the guide has.
const Steps = 3

// onMac is set on macOS, where the guide calls the machine whisper runs on 这台 Mac; elsewhere it is 这台电脑.
var onMac = runtime.GOOS == "darwin"

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

// AI is what the second step says about the translation service: a line saying which model translates or what is missing, whether it is ready, and whether something is missing. Neither means it is not known yet, as the EveryAPI sign-in before it was checked.
type AI struct {
	Text    string
	Ready   bool
	Missing bool
}

// App is the state the guide reads and the clicks it reports.
type App interface {
	// OnboardingStep is the step shown, from 1 to Steps; 0 while the guide is closed.
	OnboardingStep() int
	// Language is the language subtitles are translated into, as the settings keep it, and LanguageChoices the ones the settings offer.
	Language() *string
	LanguageChoices() []string
	// OnboardingSave writes the settings after the guide changed one of them.
	OnboardingSave()
	// OnboardingLearning is the languages of the videos the user watches, as codes ("en", "ja", …); OnboardingToggle adds or removes one.
	OnboardingLearning() []string
	OnboardingToggle(code string)
	// OnboardingModels is the speech models the second step offers for the languages chosen.
	OnboardingModels() []Model
	// OnboardingDownload starts downloading a model, OnboardingCancelDownload stops the download running, and OnboardingUseModel makes a downloaded model the one in use. OnboardingDownloading reports whether a download runs, as only one runs at a time.
	OnboardingDownload(id string)
	OnboardingCancelDownload()
	OnboardingUseModel(id string)
	OnboardingDownloading() bool
	// OnboardingDownloadsOnNext reports whether 继续 on the second step starts downloading the recommended model, so the button says so.
	OnboardingDownloadsOnNext() bool
	// OnboardingAI is the translation service's state.
	OnboardingAI() AI
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
	sheetFill = ui.Hex("#F6F6F8")
	ink       = ui.Hex("#1D1D1F")
	gray      = ui.Hex("#8A8A90")
	muted     = ui.Hex("#6A6A70")
	blue      = ui.Hex("#0A66D8")
	// cardRing is the hairline around a white card, and rowRule the line between a card's rows.
	cardRing = ui.RGBA(0, 0, 0, 0.1)
	rowRule  = ui.RGBA(0, 0, 0, 0.08)
)

// Show draws the guide while a.OnboardingStep() is not 0: the art on the left, the step on the right, and its buttons at the foot. Escape skips it, as 跳过 does.
func Show(c *ui.Context, a App) {
	step := a.OnboardingStep()
	if step < 1 {
		return
	}
	open := true
	panel := breeze.Sheet(c, &open, func() {
		ui.Row(c).Width(sheetWidth).Height(sheetHeight).AlignItems(ui.Stretch).Children(func() {
			art(c, a, step)
			content(c, a, step)
		})
	})
	panel.Padding(0).Gap(0).Radius(16).Clip().Background(sheetFill).Label(locales.OnboardingWelcome()).
		Shadow(0, 24, 70, 0, ui.RGBA(0, 0, 0, 0.35)).Border(0.5, ui.RGBA(0, 0, 0, 0.22))
	if !open {
		a.OnboardingSkip()
	}
}

// content is the right side: the step's number and dots and 跳过, its title and description, its body, and the buttons.
func content(c *ui.Context, a App, step int) {
	ui.Column(c).Grow(1).MinWidth(0).Padding(26, 30, 20, 30).AlignItems(ui.Stretch).TextColor(ink).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.OnboardingStepCounter(step, Steps)).FontSize(12).FontWeight(600).TextColor(gray).FontFeatures("tnum")
			dots(c, step)
			ui.Box(c).Grow(1)
			skip := locales.OnboardingSkip()
			if step == Steps {
				skip = locales.OnboardingLookAround()
			}
			c.Key("onboarding-skip")
			if breeze.FlatButton(c, skip).Height(24).FontSize(12).TextColor(gray).Clicked() {
				a.OnboardingSkip()
			}
		})
		title, description := heading(step)
		ui.Text(c, title).FontSize(22).Bold().LetterSpacing(-0.22).Margin(14, 0, 0, 0).Role(ui.RoleHeading).Level(1)
		ui.Text(c, description).FontSize(13).FixedLineHeight(20).TextColor(muted).Margin(6, 0, 0, 0).Wrap()
		ui.Scroll(c).Grow(1).MinHeight(0).Margin(15, -4, 0, -4).Children(func() {
			ui.Column(c).Padding(3, 4).Gap(16).AlignItems(ui.Stretch).Children(func() {
				switch step {
				case 1:
					languagesStep(c, a)
				case 2:
					modelsStep(c, a)
				default:
					linkStep(c, a)
				}
			})
		})
		footer(c, a, step)
	})
}

// heading is a step's title and the line under it.
func heading(step int) (title, description string) {
	switch step {
	case 1:
		return locales.OnboardingWelcome(), locales.OnboardingWelcomeDescription()
	case 2:
		if onMac {
			return locales.OnboardingModelsTitle(), locales.OnboardingModelsDescriptionMac()
		}
		return locales.OnboardingModelsTitle(), locales.OnboardingModelsDescriptionComputer()
	}
	return locales.OnboardingLinkTitle(), locales.OnboardingLinkDescription()
}

// dots are the step dots: 4 high, the current one 16 wide and the others 4, blue up to the current one.
func dots(c *ui.Context, step int) {
	ui.Row(c).Gap(4).AlignItems(ui.Center).Label(locales.OnboardingStepOf(step, Steps)).Children(func() {
		for i := 1; i <= Steps; i++ {
			w := float32(4)
			if i == step {
				w = 16
			}
			fill := ui.RGBA(0, 0, 0, 0.14)
			if i <= step {
				fill = blue
			}
			ui.Box(c).Size(w, 4).Radius(2).Background(fill)
		}
	})
}

// footer is the line about where the work runs, 上一步 after the first step, and the primary button.
func footer(c *ui.Context, a App, step int) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Margin(16, 0, 0, 0).Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Gap(6).AlignItems(ui.Center).TextColor(gray).Children(func() {
			ui.Icon(c, lockIcon).Size(11, 11).Shrink(0)
			ui.Text(c, locales.OnboardingPrivacy()).FontSize(11).SingleLine()
		})
		if step > 1 {
			c.Key("onboarding-back")
			if breeze.Button(c, locales.OnboardingBack()).Height(30).PaddingX(14).Clicked() {
				a.OnboardingBack()
			}
		}
		label, disabled := locales.OnboardingContinue(), false
		switch step {
		case 2:
			if a.OnboardingDownloadsOnNext() {
				label = locales.OnboardingDownloadAndContinue()
			}
		case Steps:
			label = locales.OnboardingDone()
			if *a.OnboardingLink() != "" {
				label = locales.OnboardingAddAndStart()
				_, ok := LinkHint(*a.OnboardingLink())
				disabled = !ok
			}
		}
		c.Key("onboarding-next")
		if breeze.PrimaryButton(c, label).Height(30).PaddingX(18).Disabled(disabled).Clicked() && !disabled {
			a.OnboardingNext()
		}
	})
}
