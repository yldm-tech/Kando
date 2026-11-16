package settings

import (
	"path/filepath"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// CLI is where Kando's command line stands, as the 命令行与 MCP card of 通用 shows it. The app looks it up when the page opens, never while the page is drawn.
type CLI struct {
	// Known reports whether the app has looked yet; until it has, the card shows no state and copies nothing.
	Known bool
	// Installed reports whether the kando a terminal finds runs this app. Command is that kando, "" when a terminal finds none.
	Installed bool
	Command   string
	// Link is a kando that runs this app in a folder a terminal does not search, as a ~/.local/bin not on PATH.
	Link string
	// Exe is this app's executable.
	Exe string
	// Installable reports whether 安装 can put the command somewhere on this system; Installing is one under way, Err why the last one failed.
	Installable bool
	Installing  bool
	Err         string
	// Temporary reports whether Kando runs from a place that goes away, its disk image or a temporary copy macOS made, where any path would soon stop working: the card then asks to move Kando to Applications first, and has nothing to install or copy.
	Temporary bool
	// MCPConfig is the JSON an MCP client's configuration takes, ClaudeCommand the command that adds the server to Claude Code; Copied is the text the card last put on the clipboard. Both are "" while Temporary.
	MCPConfig     string
	ClaudeCommand string
	Copied        string
}

// cliState is what the end of the 命令行工具 row shows.
type cliState uint8

const (
	cliNothing cliState = iota
	cliInstalled
	cliInstallable
	cliMissing
)

// cli is the 命令行与 MCP card: the kando command, whether a terminal finds it, and 安装 when it does not; and the MCP server, with its configuration and Claude Code's command to copy.
type cli struct{ s screen }

// Render implements core.Node.
func (c cli) Render(cx *core.Scope) core.Node {
	a := c.s.a
	colors := ui.ColorsOf(cx)
	state := core.NewMemo(cx, live.Func(a, func() cliState {
		switch st := a.CLI(); {
		case !st.Known, st.temporary():
			return cliNothing
		case st.Installed:
			return cliInstalled
		case st.Installable:
			return cliInstallable
		}
		return cliMissing
	}))
	end := core.Dynamic(func(cx *core.Scope) core.Node {
		switch state.Get() {
		case cliInstalled:
			return form.Status(ui.Fixed(form.OK), colors.Muted, static(locales.SettingsGeneralInstalled(cx)))
		case cliInstallable:
			return form.Button{TestID: "settings-cli-install", OnPress: live.Do(a, func() {
				if !a.CLI().Installing {
					a.InstallCLI()
				}
			}),
				Disabled: live.Func(a, func() bool { return a.CLI().Installing }),
				TextFunc: live.Func(a, func() string {
					if st := a.CLI(); st.Command != "" || st.Link != "" {
						return locales.SettingsGeneralCLIReinstall(cx)
					}
					return locales.SettingsGeneralCLIInstall(cx)
				})}
		case cliMissing:
			return form.Status(theme.Of(cx).Idle, colors.Muted, static(locales.SettingsGeneralNotInstalled(cx)))
		}
		return core.Group(nil)
	})
	tool := form.Row{Title: locales.SettingsGeneralCLITool(cx), SubtitleFunc: live.Func(a, func() string { return cliToolDetail(cx, a, a.CLI()) }), Trailing: []core.Node{end}}
	mcp := form.Row{Title: locales.SettingsGeneralCLIMCP(cx),
		SubtitleFunc: live.Func(a, func() string {
			if a.CLI().temporary() {
				return locales.SettingsGeneralCLIMoveFirst(cx)
			}
			return locales.SettingsGeneralCLIMCPHint(cx)
		}),
		Trailing: []core.Node{cliCopy(cx, a, "settings-cli-copy-config", locales.SettingsGeneralCLICopyConfig(cx), func(st CLI) string { return st.MCPConfig })}}
	command := core.Show{When: live.Func(a, func() bool { st := a.CLI(); return st.Known && st.ClaudeCommand != "" }), Then: func(*core.Scope) core.Node {
		return core.Text{ContentFunc: live.Func(a, func() string { return a.CLI().ClaudeCommand }), Selectable: core.SelectableOn,
			Style: core.Style{FontFamily: "Menlo", FontSize: 11, LineHeight: 16}, StyleFunc: ui.Colored(colors.Muted)}
	}}
	claude := core.View{Role: core.RoleGroup, Label: locales.SettingsGeneralCLIClaudeCode(cx),
		Style: core.Style{Direction: core.Row, FlexShrink: 0, MinHeight: core.Pt(46), Padding: core.Symmetric(6, 14), Gap: 10, AlignItems: core.AlignCenter},
		Children: []core.Node{
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: []core.Node{
				core.Text{Content: locales.SettingsGeneralCLIClaudeCode(cx), Style: core.Style{FontSize: 13, LineHeight: 18}, StyleFunc: ui.Colored(colors.Text)},
				command,
			}},
			core.View{Style: core.Style{Direction: core.Row, FlexShrink: 0, AlignItems: core.AlignCenter}, Children: []core.Node{
				cliCopy(cx, a, "settings-cli-copy-command", locales.SettingsGeneralCLICopyCommand(cx), func(st CLI) string { return st.ClaudeCommand }),
			}},
		}}
	failed := core.Show{When: live.Func(a, func() bool { return a.CLI().Err != "" }), Then: func(cx *core.Scope) core.Node {
		return core.Text{ContentFunc: live.Func(a, func() string { return locales.SettingsGeneralCLIInstallFailed(cx, a.CLI().Err) }),
			Style: core.Style{FontSize: 11, Padding: core.Edges{Top: 4, Right: 12, Left: 12}}, StyleFunc: ui.Colored(colors.Danger)}
	}}
	return core.Group([]core.Node{
		form.Label(locales.SettingsGeneralCLI(cx), 14),
		form.Card(tool, form.Separator(true), mcp, form.Separator(true), claude),
		failed,
		form.Note(static(locales.SettingsGeneralCLINote(cx)), 0),
	})
}

