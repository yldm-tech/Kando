package settings

import (
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	"kando/apps/kando/theme"
	"kando/apps/kando/toolhealth"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// ToolFetch is Kando's own download of a program: whether it can download it here, how far one under way has come (Progress, from 0 to 1, or -1 before its size is known), and why the last one failed.
type ToolFetch struct {
	Available bool
	Running   bool
	Progress  float64
	Err       string
}

// ToolHealth is the copy of a program the pipeline runs: where it is ("" when the computer has none), whether it is Kando's own, and how it answered when Kando last ran it (media.Probe); and Other, a copy on the computer besides Kando's own that works, which the row lets the user choose between ("" when there is none).
type ToolHealth struct {
	Path string
	Own  bool
	media.Health
	Other string
}

// toolState is what the end of a program's row shows.
type toolState uint8

const (
	toolReady toolState = iota
	toolChecking
	toolBroken
	toolDownloading
	toolDownloadable
	toolMissing
)

// The choices of a program's copy, when the computer has one that works besides Kando's own.
const (
	copyOwn      = "own"
	copyComputer = "computer"
)

// toolRow is one of the programs Kando runs: its tile, its name over what it is for and which copy runs (看懂自带, or where it was found), and at its end a dot with its version. A copy that does not start says so in red, why under its name, and offers Kando's own (改用看懂自带的), or downloading Kando's own again when that is the one that does not start (重新下载), or where Kando cannot download it, as on Windows, the program's page. A copy not yet run says 检查中. A program the computer lacks shows Kando's download of it: 下载 or 重试, or its progress while it runs, or 未安装 where Kando cannot download it. When the computer has a copy that works besides Kando's own, which copy runs is a choice under the name.
type toolRow struct {
	s               screen
	mark, name, use string
}

// Render implements core.Node.
func (r toolRow) Render(cx *core.Scope) core.Node {
	a := r.s.a
	colors := ui.ColorsOf(cx)
	health := func() ToolHealth { return a.ToolHealth(r.name) }
	fetch := func() ToolFetch { return a.ToolDownload(r.name) }
	where := live.Func(a, func() string {
		switch h := health(); {
		case h.Path != "" && h.Own:
			return locales.SettingsGeneralToolOwn(cx)
		case h.Path != "":
			return a.DisplayPath(h.Path)
		}
		switch f := fetch(); {
		case f.Running:
			return locales.SettingsGeneralDownloading(cx)
		case f.Err != "":
			return locales.SettingsGeneralDownloadFailed(cx, f.Err)
		}
		return locales.SettingsGeneralNotInstalled(cx)
	})
	state := core.NewMemo(cx, live.Func(a, func() toolState {
		h, f := health(), fetch()
		switch {
		case f.Running && (h.Path == "" || h.State == media.HealthBroken):
			return toolDownloading
		case h.Path != "" && h.State == media.HealthReady:
			return toolReady
		case h.Path != "" && h.State == media.HealthBroken:
			return toolBroken
		case h.Path != "":
			return toolChecking
		case f.Available:
			return toolDownloadable
		}
		return toolMissing
	}))
	page := func() core.Node {
		src, ok := media.SourceFor(r.name)
		if !ok {
			return nil
		}
		return ui.Link{Text: locales.SettingsGeneralToolPage(cx), Label: locales.SettingsGeneralToolPageLabel(cx, r.name), Href: src.Page, TestID: "settings-tool-page-" + r.name}
	}
	beside := func(status, action core.Node) core.Node {
		if action == nil {
			return status
		}
		return core.View{Style: core.Style{Direction: core.Row, Gap: 12, AlignItems: core.AlignCenter, FlexShrink: 0}, Children: []core.Node{status, action}}
	}
	end := core.Dynamic(func(cx *core.Scope) core.Node {
		switch state.Get() {
		case toolReady:
			return form.Status(ui.Fixed(form.OK), colors.Muted, live.Func(a, func() string {
				if version := health().Version; version != "" {
					return version
				}
				return locales.SettingsGeneralInstalled(cx)
			}))
		case toolChecking:
			return form.Status(colors.Muted, colors.Muted, static(locales.SettingsGeneralToolChecking(cx)))
		case toolBroken:
			status := form.Status(colors.Danger, colors.Danger, static(locales.SettingsGeneralToolBroken(cx)))
			if !fetch().Available {
				return beside(status, page())
			}
			return beside(status, form.Button{TestID: "settings-tool-fix-" + r.name, OnPress: live.Do(a, func() { a.UseOwnTool(r.name) }),
				TextFunc: live.Func(a, func() string {
					if health().Own {
						return locales.SettingsGeneralToolRedownload(cx)
					}
					return locales.SettingsGeneralToolUseOwn(cx)
				})})
		case toolDownloading:
			progress := live.Func(a, func() float64 { return fetch().Progress })
			return ui.Progress{Label: locales.SettingsGeneralDownloading(cx), TestID: "settings-tool-progress-" + r.name, Style: core.Style{Width: core.Pt(72), FlexShrink: 0},
				Value:         func() float32 { return float32(max(progress(), 0) * 100) },
				Indeterminate: func() bool { return progress() < 0 }}
		case toolDownloadable:
			return form.Button{TestID: "settings-tool-download-" + r.name, OnPress: live.Do(a, func() { a.DownloadTool(r.name) }),
				TextFunc: live.Func(a, func() string {
					if fetch().Err != "" {
						return locales.SettingsGeneralRetry(cx)
					}
					return locales.SettingsGeneralDownload(cx)
				})}
		}
		// Where Kando cannot download it, as on Windows, the program's own page of builds is where to get it.
		return beside(form.Status(colors.Danger, colors.Muted, static(locales.SettingsGeneralNotInstalled(cx))), page())
	})
	k := theme.Of(cx)
	tile := core.View{Style: core.Style{Width: core.Pt(30), Height: core.Pt(30), FlexShrink: 0, CornerRadius: 8, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}, StyleFunc: ui.Filled(k.Tile),
		Children: []core.Node{core.Text{Content: r.mark, Role: core.RoleNone, Style: core.Style{FontFamily: "monospace", FontSize: 10, FontWeight: ui.Bold}, StyleFunc: ui.Colored(k.Strong)}}}
	detailStyle := core.Style{FontSize: 11}
	// other is the computer's copy that works besides Kando's own; while there is one, which copy runs is chosen under the name, and the choice is built again only when that copy moves.
	other := core.NewMemo(cx, live.Func(a, func() string { return health().Other }))
	detail := core.Show{When: func() bool { return other.Get() == "" },
		Then: func(cx *core.Scope) core.Node {
			return core.Text{ContentFunc: func() string { return locales.SettingsGeneralToolDetail(cx, r.use, where()) }, Style: detailStyle, StyleFunc: ui.Colored(colors.Muted)}
		},
		Else: func(cx *core.Scope) core.Node {
			choice := core.Dynamic(func(cx *core.Scope) core.Node {
				options := []ui.Option[string]{
					{Value: copyOwn, Label: locales.SettingsGeneralToolOwn(cx)},
					{Value: copyComputer, Label: locales.SettingsGeneralToolComputer(cx, a.DisplayPath(other.Get()))},
				}
				// The select is as small as the line's other controls (ui.SizeSmall), its type a size under the body's, beside the 11 pt line.
				return ui.Select[string]{Options: options, Label: locales.SettingsGeneralToolCopy(cx, r.name), TestID: "settings-tool-copy-" + r.name,
					Value: live.Func(a, func() string {
						if health().Own {
							return copyOwn
						}
						return copyComputer
					}),
					OnChange: live.On(a, func(value string) { a.UseComputerTool(r.name, value == copyComputer) }),
					Style:    core.Style{Width: core.Pt(220), FlexShrink: 1, Height: core.Pt(ui.SizeSmall.Height()), FontSize: 12}}
			})
			return core.View{Style: core.Style{Direction: core.Row, Gap: 6, AlignItems: core.AlignCenter, MinWidth: core.Pt(0)}, Children: []core.Node{
				core.Text{Content: r.use, Style: detailStyle, StyleFunc: ui.Colored(colors.Muted)},
				choice,
			}}
		}}
	why := core.Show{When: live.Func(a, func() bool { h := health(); return h.Path != "" && h.State == media.HealthBroken }), Then: func(cx *core.Scope) core.Node {
		return core.Text{ContentFunc: live.Func(a, func() string { return toolhealth.Why(locales.For(cx), health().Health) }), Selectable: core.SelectableOn,
			Style: detailStyle, StyleFunc: ui.Colored(colors.Danger), TestID: "settings-tool-why-" + r.name}
	}}
	texts := core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 1}, Children: []core.Node{
		core.Text{Content: r.name, Style: core.Style{FontSize: 13, FontWeight: ui.Semibold}, StyleFunc: ui.Colored(colors.Text)},
		detail,
		why,
	}}
	return core.View{Role: core.RoleGroup, Label: r.name, TestID: "settings-tool-" + r.name,
		Style:    core.Style{Direction: core.Row, FlexShrink: 0, MinHeight: core.Pt(48), Padding: core.Symmetric(6, 14), Gap: 12, AlignItems: core.AlignCenter},
		Children: []core.Node{tile, texts, end}}
}
