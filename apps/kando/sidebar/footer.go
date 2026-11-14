package sidebar

import (
	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// ReadyState is how far the app is from starting a job.
type ReadyState int

const (
	// Checking is a check still running, as the EveryAPI sign-in, which may take seconds.
	Checking ReadyState = iota
	// Ready means a job can start.
	Ready
	// Missing means something a job needs is missing; Readiness.Message names it.
	Missing
)

// Readiness is the status line at the foot of the sidebar: a dot, a message and the settings section that fixes what is missing.
type Readiness struct {
	State ReadyState
	// Message says the message in the language the app shows under cx, read in the sidebar's binding, so it follows the language as it changes; nil says nothing. A generated accessor such as locales.ReadinessReady is one.
	Message func(cx *core.Scope) string
	// Section is the settings section the gear opens, one of the settings package's Section values; "" opens 通用.
	Section string
}

// Text is r's message in the language the app shows under cx, "" without one.
func (r Readiness) Text(cx *core.Scope) string {
	if r.Message == nil {
		return ""
	}
	return r.Message(cx)
}

var (
	footerFill     = core.RGBA(0xFFFFFF, 0.55)
	footerHairline = core.RGBA(0x000000, 0.06)
	footerText     = core.RGB(0x3A3A3F)
	readyDot       = core.RGB(0x30D158)
	missingDot     = core.RGB(0xFF9F0A)
)

// footer is the status card pinned to the bottom of the sidebar: an 8-point dot, green when a job can start and orange when something is missing, the message saying which, and a gear that opens the settings where it is fixed.
type footer struct{ a App }

// Render implements core.Node.
func (f footer) Render(cx *core.Scope) core.Node {
	a := f.a
	colors := ui.ColorsOf(cx)
	state := live.Memo(cx, a, func() ReadyState { return a.Readiness().State })
	dot := core.View{TestID: "sidebar-status-dot",
		Style: core.Style{Width: core.Pt(8), Height: core.Pt(8), FlexShrink: 0, CornerRadius: 4},
		StyleFunc: func() core.Style {
			switch state() {
			case Ready:
				return core.Style{Background: readyDot}
			case Missing:
				return core.Style{Background: missingDot}
			}
			return core.Style{Background: colors.Muted()}
		}}
	// The message is the prototype's 11 points on 15, a point above the platform's caption.
	message := core.Text{TestID: "sidebar-status", ContentFunc: live.Func(a, func() string { return a.Readiness().Text(cx) }),
		Style: core.Style{FontSize: 11, LineHeight: 15, Color: footerText, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}}
	// The gear's button is the prototype's 26-point square, the gear in the message's color.
	gear := ui.IconButton{Icon: svg(iconGear), LabelFunc: func() string { return locales.SidebarSettings(cx) }, Size: 26, Tint: ui.Fixed(footerText), TestID: "sidebar-settings",
		OnPress: live.Do(a, func() { a.OpenSettings(a.Readiness().Section) })}
	// The hairline is a ring outside the edge that takes no room, as the prototype's box-shadow.
	// A group named 状态, as MyGo's labeled row was: assistive technology reads the name before the dot's and the message's.
	return core.View{
		Role:      core.RoleGroup,
		LabelFunc: func() string { return locales.SidebarStatus(cx) },
		Style: core.Style{Direction: core.Row, FlexShrink: 0, Margin: core.Edges{Top: 8, Right: insetEnd, Bottom: 8, Left: insetStart}, Padding: core.Symmetric(10, 12), Gap: 10,
			AlignItems: core.AlignCenter, CornerRadius: 11, Background: footerFill, Shadows: []core.BoxShadow{{Spread: 0.5, Color: footerHairline}}},
		Children: []core.Node{dot, message, gear},
	}
}
