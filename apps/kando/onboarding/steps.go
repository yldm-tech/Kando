package onboarding

import (
	"slices"

	"kando/apps/kando/locales"
	"kando/apps/kando/settings/sectionid"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// card is a white card with the prototype's hairline and soft shadow, its rows stacked.
func card(c *ui.Context, fn func()) ui.Element {
	return ui.Column(c).AlignItems(ui.Stretch).Radius(12).Background(white).Border(0.5, cardRing).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.04)).Children(fn)
}

// sectionLabel is the 12 point label over a group of the step's body.
func sectionLabel(c *ui.Context, text string) ui.Element {
	return ui.Text(c, text).FontSize(12).FontWeight(600).TextColor(muted)
}

// note is the 11 point gray line under a group.
func note(c *ui.Context, text string) ui.Element {
	return ui.Text(c, text).FontSize(11).FixedLineHeight(16).TextColor(gray).Wrap()
}

// languagesStep is the first step: the language subtitles are translated into, and tiles for the languages of the videos the user watches.
func languagesStep(c *ui.Context, a App) {
	card(c, func() {
		ui.Row(c).Height(44).Padding(0, 8, 0, 14).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.OnboardingTranslateInto()).FontSize(13).Grow(1)
			c.Key("onboarding-language")
			if breeze.Select(c, a.Language(), a.LanguageChoices()).Label(locales.OnboardingTranslateInto()).Changed() {
				a.OnboardingSave()
			}
		})
	})
	target := *a.Language()
	chosen := Effective(a.OnboardingLearning(), target)
	ui.Column(c).Gap(8).AlignItems(ui.Stretch).Children(func() {
		sectionLabel(c, locales.OnboardingWatchLanguages())
		ui.Grid(c).Columns(3).GapX(8).GapY(8).Role(ui.RoleGroup).Label(locales.OnboardingWatchLanguagesLabel()).Children(func() {
			for _, code := range tiles(target) {
				c.Key("onboarding-tile-" + code)
				if tile(c, code, slices.Contains(chosen, code)).Clicked() {
					a.OnboardingToggle(code)
				}
			}
		})
	})
	note(c, locales.OnboardingLanguagesNote())
}

// tile is a language's tile: its own name over its name in the interface's language, and a check circle at its top right, blue and ringed in blue while chosen.
func tile(c *ui.Context, code string, on bool) ui.Element {
	own := autonyms[code]
	b := ui.ButtonBase(c).Label(own).Checked(on).Column().Height(52).Padding(0, 12).Gap(1).Radius(10).Justify(ui.Center).AlignItems(ui.Start)
	if on {
		b.Background(ui.RGBA(10, 102, 216, 0.07)).InsetShadow(0, 0, 0, 1.5, blue)
	} else {
		b.Background(white).Border(0.5, cardRing).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.04))
		if b.Hovered() {
			b.Background(ui.Hex("#FAFAFC"))
		}
	}
	b.Children(func() {
		ui.Text(c, own).FontSize(13).FontWeight(600).TextColor(ink).Lang(code).SingleLine()
		ui.Text(c, languageName(code)).FontSize(11).TextColor(gray).SingleLine()
		mark := checkOff
		if on {
			mark = checkOn
		}
		ui.Image(c, mark).Size(16, 16).Absolute().Top(9).Right(9)
	})
	return b
}

// modelsStep is the second step: the speech models recommended for the languages chosen, each with its download, and the translation service.
func modelsStep(c *ui.Context, a App) {
	models := a.OnboardingModels()
	busy := a.OnboardingDownloading()
	card(c, func() {
		ui.Row(c).Height(38).Padding(0, 14).AlignItems(ui.Center).Children(func() {
			ui.Text(c, modelsHead(models)).FontSize(12).TextColor(muted).Grow(1)
		})
		for _, m := range models {
			ui.Box(c).Height(0.5).Shrink(0).Background(rowRule)
			modelRow(c, a, m, busy)
		}
	})
	note(c, modelsNote(models))
	ai := a.OnboardingAI()
	card(c, func() {
		ui.Row(c).MinHeight(46).Padding(5, 14).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, locales.OnboardingTranslationService()).FontSize(13).TextColor(ink)
				color := muted
				if ai.Missing {
					color = ui.Hex("#B25000")
				}
				ui.Text(c, ai.Text).FontSize(11).TextColor(color).Wrap()
			})
			if ai.Ready {
				ui.Image(c, readyMark).Size(16, 16).Shrink(0)
			}
			c.Key("onboarding-ai")
			if breeze.Button(c, locales.OnboardingSettings()).Height(24).Shrink(0).Clicked() {
				a.OpenSettings(sectionid.AI)
			}
		})
	})
}

