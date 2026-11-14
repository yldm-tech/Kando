package film

import (
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// glassSpec is a glass surface of the player's: what is under it blurred by blur, tinted with fill, inside a half-point white ring of alpha ring, its corners radius.
type glassSpec struct {
	fill         core.Color
	blur, radius core.Points
	ring         float32
}

// style is the glass as a view's style.
func (g glassSpec) style() core.Style {
	return core.Style{
		CornerRadius:    g.radius,
		Background:      g.fill,
		BackdropFilters: []core.Filter{core.Blur(g.blur)},
		Shadows:         []core.BoxShadow{{Spread: 0.5, Color: core.RGBA(0xFFFFFF, g.ring), Inset: true}},
	}
}

// capsuleGlass is the glass of the title bar's small buttons and capsules, 34 high.
var capsuleGlass = glassSpec{fill: core.RGBA(0xFFFFFF, 0.12), blur: 20, ring: 0.25, radius: 17}

// tip is the bubble a control shows under the pointer: its label, and the key that does the same when there is one.
func tip(cx *core.Scope, text func() string) core.Node {
	return core.View{PointerEvents: core.PointerEventsNone,
		Style: core.Style{CornerRadius: 6, Padding: core.Symmetric(4, 8), MaxWidth: core.Pt(280), Background: core.RGBA(0x1C1C20, 0.92),
			Shadows: []core.BoxShadow{{Spread: 0.5, Color: core.RGBA(0xFFFFFF, 0.2), Inset: true}}},
		Children: []core.Node{core.Text{ContentFunc: text, Style: core.Style{FontSize: 12, LineHeight: 16, Color: onGlass}}}}
}

// withKey is label followed by key as the platform writes it, for a tooltip.
func withKey(cx *core.Scope, label func() string, key string) func() string {
	return func() string { return label() + " " + cx.KeyLabel(key) }
}

// glyphButton is a button over the picture showing icon at iw×ih in a w×h box, in white, named label: its face shows only under the pointer, as a flat button's, and it holds the prototype's glyph at the prototype's size. tooltip is what its bubble says, label when nil.
func glyphButton(cx *core.Scope, icon func() *core.SVG, label, tooltip func() string, w, h, iw, ih core.Points, id string, press func()) base.Button {
	if tooltip == nil {
		tooltip = label
	}
	return base.Button{TestID: id, LabelFunc: label, Tooltip: tip(cx, tooltip),
		Desktop: core.DesktopViewProps{WindowDrag: core.WindowDragOff},
		OnPress: func(core.PressEvent) { press() },
		Style: core.Style{Width: core.Pt(w), Height: core.Pt(h), FlexShrink: 0, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, CornerRadius: min(h/2, 10),
			Hover: &core.Style{Background: core.RGBA(0xFFFFFF, 0.12)}, Active: &core.Style{Background: core.RGBA(0xFFFFFF, 0.2)}, Disabled: ui.Dimmed(), FocusVisible: ui.FocusRing(cx, -2)},
		Children: []core.Node{core.Icon{SVGFunc: icon, Style: core.Style{Width: core.Pt(iw), Height: core.Pt(ih), Color: onGlass}}}}
}

// glyphToggle is glyphButton lit while on.
func glyphToggle(cx *core.Scope, icon func() *core.SVG, label, tooltip func() string, w, h, iw, ih core.Points, id string, on func() bool, press func()) base.Button {
	b := glyphButton(cx, icon, label, tooltip, w, h, iw, ih, id, press)
	b.Selected = on
	b.StyleFunc = func() core.Style {
		if on() {
			return core.Style{Background: core.RGBA(0xFFFFFF, 0.22)}
		}
		return core.Style{}
	}
	return b
}

// glassButton is a round button on the player's glass, holding child.
func glassButton(cx *core.Scope, g glassSpec, size core.Style, label func() string, id string, press func(), child core.Node) base.Button {
	return base.Button{TestID: id, LabelFunc: label, Tooltip: tip(cx, label),
		Desktop:  core.DesktopViewProps{WindowDrag: core.WindowDragOff},
		OnPress:  func(core.PressEvent) { press() },
		Style:    g.style().Merge(size).Merge(core.Style{AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, FlexShrink: 0, FocusVisible: ui.FocusRing(cx, -2)}),
		Children: []core.Node{child}}
}

// svg wraps an icon as the getter NSGo's controls take.
func svg(icon *core.SVG) func() *core.SVG { return func() *core.SVG { return icon } }