// cliCopy is a button that puts the text pick takes from the card's state on the clipboard and reads 已拷贝 while that text is what the card copied last. Until the app has looked for the command, and while Kando runs from a place that goes away, there is nothing to copy.
func cliCopy(cx *core.Scope, a App, id, label string, pick func(CLI) string) core.Node {
	can := func(st CLI) bool { return st.Known && pick(st) != "" }
	return form.Button{TestID: id, Label: label,
		Disabled: live.Func(a, func() bool { return !can(a.CLI()) }),
		TextFunc: live.Func(a, func() string {
			if st := a.CLI(); can(st) && st.Copied == pick(st) {
				return locales.SettingsGeneralCLICopied(cx)
			}
			return label
		}),
		OnPress: live.Do(a, func() {
			if st := a.CLI(); can(st) {
				cx.SetClipboard(pick(st))
				a.CLICopied(pick(st))
			}
		})}
}

// temporary reports whether the app has looked and found Kando running from a place that goes away, where the card offers nothing to install or copy.
func (cli CLI) temporary() bool { return cli.Known && cli.Temporary }

// cliToolDetail is the line under 命令行工具, in the language the app shows under cx: what the command is for until the app has looked or while it is missing, to move Kando first while it runs from a place that goes away, where it is once a terminal finds it, or why a terminal does not.
func cliToolDetail(cx *core.Scope, a App, cli CLI) string {
	switch {
	case !cli.Known:
		return locales.SettingsGeneralCLIToolHint(cx)
	case cli.temporary():
		return locales.SettingsGeneralCLIMoveFirst(cx)
	case cli.Installed:
		return locales.SettingsGeneralCLIInstalledAt(cx, a.DisplayPath(cli.Command))
	case cli.Link != "":
		return locales.SettingsGeneralCLINotOnPath(cx, a.DisplayPath(cli.Link), a.DisplayPath(filepath.Dir(cli.Link)))
	case cli.Command != "":
		return locales.SettingsGeneralCLIOther(cx, a.DisplayPath(cli.Command))
	case !cli.Installable && cli.Exe != "":
		return locales.SettingsGeneralCLIAddToPath(cx, a.DisplayPath(filepath.Dir(cli.Exe)))
	}
	return locales.SettingsGeneralCLIToolHint(cx)
}
