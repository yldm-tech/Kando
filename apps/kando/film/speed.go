package film

import (
	"math"
	"strconv"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/core"
)

// Rates are the speeds a film plays at, slowest first, as multiples of the speed it was recorded at. The speed menu lists them and ⇧, and ⇧. step through them. The steps around 1 are a tenth, for slowing a film down a little to listen.
var Rates = [...]float64{0.5, 0.75, 0.8, 0.9, 1, 1.1, 1.25, 1.5, 2}

// RateWorks reports whether the system's player on the system cx runs on plays a film at core.Video's Rate. Where it does not, the transport has no speed button and ⇧, and ⇧. do nothing, rather than name a speed the film does not play at.
func RateWorks(cx *core.Scope) bool {
	// TODO(nsgo#110): NSGo's player on Linux ignores core.Video's Rate and plays every film at the speed it was recorded at; once it plays at Rate, as MyGo's player and its atempo sound did, drop this and show the speed button and the speed keys on Linux again https://github.com/nsgo-dev/nsgo/issues/110
	return cx.Env().OS() != core.OSLinux
}

// RateText is how the player writes a speed, as "1.5×", in the language the interface shows under cx.
func RateText(cx *core.Scope, rate float64) string { return RateTextIn(locales.For(cx), rate) }

// RateTextIn is RateText in the language of m, for a message on the picture.
func RateTextIn(m locales.Messages, rate float64) string {
	return m.FilmSpeedRate(strconv.FormatFloat(rate, 'f', -1, 64))
}

// speedMenu is the transport's speed button, named for the speed the film plays at, and the menu it opens above it, as the subtitle button's: the speeds from slowest to fastest, the one playing checked.
func (s *screen) speedMenu(cx *core.Scope) core.Node {
	a := s.a
	rate := live.Func(a, func() string { return RateText(cx, a.Rate()) })
	label := func() string { return locales.FilmSpeedLabel(cx, rate()) }
	keys := live.Func(a, func() [2]string {
		slower, faster := a.SpeedKeys(cx.Env().OS())
		return [2]string{slower, faster}
	})
	tooltip := func() string {
		k := keys()
		return locales.FilmSpeed(cx) + " " + cx.KeyLabel(k[0]) + " / " + cx.KeyLabel(k[1])
	}
	trigger := []core.Node{core.Text{ContentFunc: rate, MaxLines: 1, Style: core.Style{FontSize: 12, LineHeight: 16, FontVariant: core.FontTabularNums, Color: onGlass}}}
	rows := func(cx *core.Scope) []core.Node {
		out := []core.Node{menuHeading(func() string { return locales.FilmSpeed(cx) })}
		for _, r := range Rates {
			out = append(out, s.speedRow(cx, r))
		}
		return []core.Node{core.View{Style: core.Style{Width: core.Pt(168)}, Children: out}}
	}
	return s.transportMenu(cx, "film-speed", a.SpeedMenu(), label, tooltip, speedButtonWidth(cx), trigger, rows)
}

// speedButtonWidth is the speed button's width: the widest of the speeds' labels as the button draws them, and its padding, so the scrubber beside it keeps its length whatever the speed on every platform. It is at least 50, as before anything is measured.
func speedButtonWidth(cx *core.Scope) core.Points {
	widest := float32(0)
	for _, r := range Rates {
		m := cx.MeasureText(RateText(cx, r), core.Font{Size: 12, Variant: core.FontTabularNums})
		widest = max(widest, float32(m.Width))
	}
	return core.Points(max(50, float32(math.Ceil(float64(widest)))+16))
}

// speedRow is a radio row of the speed menu: a check while the film plays at rate, the speed, and for 1× a note that it is the normal speed.
func (s *screen) speedRow(cx *core.Scope, rate float64) core.Node {
	a := s.a
	on := live.Func(a, func() bool { return a.Rate() == rate })
	name := func() string { return RateText(cx, rate) }
	children := []core.Node{check(on), core.Text{ContentFunc: name, Style: core.Style{FontSize: 13, LineHeight: 18, FontVariant: core.FontTabularNums, FlexGrow: 1, Color: onGlass}}}
	if rate == 1 {
		children = append(children, core.Text{ContentFunc: func() string { return locales.FilmSpeedNormal(cx) }, Style: core.Style{FontSize: 11, LineHeight: 18, FlexShrink: 0, Color: onGlass, Opacity: core.Alpha(0.5)}})
	}
	return menuRow(cx, "film-speed-"+strconv.FormatFloat(rate, 'f', -1, 64), core.RoleRadio, name, on, live.Do(a, func() {
		a.SetRate(rate)
		*a.SpeedMenu() = false
	}), core.Style{AlignItems: core.AlignCenter}, children...)
}
