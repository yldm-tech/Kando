package films

import (
	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// noticeLook is the message bar's state, which its parts follow: whether it shows, what kind it is, what it says, and its buttons.
type noticeLook struct {
	open          bool
	success       bool
	text, section string
	// film is the film the bar's 打开 opens: the film it names, while it is still in the library.
	film Film
}

// notice is the message bar under the top bar, as the prototype draws it: amber with a warning triangle for what went wrong, green with a check mark for what worked, 打开设置 when a setting fixes what it says, 打开 when it names a film in the library, and × to close it.
type notice struct{ a App }

// Render implements core.Node.
func (n notice) Render(cx *core.Scope) core.Node {
	a := n.a
	look := live.Memo(cx, a, func() noticeLook {
		bar := a.Notice()
		l := noticeLook{open: bar.Open, success: bar.Kind == "success", text: bar.Text, section: bar.Section}
		// The film named may have been removed since the bar opened; then there is nothing to open and no button.
		if bar.Film != "" {
			l.film, _ = a.JobFilm(bar.Film)
		}
		return l
	})
	return core.Show{When: func() bool { return look().open }, Then: func(cx *core.Scope) core.Node { return noticeBar(cx, a, look) }}
}

// The message bar's colors: its fill, hairline and ink, amber for a warning and green for news that something worked.
var (
	warnFill, warnRing, warnInk = core.RGB(0xFFF7E3), core.RGBA(0xB47800, 0.25), core.RGB(0x5A3A00)
	okFill, okRing, okInk       = core.RGB(0xEAF7EE), core.RGBA(0x1E823C, 0.25), core.RGB(0x14532D)
)

func noticeBar(cx *core.Scope, a App, look func() noticeLook) core.Node {
	ink := func() core.Color {
		if look().success {
			return okInk
		}
		return warnInk
	}
	mark := core.SVGView{Style: core.Style{Width: core.Pt(16), Height: core.Pt(16), FlexShrink: 0}, SVGFunc: func() *core.SVG {
		if look().success {
			return okMark
		}
		return warningMark
	}}
	text := core.Text{ContentFunc: func() string { return look().text }, Style: core.Style{FontSize: 13, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)},
		StyleFunc: func() core.Style { return core.Style{Color: ink()} }}
	settings := core.Show{When: func() bool { return look().section != "" }, Then: func(*core.Scope) core.Node {
		return textButton(locales.FilmsOpenSettings, ui.Button{Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "notice-settings",
			OnPress: live.Do(a, func() { a.OpenSettings(look().section) })})
	}}
	open := core.Show{When: func() bool { return look().film.Key != "" }, Then: func(*core.Scope) core.Node {
		return textButton(locales.FilmsNoticeOpen, ui.Button{Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "notice-film",
			OnPress: live.Do(a, func() {
				item := look().film
				a.Notice().Open = false
				a.Open(item)
			})})
	}}
	close := base.Button{TestID: "notice-close", LabelFunc: func() string { return locales.FilmsCloseNotice(cx) },
		OnPress: live.On(a, func(core.PressEvent) { a.Notice().Open = false }),
		Style: core.Style{Width: core.Pt(24), Height: core.Pt(24), CornerRadius: 12, FlexShrink: 0, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
			Cursor: core.CursorDefault, Hover: &core.Style{Background: core.RGBA(0x000000, 0.06)}},
		Children: []core.Node{core.Text{Content: "×", Style: core.Style{FontSize: 15}, StyleFunc: func() core.Style { return core.Style{Color: ink()} }}}}
	// The bar is 42 points high with one line of text, its hairline included.
	// TODO(nsgo#144): give the bar the status role, so assistive technology reads it as MyGo's RoleStatus did; NSGo's roles have none, and a polite live region announces what it says instead https://github.com/nsgo-dev/nsgo/issues/144
	return core.View{
		TestID:     "films-notice",
		LiveRegion: core.LiveRegionPolite,
		LabelFunc:  func() string { return look().text },
		Style: core.Style{Direction: core.Row, MinHeight: core.Pt(42), Margin: core.Edges{Bottom: 14}, Padding: core.Edges{Top: 8.5, Right: 10, Bottom: 8.5, Left: 14}, Gap: 10,
			AlignItems: core.AlignCenter, CornerRadius: 14, BorderWidth: 0.5},
		StyleFunc: func() core.Style {
			if look().success {
				return core.Style{Background: okFill, BorderColor: okRing}
			}
			return core.Style{Background: warnFill, BorderColor: warnRing}
		},
		Children: []core.Node{mark, text, settings, open, close},
	}
}
