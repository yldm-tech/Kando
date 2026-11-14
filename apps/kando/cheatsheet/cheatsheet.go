// Package cheatsheet draws the list of keyboard shortcuts that ⌘/ opens over the window.
package cheatsheet

import (
	"slices"
	"strconv"
	"strings"

	"kando/apps/kando/locales"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Group is a heading of the cheat sheet and the lines under it.
type Group struct {
	Title string
	Keys  []Key
}

// Key is one line of the sheet: what it does and the shortcuts that run it, on one keycap, as 后退 / 前进 5 秒 shows ← →. Each combo is a key in NSGo's form, as core.Scope.OnKey takes it: "space", "left", "mod+shift+t", "shift+,".
type Key struct {
	Label  string
	Combos []string
}

// The sheet's measures, as the prototype's dialog has them: 26 below the window's top, at most 860 wide and 32 narrower than the window, padded 20 22 18, the header 14 above the groups, the groups in columns at least 240 wide, 22 apart, rows of groups 18 apart, and lines 22 high, 6 apart. The close button is a 28 circle.
const (
	sheetTop    = 26
	sheetWidth  = 860
	sheetInset  = 32
	sheetPadTop = 20
	sheetPadX   = 22
	sheetPadBot = 18
	sheetRadius = 24
	headerGap   = 14
	columnWidth = 240
	columnGap   = 22
	groupGap    = 18
	lineGap     = 6
	lineHeight  = 22
	closeSize   = 28
)

var (
	// scrim dims the window under the sheet.
	scrim = core.RGBA(0x000000, 0.25)
	// glassTint is the dark glass's color over what the blur shows through.
	glassTint = core.RGBA(0x1C1C20, 0.86)
	// rim is the glass's hairline edge, and shadow the shadow it casts.
	rim    = core.RGBA(0xFFFFFF, 0.2)
	shadow = core.RGBA(0x000000, 0.5)
	// ink is the sheet's text, and closeFill the close button's round plate.
	ink       = core.RGB(0xFFFFFF)
	closeFill = core.RGBA(0xFFFFFF, 0.12)
	// keycapFill is a keycap's plate on the dark glass and keycapShade the line inside its bottom, as MyGo's breeze drew a macOS keycap in the dark theme.
	keycapFill  = core.RGBA(0xFFFFFF, 0.12)
	keycapShade = core.RGBA(0x000000, 0.3)
)

// Sheet shows the cheat sheet over the window in the dark theme while Open returns true: the window dimmed, and near its top dark glass with the groups in as many columns as fit. Escape, the close button or a click outside it run OnClose; the app's own ⌘/ closes it as it opened it. Groups is read in a binding, so it may follow the app's whole state (live.Func): the groups are laid out again only when they change.
type Sheet struct {
	Open    func() bool
	Groups  func() []Group
	OnClose func()
}

// Render implements core.Node.
func (s Sheet) Render(cx *core.Scope) core.Node {
	theme.Dark(cx)
	closeSheet := func() {
		if s.OnClose != nil {
			s.OnClose()
		}
	}
	// TODO(nsgo): give base.Dialog a LabelFunc so the sheet's accessible name follows the interface language as it changes; it is read once as the sheet is built
	return base.Dialog{
		Open: s.Open,
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				closeSheet()
			}
		},
		Label:         locales.CheatsheetTitle(cx),
		TestID:        "cheatsheet",
		BackdropStyle: core.Style{Background: scrim},
		LayerStyle:    core.Style{JustifyContent: core.JustifyStart, Padding: core.Edges{Top: sheetTop}},
		PopupStyleFunc: func() core.Style {
			window := cx.Env().Window()
			return core.Style{Width: core.Pt(sheetFits(window.Width)), MaxHeight: core.Pt(max(0, window.Height-2*sheetTop)), CornerRadius: sheetRadius,
				BackdropFilters: []core.Filter{core.Blur(36)}, Shadows: []core.BoxShadow{{Offset: core.Point{Y: 30}, Blur: 80, Color: shadow}}}
		},
		Children: []core.Node{panel{groups: s.Groups, close: closeSheet}},
	}
}

// sheetFits is the sheet's width in a window width wide.
func sheetFits(width core.Points) core.Points { return min(sheetWidth, width-sheetInset) }

// columns is how many columns of groups fit in a sheet width wide.
func columns(width core.Points) int {
	return max(1, int((width-2*sheetPadX+columnGap)/(columnWidth+columnGap)))
}

// panel is the glass's content, built while the sheet is open: the header and the groups. Escape closes the sheet wherever the keyboard focus is, before the window's own Escape, which would close the open film behind it.
type panel struct {
	groups func() []Group
	close  func()
}

