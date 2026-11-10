package films

import (
	"strings"

	"kando/apps/kando/locales"
	"kando/apps/kando/sidebar"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// grid is the films that are done, as 16:9 posters across the page.
func grid(c *ui.Context, t *ui.Theme, a App, done []Film, source string) {
	title := locales.FilmsGridTitle()
	if source == sidebar.SourceAll {
		title = locales.SourceAll()
	}
	ui.Column(c).Margin(26, 0, 0, 0).Gap(14).Children(func() {
		sectionTitle(c, t, title, locales.FilmsGridCount(len(done)))
		width, _ := c.Size()
		ui.Grid(c).Columns(Columns(width)).GapX(CardGapX).GapY(CardGapY).AlignItems(ui.Start).Children(func() {
			for _, item := range done {
				c.Key("film-" + item.Key)
				doneCard(c, t, a, item)
			}
		})
	})
}

// doneCard is a film in the grid: the poster opens it, and ⋯ and a right click offer 播放, 在访达中显示, 重新处理, 重新翻译 and 从资料库移除, and 补译未译的句子 for a film whose translation left lines untranslated, which also has that button under its line.
func doneCard(c *ui.Context, t *ui.Theme, a App, item Film) {
	menu := filmMenu(a, item, breeze.PlatformOf(c))
	name := filmName(item)
	ui.Column(c).WidthPercent(100).MinWidth(0).Gap(9).ContextMenu(menu).Children(func() {
		thumb := ui.ButtonBase(c).Label(name).WidthPercent(100).AspectRatio(16.0/9).Radius(13).Clip().Background(thumbFill).TransformOrigin(0.5, 0.5).Border(0.5, ui.RGBA(0, 0, 0, 0.1))
		if thumb.Clicked() {
			a.Open(item)
		}
		if thumb.Hovered() {
			thumb.Shadow(0, 14, 30, 0, ui.RGBA(0, 0, 0, 0.22)).Scale(1.015, 1.015)
		} else {
			thumb.Shadow(0, 4, 12, 0, ui.RGBA(0, 0, 0, 0.12))
		}
		thumb.Children(func() {
			if img := a.Poster(item.VideoPath); img != nil {
				ui.Image(c, img).Absolute().Top(0).Left(0).WidthPercent(100).HeightPercent(100).Fit(ui.Cover)
			}
			if item.Language != "" {
				posterChip(c, locales.FilmsPosterBilingual(LanguageShort(item.Language))).Absolute().Left(9).Top(9).LetterSpacing(0.4).Bold()
			}
			if d := a.Duration(item.VideoPath); d > 0 {
				posterChip(c, Clock(d)).Absolute().Right(9).Bottom(9).FontWeight(600).FontFeatures("tnum")
			}
		})
		ui.Row(c).Gap(6).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, name).FontSize(13).FontWeight(600).FixedLineHeight(17).SingleLine()
				ui.Text(c, doneMeta(a, item)).FontSize(12).TextColor(t.TextMuted).Wrap().MaxLines(2)
				if item.JobID != "" && item.Untranslated > 0 {
					c.Key("film-translate-missing")
					if breeze.Button(c, locales.FilmsTranslateMissing()).Height(24).AlignSelf(ui.Start).Margin(5, 0, 0, 0).Clicked() {
						a.TranslateMissing(item)
					}
				}
			})
			c.Key("film-more")
			moreButton(c).Menu(menu)
		})
	})
}

// filmMenu is what can be done with a done film, from ⋯ or a right click on its poster or the hero. p is the platform of the Context the menu is built in, so its reveal item names the same file manager as the buttons beside it.
func filmMenu(a App, item Film, p breeze.Platform) func(m *ui.Menu) {
	return func(m *ui.Menu) {
		if m.Item(locales.FilmsPlay()).Chosen() {
			a.Open(item)
		}
		if m.Item(locales.FilmsShowInFinder(p.String())).Chosen() {
			a.Reveal(item.VideoPath)
		}
		if m.Item(locales.FilmsReprocess()).Chosen() {
			a.Reprocess(item)
		}
		if item.JobID != "" {
			if m.Item(locales.FilmsTranslateAgain()).Chosen() {
				a.TranslateAgain(item)
			}
			if item.Untranslated > 0 && m.Item(locales.FilmsTranslateMissing()).Chosen() {
				a.TranslateMissing(item)
			}
		}
		m.Separator()
		if m.Item(locales.FilmsRemoveFromLibrary()).Chosen() {
			a.AskRemove(item)
		}
	}
}

// posterChip is a small label over a poster, such as the subtitle language or the length, on frosted dark glass.
func posterChip(c *ui.Context, label string) ui.Element {
	return ui.Text(c, label).FontSize(10).TextColor(onPicture).Material(frosted{10, ui.RGBA(0, 0, 0, 0.42)}).Radius(6).Padding(2, 7).SingleLine()
}

// moreButton is ⋯ beside a poster's title: a 26-point round button with the prototype's three dots, gray while pressed.
func moreButton(c *ui.Context) ui.Element {
	b := ui.ButtonBase(c).Label(locales.FilmsMoreActions()).Size(26, 26).Radius(13).Shrink(0).AlignItems(ui.Center).Justify(ui.Center)
	switch {
	case b.Pressed():
		b.Background(ui.RGBA(0, 0, 0, 0.08))
	case b.Hovered():
		b.Background(ui.RGBA(0, 0, 0, 0.06))
	}
	b.Children(func() {
		ui.Icon(c, moreIcon).Size(14, 4).TextColor(ui.Hex("#3A3A3F"))
	})
	return b
}

// doneMeta is the line under a poster's title: where the film comes from, how long it is, that it has no subtitles yet, and how many lines its translation left untranslated.
func doneMeta(a App, item Film) string {
	parts := []string{SourceLabel(item)}
	if d := a.Duration(item.VideoPath); d > 0 {
		parts = append(parts, Clock(d))
	}
	if item.SubtitlePath == "" {
		parts = append(parts, locales.FilmsNoSubtitles())
	}
	if note := untranslatedNote(item); note != "" {
		parts = append(parts, note)
	}
	return strings.Join(parts, " · ")
}

// untranslatedNote says how many lines of a done film the translation left untranslated, as the prototype words a translation that failed for some lines; "" when it left none.
func untranslatedNote(item Film) string {
	if item.Untranslated <= 0 {
		return ""
	}
	return locales.FilmsUntranslated(item.Untranslated)
}
