package film

import (
	"strconv"
	"strings"
	"time"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// ModeName is what the player calls a subtitle mode, on its button, in its menu and on the OSD, in the language the interface shows under cx.
func ModeName(cx *core.Scope, mode string) string { return subtitle.ModeName(cx, mode) }

// ModeNameIn is ModeName in the language of m, for a message on the picture.
func ModeNameIn(m locales.Messages, mode string) string { return subtitle.ModeNameIn(m, mode) }

// modeChoices are the mode menu's rows, in the prototype's order (template 619-636), with the key that reaches each.
var modeChoices = []struct {
	mode string
	key  string
}{
	{SubOff, "v"},
	{SubOriginal, "s"},
	{SubTranslation, "s"},
	{SubBilingual, "s"},
}

// modeNote is the line under a row of the mode menu, or "" for a row without one. Translation Only's says what the picture shows for a line not translated yet, which untranslated, the setting, picks (pictureLines).
func modeNote(cx *core.Scope, mode, untranslated string) string {
	switch mode {
	case SubOff:
		return locales.FilmModeOffNote(cx)
	case SubTranslation:
		if untranslated == UntranslatedBlank {
			return locales.FilmModeTranslationBlankNote(cx)
		}
		return locales.FilmModeTranslationNote(cx)
	}
	return ""
}

// menuGlass is the glass of the transport's menus, which open above it.
var menuGlass = glassSpec{theme.GlassCard, 14}

// transportMenu is a transport button that opens a menu above it, as the subtitle and the speed buttons do: open is the app's flag, which the menu sets as it opens and closes; trigger is what the button shows, label its name, and rows what the menu holds. The prototype opens the menu above the button, its right edge 6 past the button's and its bottom 14 above the button's top, over the picture rather than the panel.
func (s *screen) transportMenu(cx *core.Scope, id string, open *bool, label func() string, tooltip func() string, width core.Points, trigger []core.Node, rows func(cx *core.Scope) []core.Node) core.Node {
	a := s.a
	isOpen := live.Func(a, func() bool { return *open })
	return base.Popover{
		TestID: id, LabelFunc: label, Tooltip: tip(cx, tooltip),
		Open: isOpen,
		OnOpenChange: func(now bool, _ base.Reason) {
			*open = now
			a.Changed()
		},
		Placement: core.PlaceTop, Align: core.AlignEnd, Offset: 14,
		Style: core.Style{Direction: core.Row, Height: core.Pt(30), Padding: core.Symmetric(0, 8), CornerRadius: 15, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, FlexShrink: 0,
			Hover: &core.Style{Background: core.RGBA(0xFFFFFF, 0.12)}, Expanded: &core.Style{Background: core.RGBA(0xFFFFFF, 0.2)}, FocusVisible: ui.FocusRing(cx, -2)},
		StyleFunc: func() core.Style {
			if width > 0 {
				return core.Style{Width: core.Pt(width)}
			}
			return core.Style{}
		},
		Trigger:    core.View{PointerEvents: core.PointerEventsNone, Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter}, Children: trigger},
		PopupStyle: menuGlass.floating().Merge(core.Style{Padding: core.All(6), Margin: core.Edges{Right: -6}}),
		Children:   rows(cx),
	}
}

// menuHeading is the small heading over a menu's rows.
func menuHeading(text func() string) core.Node {
	return core.Text{ContentFunc: text, Style: core.Style{FontSize: 11, LineHeight: 14, FontWeight: core.FontWeightBold, Color: onGlass, Opacity: core.Alpha(0.55), Padding: core.Edges{Top: 6, Right: 10, Bottom: 4, Left: 10}}}
}

// menuRow is a row of a transport menu that takes a press, lit under the pointer.
func menuRow(cx *core.Scope, id string, role core.Role, label func() string, checked func() bool, press func(), style core.Style, children ...core.Node) core.Node {
	return base.Button{TestID: id, Role: role, LabelFunc: label, Selected: checked,
		OnPress: func(core.PressEvent) { press() },
		Style: core.Style{Direction: core.Row, CornerRadius: 8, Gap: 8, AlignItems: core.AlignStart, Padding: core.Symmetric(6, 10),
			Hover: &core.Style{Background: core.RGBA(0xFFFFFF, 0.1)}, FocusVisible: ui.FocusRing(cx, -2)}.Merge(style),
		Children: children}
}

// check is a menu row's check mark, shown while checked holds.
func check(checked func() bool) core.Node {
	return core.Text{ContentFunc: func() string {
		if checked() {
			return "✓"
		}
		return ""
	}, Style: core.Style{FontSize: 13, LineHeight: 18, Width: core.Pt(14), FlexShrink: 0, Color: onGlass}}
}