// modelRow is a speech model: its name and size, and what can be done with it: 下载, a progress bar and 取消 while it downloads, 使用 once it is here, and a green check while it is in use.
func modelRow(c *ui.Context, a App, m Model, busy bool) {
	ui.Row(c).MinHeight(46).Padding(5, 14).Gap(10).AlignItems(ui.Center).Label(m.Name).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, locales.OnboardingSpeechModel(m.Name)).FontSize(13).TextColor(ink).SingleLine()
			sub, color := modelSub(m)
			ui.Text(c, sub).FontSize(11).TextColor(color).Wrap()
		})
		switch m.State {
		case ModelInUse:
			ui.Image(c, readyMark).Size(16, 16).Shrink(0).Label(locales.OnboardingInUse())
		case ModelReady:
			c.Key("onboarding-use-" + m.ID)
			if breeze.Button(c, locales.OnboardingUse()).Height(24).Shrink(0).Clicked() {
				a.OnboardingUseModel(m.ID)
			}
		case ModelDownloading:
			breeze.Progress(c, m.Progress).Width(72).Shrink(0).Label(locales.OnboardingDownloadProgress())
			c.Key("onboarding-cancel-" + m.ID)
			if breeze.Button(c, locales.OnboardingCancel()).Height(24).Shrink(0).Clicked() {
				a.OnboardingCancelDownload()
			}
		default:
			label := locales.OnboardingDownload()
			if m.Err != "" {
				label = locales.OnboardingRetry()
			}
			c.Key("onboarding-download-" + m.ID)
			if breeze.Button(c, label).Height(24).Shrink(0).Disabled(busy).Tooltip(locales.OnboardingDownloadTooltip(m.Name, m.Size)).Clicked() && !busy {
				a.OnboardingDownload(m.ID)
			}
		}
	})
}

// modelSub is the line under a model's name and its color: the download's progress in blue while it runs, why it failed in red, otherwise its size and what it suits.
func modelSub(m Model) (string, ui.Color) {
	switch m.State {
	case ModelInUse:
		return locales.OnboardingModelInUse(), muted
	case ModelReady:
		return locales.OnboardingModelDownloaded(m.Blurb), muted
	case ModelDownloading:
		if m.Progress < 0 {
			return locales.OnboardingModelConnecting(m.Size), ui.Hex("#0052B0")
		}
		return locales.OnboardingModelProgress(int(m.Progress*100), m.Size), ui.Hex("#0052B0")
	}
	if m.Err != "" {
		return locales.OnboardingModelFailed(m.Err), ui.Hex("#C4291C")
	}
	return locales.OnboardingModelAbout(m.Size, m.Blurb), muted
}

// modelsHead is the card's head: what is in use, or what is missing.
func modelsHead(models []Model) string {
	for _, m := range models {
		if m.State == ModelInUse {
			return locales.OnboardingModelsReady()
		}
	}
	for _, m := range models {
		if m.State == ModelDownloading {
			return locales.OnboardingModelsDownloading(m.Name)
		}
	}
	return locales.OnboardingModelsMissing()
}

// modelsNote is the line under the models.
func modelsNote(models []Model) string {
	for _, m := range models {
		if m.State == ModelDownloading {
			return locales.OnboardingModelsNoteDownloading()
		}
	}
	for _, m := range models {
		if m.State == ModelInUse {
			return locales.OnboardingModelsNoteInUse()
		}
	}
	return locales.OnboardingModelsNoteLater()
}

// linkStep is the third step: the field for the first link and what it reads as.
func linkStep(c *ui.Context, a App) {
	value := a.OnboardingLink()
	ui.Column(c).Gap(6).AlignItems(ui.Stretch).Children(func() {
		field := ui.Row(c).Height(44).Radius(10).Padding(0, 14, 0, 13).Gap(8).AlignItems(ui.Center).Background(white)
		field.Shadow(0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.05)).Border(0.5, ui.RGBA(0, 0, 0, 0.14))
		focused := false
		field.Children(func() {
			ui.Icon(c, linkIcon).Size(15, 15).TextColor(gray).Shrink(0)
			c.Key("onboarding-link")
			in := ui.TextInputBase(c, value).Placeholder(locales.OnboardingLinkPlaceholder()).Label(locales.OnboardingLinkLabel()).Grow(1).MinWidth(0).FontSize(14).FocusRing(false).AutoFocus()
			if in.Submitted() {
				if _, ok := LinkHint(*value); ok {
					a.OnboardingNext()
				}
			}
			focused = in.Focused()
		})
		if focused {
			field.Shadow(0, 0, 0, 3, blue.Alpha(0.5))
		}
		hint, ok := LinkHint(*value)
		color := ui.Hex("#1B6B30")
		switch {
		case a.OnboardingLinkError() != "" && *value != "":
			hint, color = a.OnboardingLinkError(), ui.Hex("#C4291C")
		case hint == "":
			hint, color = locales.OnboardingLinkHintEmpty(), gray
		case !ok:
			color = ui.Hex("#C4291C")
		}
		ui.Text(c, hint).FontSize(12).TextColor(color).MinHeight(16).Wrap()
	})
}
