package settings

import (
	"path/filepath"

	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"

	"github.com/egoist/mygo/ui"
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

// cli is the 命令行与 MCP card: the kando command, whether a terminal finds it, and 安装 when it does not; and the MCP server, with its configuration and Claude Code's command to copy.
func (s screen) cli(c *ui.Context, t *ui.Theme) {
	cli := s.CLI()
	form.Label(c, locales.SettingsGeneralCLI(), 14, nil)
	form.Card(c, func() {
		form.Row(c, locales.SettingsGeneralCLITool(), cliToolDetail(cli), func() {
			switch {
			case !cli.Known, cli.temporary():
			case cli.Installed:
				form.Status(c, form.OK, form.Secondary, locales.SettingsGeneralInstalled())
			case cli.Installable:
				label := locales.SettingsGeneralCLIInstall()
				if cli.Command != "" || cli.Link != "" {
					label = locales.SettingsGeneralCLIReinstall()
				}
				c.Key("cli-install")
				if form.Button(c, label, false).Disabled(cli.Installing).Clicked() && !cli.Installing {
					s.InstallCLI()
				}
			default:
				form.Status(c, form.Idle, form.Secondary, locales.SettingsGeneralNotInstalled())
			}
		})
		form.Separator(c, true)
		mcpHint := locales.SettingsGeneralCLIMCPHint()
		if cli.temporary() {
			mcpHint = locales.SettingsGeneralCLIMoveFirst()
		}
		form.Row(c, locales.SettingsGeneralCLIMCP(), mcpHint, func() {
			s.cliCopy(c, "cli-copy-config", locales.SettingsGeneralCLICopyConfig(), cli, cli.MCPConfig)
		})
		form.Separator(c, true)
		ui.Row(c).Shrink(0).MinHeight(46).Padding(6, 14).Gap(10).AlignItems(ui.Center).Role(ui.RoleGroup).Label(locales.SettingsGeneralCLIClaudeCode()).Children(func() {
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
				ui.Text(c, locales.SettingsGeneralCLIClaudeCode()).FontSize(13).FixedLineHeight(18).TextColor(form.Text).Wrap()
				if cli.Known && cli.ClaudeCommand != "" {
					ui.Text(c, cli.ClaudeCommand).Font("Menlo").FontSize(11).FixedLineHeight(16).TextColor(form.Secondary).Wrap().Selectable()
				}
			})
			ui.Row(c).Shrink(0).AlignItems(ui.Center).Children(func() {
				s.cliCopy(c, "cli-copy-command", locales.SettingsGeneralCLICopyCommand(), cli, cli.ClaudeCommand)
			})
		})
	})
	if cli.Err != "" {
		ui.Text(c, locales.SettingsGeneralCLIInstallFailed(cli.Err)).FontSize(11).TextColor(t.Danger).Wrap().Padding(4, 12, 0, 12)
	}
	form.Note(c, locales.SettingsGeneralCLINote(), 0)
}

// cliCopy is a button that puts text on the clipboard and reads 已拷贝 while text is what the card copied last. Until the app has looked for the command, and while Kando runs from a place that goes away, there is nothing to copy.
func (s screen) cliCopy(c *ui.Context, key, label string, cli CLI, text string) {
	can := cli.Known && text != ""
	shown := label
	if can && cli.Copied == text {
		shown = locales.SettingsGeneralCLICopied()
	}
	c.Key(key)
	if form.Button(c, shown, false).Label(label).Disabled(!can).Clicked() && can {
		c.WriteClipboard(text)
		s.CLICopied(text)
	}
}

// temporary reports whether the app has looked and found Kando running from a place that goes away, where the card offers nothing to install or copy.
func (cli CLI) temporary() bool { return cli.Known && cli.Temporary }

// cliToolDetail is the line under 命令行工具: what the command is for until the app has looked or while it is missing, to move Kando first while it runs from a place that goes away, where it is once a terminal finds it, or why a terminal does not.
func cliToolDetail(cli CLI) string {
	switch {
	case !cli.Known:
		return locales.SettingsGeneralCLIToolHint()
	case cli.temporary():
		return locales.SettingsGeneralCLIMoveFirst()
	case cli.Installed:
		return locales.SettingsGeneralCLIInstalledAt(films.DisplayPath(cli.Command))
	case cli.Link != "":
		return locales.SettingsGeneralCLINotOnPath(films.DisplayPath(cli.Link), films.DisplayPath(filepath.Dir(cli.Link)))
	case cli.Command != "":
		return locales.SettingsGeneralCLIOther(films.DisplayPath(cli.Command))
	case !cli.Installable && cli.Exe != "":
		return locales.SettingsGeneralCLIAddToPath(films.DisplayPath(filepath.Dir(cli.Exe)))
	}
	return locales.SettingsGeneralCLIToolHint()
}