// Render implements core.Node.
func (p panel) Render(cx *core.Scope) core.Node {
	// The groups are read while the sheet is open only, and laid out again only when they change.
	groups := core.NewMemoFunc(cx, p.groups, equalGroups)
	cx.OnKeyWith("escape", core.KeyOptions{When: core.KeyAlways}, p.close)
	// The header: the title 17 bold, the hint 12 at 60%, the close button.
	header := core.View{Style: core.Style{Direction: core.Row, Gap: 10, AlignItems: core.AlignCenter, Margin: core.Edges{Bottom: headerGap}}, Children: []core.Node{
		core.Text{ContentFunc: func() string { return locales.CheatsheetTitle(cx) }, Role: core.RoleHeader,
			Style: core.Style{FontSize: 17, FontWeight: ui.Bold, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}},
		core.Text{ContentFunc: func() string { return locales.CheatsheetHint(cx) }, Style: core.Style{FontSize: 12, Opacity: core.Alpha(0.6), FlexShrink: 1}},
		base.Button{TestID: "cheatsheet-close", LabelFunc: func() string { return locales.CheatsheetClose(cx) }, OnPress: func(core.PressEvent) { p.close() },
			Style: core.Style{Width: core.Pt(closeSize), Height: core.Pt(closeSize), FlexShrink: 0, CornerRadius: closeSize / 2, Background: closeFill,
				AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Active: &core.Style{Opacity: core.Alpha(0.7)}},
			Children: []core.Node{ui.Icon(svg(iconClose), 16, func() core.Color { return ink })}},
	}}
	grid := core.View{TestID: "cheatsheet-groups",
		StyleFunc: func() core.Style {
			n := columns(sheetFits(cx.Env().Window().Width))
			return core.Style{Display: core.DisplayGrid, GridTemplateColumns: core.GridTracks("repeat(" + strconv.Itoa(n) + ", minmax(0, 1fr))"),
				ColumnGap: columnGap, RowGap: groupGap, AlignItems: core.AlignStart}
		},
		// The groups hold no control, so building them again when they change loses nothing.
		Children: []core.Node{core.Dynamic(func(cx *core.Scope) core.Node {
			var nodes core.Group
			for _, g := range groups.Get() {
				nodes = append(nodes, group(cx, g))
			}
			return nodes
		})}}
	return core.View{
		Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0), Overflow: core.Hidden, CornerRadius: sheetRadius, Background: glassTint,
			Shadows: []core.BoxShadow{{Spread: 0.5, Color: rim, Inset: true}}, Color: ink,
			Padding: core.Edges{Top: sheetPadTop, Right: sheetPadX, Bottom: sheetPadBot, Left: sheetPadX}},
		Children: []core.Node{header, core.Scroll{Style: core.Style{FlexShrink: 1, MinHeight: core.Pt(0)}, Children: []core.Node{grid}}},
	}
}

// group draws a group's heading, 11 bold at 55% on a line 16 high (the browser's normal line of PingFang), and a line per entry: what it does at 90%, then its keycap, its text 600.
func group(cx *core.Scope, g Group) core.Node {
	nodes := []core.Node{core.Text{Content: g.Title, Role: core.RoleHeader, Style: core.Style{FontSize: 11, LineHeight: 16, FontWeight: ui.Bold, Opacity: core.Alpha(0.55)}}}
	for _, k := range g.Keys {
		nodes = append(nodes, core.View{Style: core.Style{Direction: core.Row, Gap: 10, AlignItems: core.AlignCenter, MinHeight: core.Pt(lineHeight)}, Children: []core.Node{
			core.Text{Content: k.Label, Style: core.Style{FontSize: 13, Opacity: core.Alpha(0.9), FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}},
			keycap(cx, k.Combos),
		}})
	}
	return core.View{Style: core.Style{Gap: lineGap, MinWidth: core.Pt(0)}, Children: nodes}
}

// keycap draws combos as the platform shows a shortcut. On macOS several share one keycap, as the prototype draws ⌘← ⌘→; elsewhere they stand side by side, each the platform's own. Escape alone reads Esc on every platform, as the prototype writes it: macOS's menu glyph ⎋ on a keycap reads like an undo arrow.
func keycap(cx *core.Scope, combos []string) core.Node {
	texts := make([]string, len(combos))
	for i, k := range combos {
		texts[i] = comboText(cx, k)
	}
	label := strings.Join(texts, " ")
	if len(combos) == 1 && isEscape(combos[0]) {
		label = "Escape"
	}
	var caps []core.Node
	if core.Untracked(cx, func() bool { return cx.Env().OS() == core.OSMacOS }) {
		caps = []core.Node{plate(strings.Join(texts, " "))}
	} else {
		for _, text := range texts {
			caps = append(caps, plate(text))
		}
	}
	return core.View{Role: core.RoleText, Label: label, Style: core.Style{Direction: core.Row, Gap: 4, AlignItems: core.AlignCenter, FlexShrink: 0}, Children: caps}
}

// plate is one keycap, as MyGo's breeze drew a macOS keycap in the dark theme: 22 high and at least as wide, padded 8 either side, corners of 6, the text 12 at 600, a line of shade inside its bottom.
func plate(text string) core.Node {
	return core.View{
		Style: core.Style{MinHeight: core.Pt(22), MinWidth: core.Pt(22), Padding: core.Symmetric(0, 8), CornerRadius: 6, Background: keycapFill,
			Shadows: []core.BoxShadow{{Offset: core.Point{Y: -1}, Color: keycapShade, Inset: true}}, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, FlexShrink: 0},
		Children: []core.Node{core.Text{Content: text, Role: core.RoleNone, MaxLines: 1, Style: core.Style{FontSize: 12, FontWeight: core.FontWeightSemibold}}},
	}
}

func isEscape(key string) bool { return key == "escape" }

// comboText is key as the platform's menus write it, and Esc for Escape alone.
func comboText(cx *core.Scope, key string) string {
	if isEscape(key) {
		return "Esc"
	}
	return cx.KeyLabel(key)
}

// equalGroups reports whether a and b list the same lines under the same headings.
func equalGroups(a, b []Group) bool {
	return slices.EqualFunc(a, b, func(x, y Group) bool {
		return x.Title == y.Title && slices.EqualFunc(x.Keys, y.Keys, func(p, q Key) bool { return p.Label == q.Label && slices.Equal(p.Combos, q.Combos) })
	})
}
