package films

import (
	"path/filepath"
	"strings"
	"time"

	"kando/packages/core"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

// onPicture is the text over a poster or the hero, which are a picture or a dark color in either theme.
var onPicture = ui.Hex("#FFFFFF")

// frosted is the prototype's backdrop-filter glass: what is under the element blurred by blur DIPs, under tint.
type frosted struct {
	blur float32
	tint ui.Color
}

func (f frosted) PaintMaterial(p *ui.Painter, box ui.Rect, radii [4]float32) {
	glass.Blur{Radius: f.blur}.PaintMaterial(p, box, radii)
	p.Fill(box, f.tint, radii[0])
}

// HeroFilm is the film the hero shows: the newest one a job finished. items is newest first, as the shelf lists them.
func HeroFilm(items []Film) (Film, bool) {
	for _, item := range items {
		if item.JobID != "" && item.Stage == core.StageComplete {
			return item, true
		}
	}
	return Film{}, false
}

// resumeFrom is where 继续 picks up a film: where it stopped, unless that is within 3 seconds of either end.
func resumeFrom(a App, item Film) (time.Duration, bool) {
	at := a.ResumeAt(item.VideoPath)
	if at < 3*time.Second {
		return 0, false
	}
	if d := a.Duration(item.VideoPath); d > 0 && at > d-3*time.Second {
		return 0, false
	}
	return at, true
}

// hero is the large card of the film finished last: its poster under a dark gradient, its title, its first bilingual line, and play or continue. A click elsewhere on it opens the film, and a right click offers what the film's poster in the grid does.
func hero(c *ui.Context, a App, item Film) {
	box := ui.ButtonBase(c).Label(filmName(item)).Description("最近完成").Height(360).Radius(18).Clip().Background(posterColor(item.Key)).Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, 0.16)).ContextMenu(filmMenu(a, item))
	if box.Clicked() {
		a.Open(item)
	}
	box.Children(func() {
		if img := a.Poster(item.VideoPath); img != nil {
			ui.Image(c, img).Absolute().Top(0).Left(0).WidthPercent(100).HeightPercent(100).Fit(ui.Cover)
		}
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().LinearGradient(ui.LinearGradient{Angle: 90, Stops: []ui.GradientStop{
			{Color: ui.RGBA(0, 0, 0, 0.62), At: 0},
			{Color: ui.RGBA(0, 0, 0, 0.35), At: 0.42},
			{Color: ui.RGBA(0, 0, 0, 0), At: 0.7},
		}})
		// Its lines are as high as the prototype's, which the browser gives the fonts, so the text block rises from the bottom as far as it does there.
		ui.Column(c).Absolute().Left(36).Right(36).Bottom(34).MaxWidth(520).Gap(10).Children(func() {
			kicker := "最近完成"
			if item.Language != "" {
				kicker += " · " + item.Language + "双语"
			}
			ui.Text(c, kicker).FontSize(11).FixedLineHeight(16).Bold().LetterSpacing(1.2).TextColor(onPicture.Alpha(0.85))
			ui.Text(c, filmName(item)).FontSize(34).Bold().FixedLineHeight(40).LetterSpacing(-0.4).TextColor(onPicture).MaxLines(2)
			ui.Text(c, heroMeta(a, item)).FontSize(13).FixedLineHeight(16).TextColor(onPicture.Alpha(0.85))
			if translation, original := a.Quote(item); translation != "" || original != "" {
				ui.Column(c).AlignSelf(ui.Start).Gap(2).Padding(10, 14).Radius(12).Material(frosted{14, ui.RGBA(0, 0, 0, 0.28)}).Border(0.5, onPicture.Alpha(0.18)).Children(func() {
					if translation != "" {
						ui.Text(c, "“"+translation+"”").FontSize(15).FixedLineHeight(21).FontWeight(600).TextColor(onPicture).MaxLines(2)
					}
					if original != "" {
						ui.Text(c, original).FontSize(12).FixedLineHeight(15).TextColor(onPicture).MaxLines(2)
					}
				})
			}
			ui.Row(c).Gap(10).Margin(6, 0, 0, 0).Wrap().Children(func() {
				at, resume := resumeFrom(a, item)
				c.Key("hero-play")
				label := "播放"
				if resume {
					label = "从 " + Clock(at) + " 继续"
				}
				if heroButton(c, label, true).Clicked() {
					a.OpenAt(item, at)
				}
				c.Key("hero-alt")
				if resume {
					if heroButton(c, "从头看", false).Clicked() {
						a.OpenAt(item, 0)
					}
				} else if heroButton(c, "在访达中显示", false).Clicked() {
					a.Reveal(item.VideoPath)
				}
			})
		})
	})
}

// heroMeta is the line under the hero's title: where the film comes from, how long it is and the subtitle file it has.
func heroMeta(a App, item Film) string {
	parts := []string{SourceLabel(item)}
	if d := a.Duration(item.VideoPath); d > 0 {
		parts = append(parts, Clock(d))
	}
	if item.SubtitlePath != "" {
		parts = append(parts, "已导出 "+filepath.Base(item.SubtitlePath))
	}
	return strings.Join(parts, " · ")
}

// heroButton is one of the hero's pill buttons: white with a play glyph for the main one, frosted for the other.
func heroButton(c *ui.Context, label string, main bool) ui.Element {
	b := ui.ButtonBase(c).Label(label).Height(34).Radius(17).Gap(8).Shrink(0)
	fill, ink := onPicture.Alpha(0.2), onPicture
	if main {
		fill, ink = onPicture, ui.Hex("#111114")
		b.Padding(0, 18, 0, 14)
	} else {
		b.Padding(0, 16).Border(0.5, onPicture.Alpha(0.35))
	}
	switch {
	case b.Pressed():
		fill = fill.Mix(ui.Hex("#000000"), 0.12)
	case b.Hovered():
		fill = fill.Mix(ui.Hex("#000000"), 0.05)
	}
	if main {
		b.Background(fill)
	} else {
		b.Material(frosted{16, fill})
	}
	b.Children(func() {
		if main {
			ui.Icon(c, playIcon).Size(12, 13).TextColor(ink)
		}
		weight := 500
		if main {
			weight = 600
		}
		ui.Text(c, label).FontSize(14).FontWeight(weight).TextColor(ink)
	})
	return b
}

// filmName is the film's full name, or its short title when it has none.
func filmName(item Film) string {
	if item.Name != "" {
		return item.Name
	}
	return item.Title
}
