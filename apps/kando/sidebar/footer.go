package sidebar

import (
	"strings"

	"kando/apps/kando/hotkey"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
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

// The status dots, the same in both appearances. The card is the theme's card color, translucent, with its shade's hairline, and the message is in its strong ink (theme.Of).
var (
	readyDot   = core.RGB(0x30D158)
	missingDot = core.RGB(0xFF9F0A)
	failedDot  = core.RGB(0xFF453A)
)

// footer is the status card pinned to the bottom of the sidebar: an 8-point dot, green when a job can start and orange when something is missing, the message saying which, a gear that opens the settings where it is fixed, and before the gear, where the app has no menu bar (PaletteButton), a button that opens the command palette, its key in its tooltip.
type footer struct{ a App }

// Render implements core.Node.
func (f footer) Render(cx *core.Scope) core.Node {
	a := f.a
	colors, k := ui.ColorsOf(cx), theme.Of(cx)
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
		Style: core.Style{FontSize: 11, LineHeight: 15, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, StyleFunc: ui.Colored(k.Strong)}
	// The gear's button is the prototype's 26-point square, the gear in the message's color.
	gear := ui.IconButton{Icon: svg(iconGear), LabelFunc: func() string { return locales.SidebarSettings(cx) }, Size: 26, Tint: k.Strong, TestID: "sidebar-settings",
		OnPress: live.Do(a, func() { a.OpenSettings(a.Readiness().Section) })}
	buttons := []core.Node{dot, message}
	if a.PaletteButton() {
		// The palette's button is the gear's twin, so the two read as the card's pair of actions.
		palette := ui.IconButton{Icon: lucide.Command, LabelFunc: func() string { return locales.SidebarCommands(cx) }, Size: 26, Tint: k.Strong, TestID: "sidebar-palette",
			OnPress: live.Do(a, a.TogglePalette)}
		buttons = append(buttons, ui.Tooltip{TextFunc: func() string { return locales.SidebarCommandsTooltip(cx, hotkey.Label(cx, a.PaletteKey())) }, Trigger: palette,
			Style: core.Style{FlexShrink: 0}})
	}
	buttons = append(buttons, gear)
	// The status sits on the sidebar itself, the last of its rows, with no card of its own.
	// A group named 状态, as MyGo's labeled row was: assistive technology reads the name before the dot's and the message's.
	return core.View{
		Role:      core.RoleGroup,
		LabelFunc: func() string { return locales.SidebarStatus(cx) },
		Style:     core.Style{FlexShrink: 0, Margin: core.Edges{Top: 8, Right: insetEnd, Bottom: 8, Left: insetStart}, Padding: core.Symmetric(10, 12), Gap: 8},
		Children: []core.Node{
			jobsLine{a},
			core.View{Style: core.Style{Direction: core.Row, Gap: 10, AlignItems: core.AlignCenter}, Children: buttons},
		},
	}
}

// jobsLine is the status line's count of the queue's jobs while it holds any: a dot, in the accent while they run and red once one has failed, and how many run and how many failed. It opens 处理中.
type jobsLine struct{ a App }

// Render implements core.Node.
func (j jobsLine) Render(cx *core.Scope) core.Node {
	a := j.a
	jobs := live.Memo(cx, a, a.Jobs)
	text := func() string {
		n := jobs()
		var parts []string
		if n.Active > 0 {
			parts = append(parts, locales.SidebarJobsActive(cx, n.Active))
		}
		if n.Failed > 0 {
			parts = append(parts, locales.SidebarJobsFailed(cx, n.Failed))
		}
		return strings.Join(parts, " · ")
	}
	colors := ui.ColorsOf(cx)
	return core.Show{When: func() bool { n := jobs(); return n.Active > 0 || n.Failed > 0 }, Then: func(cx *core.Scope) core.Node {
		dot := core.View{Style: core.Style{Width: core.Pt(8), Height: core.Pt(8), FlexShrink: 0, CornerRadius: 4},
			StyleFunc: func() core.Style {
				if jobs().Failed > 0 {
					return core.Style{Background: failedDot}
				}
				return core.Style{Background: colors.Accent()}
			}}
		// It is shaded under the pointer and while pressed, as the switcher is; its padding, taken back by its margins, gives the shade room around the dot and the count without moving them.
		k := theme.Of(cx)
		return base.Button{TestID: "sidebar-jobs", LabelFunc: text, OnPress: live.On(a, func(core.PressEvent) { a.ShowJobs() }),
			Style: core.Style{Direction: core.Row, Gap: 10, AlignItems: core.AlignCenter, Cursor: core.CursorDefault, CornerRadius: 6, Padding: core.Symmetric(3, 6), Margin: core.Symmetric(-3, -6),
				FocusVisible: ui.FocusRing(cx, 2)},
			StyleFunc: func() core.Style {
				return core.Style{Hover: &core.Style{Background: k.Shade(0.05)}, Active: &core.Style{Background: k.Shade(0.07)}}
			},
			Children: []core.Node{dot, core.Text{ContentFunc: text, MaxLines: 1, EllipsizeMode: core.EllipsizeTail,
				Style: core.Style{FontSize: 11, LineHeight: 15, FlexShrink: 1, MinWidth: core.Pt(0)}, StyleFunc: ui.Colored(k.Strong)}}}
	}}
}