// modeMenu is the transport's subtitle button, named for the mode, and the menu it opens above it: the four modes, the reference layer of a film that has one (referenceRow), the subtitle delay and 样式…, which opens the style view in the side panel.
func (s *screen) modeMenu(cx *core.Scope) core.Node {
	a := s.a
	name := live.Func(a, func() string { return ModeName(cx, s.subMode()) })
	label := func() string { return locales.FilmSubtitlesMode(cx, name()) }
	tooltip := func() string {
		return locales.FilmSubtitles(cx) + " " + cx.KeyLabel("s") + " / " + cx.KeyLabel("v")
	}
	trigger := []core.Node{
		core.Icon{SVG: iconSubtitles, Style: core.Style{Width: core.Pt(20), Height: core.Pt(15), FlexShrink: 0, Margin: core.Edges{Right: 6}, Color: onGlass}},
		core.Text{ContentFunc: name, MaxLines: 1, Style: core.Style{FontSize: 12, LineHeight: 16, Color: onGlass}},
	}
	rows := func(cx *core.Scope) []core.Node {
		out := []core.Node{menuHeading(func() string { return locales.FilmModeHeading(cx) })}
		for _, choice := range modeChoices {
			// A film in its own language has no translation to show: 关闭 and 原文 are its modes.
			if ownMode(choice.mode) != choice.mode {
				out = append(out, core.Show{When: live.Func(a, func() bool { return !s.item().SameLanguage }), Then: func(cx *core.Scope) core.Node { return s.modeRow(cx, choice.mode, choice.key) }})
				continue
			}
			out = append(out, s.modeRow(cx, choice.mode, choice.key))
		}
		out = append(out,
			s.referenceRow(cx),
			core.View{Style: core.Style{Height: core.Pt(0.5), Margin: core.Symmetric(5, 10), Background: core.RGBA(0xFFFFFF, 0.15)}},
			s.delayRow(cx),
			menuRow(cx, "film-style", core.RoleButton, func() string { return locales.FilmStyleMenuItem(cx) }, nil,
				live.Do(a, func() {
					*a.ModeMenu() = false
					a.SetStyleView(true)
				}),
				core.Style{Padding: core.Edges{Top: 7, Right: 10, Bottom: 7, Left: 32}},
				core.Text{ContentFunc: func() string { return locales.FilmStyleMenuItem(cx) }, Style: core.Style{FontSize: 13, LineHeight: 18, Color: onGlass}}),
		)
		return []core.Node{core.View{Style: core.Style{Width: core.Pt(258)}, Children: out}}
	}
	return s.transportMenu(cx, "film-mode", a.ModeMenu(), label, tooltip, 0, trigger, rows)
}

// modeRow is a radio row of the mode menu: a check while chosen, the mode's name with a line under it, and its key at the end.
func (s *screen) modeRow(cx *core.Scope, mode, key string) core.Node {
	a := s.a
	on := live.Func(a, func() bool { return s.subMode() == mode })
	name := func() string { return ModeName(cx, mode) }
	note := live.Func(a, func() string { return modeNote(cx, mode, a.Untranslated()) })
	text := core.View{Style: core.Style{FlexGrow: 1, Gap: 1}, Children: []core.Node{
		core.Text{ContentFunc: name, Style: core.Style{FontSize: 13, LineHeight: 18, Color: onGlass}},
		core.Show{When: func() bool { return note() != "" }, Then: func(cx *core.Scope) core.Node {
			return core.Text{ContentFunc: note, Style: core.Style{FontSize: 11, LineHeight: 15, Color: onGlass, Opacity: core.Alpha(0.6)}}
		}},
	}}
	keycap := core.Text{ContentFunc: func() string { return cx.KeyLabel(key) }, Style: core.Style{FontSize: 11, LineHeight: 18, FlexShrink: 0, Color: onGlass, Opacity: core.Alpha(0.5)}}
	return menuRow(cx, "film-mode-"+mode, core.RoleRadio, name, on, live.Do(a, func() {
		a.SetSubMode(mode)
		*a.ModeMenu() = false
	}), core.Style{}, check(on), text, keycap)
}

// delayRow moves the subtitles against the picture by a tenth of a second, as Z and X do.
func (s *screen) delayRow(cx *core.Scope) core.Node {
	a := s.a
	return core.View{Style: core.Style{Direction: core.Row, Padding: core.Edges{Top: 4, Right: 10, Bottom: 4, Left: 32}, Gap: 6, AlignItems: core.AlignCenter}, Children: []core.Node{
		core.Text{ContentFunc: func() string { return locales.FilmDelay(cx) }, Style: core.Style{FontSize: 13, LineHeight: 18, FlexGrow: 1, Color: onGlass}},
		delayInput(cx, a, "film-delay"),
	}}
}

// delayInput is the subtitle delay as a number a tenth of a second at a time, from -30 to 30 seconds, written as DelayText writes it. The field keeps the text Format wrote until its value changes, and reads its name as it is built, as ui.NumberInput takes a static one, so it is built again when the language changes, which happens in the settings, away from it, to show the delay in the new language.
func delayInput(cx *core.Scope, a App, id string) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		// TODO(nsgo#1861): give ui.NumberInput (and base.NumberInput) a LabelFunc twin, which NSGo's Func-field guard still lists as an exception, and write the value again when what Format reads changes, so the field's name and the delay's text follow the interface language without building the field again; then drop this Dynamic https://github.com/nsgo-dev/nsgo/issues/1861
		return ui.NumberInput{TestID: id, Label: locales.FilmDelay(cx), Min: -30, Max: 30, Step: 0.1,
			Value:  live.Func(a, func() float64 { return a.Delay().Seconds() }),
			Format: func(v float64) string { return DelayText(cx, seconds(v)) },
			Parse:  parseDelay,
			OnChange: live.On(a, func(v float64) {
				a.SetDelay(seconds(v).Round(100 * time.Millisecond))
			}),
			Style: core.Style{Width: core.Pt(120), FlexShrink: 0}}
	})
}

// parseDelay reads a delay typed into its field, as "+0.3", "-1" or "0.5 秒": the number it starts with, its unit left off.
func parseDelay(text string) (float64, bool) {
	text = strings.TrimSpace(text)
	end := strings.IndexFunc(text, func(r rune) bool { return !strings.ContainsRune("+-.0123456789", r) })
	if end >= 0 {
		text = text[:end]
	}
	v, err := strconv.ParseFloat(strings.TrimPrefix(text, "+"), 64)
	return v, err == nil
}
