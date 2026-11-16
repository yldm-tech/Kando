package settings

import (
	"path/filepath"
	"strings"

	"kando/apps/kando/desktop"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	"kando/apps/kando/theme"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// general is the 通用 page: the library, the interface language, the notifications of finished videos, the version and its updates, the programs Kando runs, its command line and MCP server (cli.go), and its browser extension (browser.go).
type general struct{ s screen }

// Render implements core.Node.
func (g general) Render(cx *core.Scope) core.Node {
	a := g.s.a
	language := form.Row{Title: locales.SettingsGeneralInterfaceLanguage(cx), NoGroup: true, Trailing: []core.Node{interfaceLanguageSelect(cx, a)}}
	notify := form.SwitchRow{Title: locales.SettingsGeneralNotify(cx), Subtitle: locales.SettingsGeneralNotifyHint(cx), TestID: "settings-job-notifications",
		On: live.Func(a, a.JobNotifications),
		OnChange: live.On(a, func(on bool) {
			a.SetJobNotifications(on)
			a.Save()
		})}
	redetect := core.Show{When: live.Func(a, func() bool { _, known := a.Tools(); return known }), Then: func(cx *core.Scope) core.Node {
		return form.Button{Text: locales.SettingsGeneralRedetect(cx), OnPress: live.Do(a, a.Redetect), TestID: "settings-redetect"}
	}}
	note := static(locales.SettingsGeneralToolsNote(cx))
	// Where Kando downloads no programs, as on Windows, they are the user's to install: from each one's page, which its row links to, or on Windows all at once with Scoop, whose folder 重新检测 looks in.
	if _, ok := media.KitFor("ffmpeg"); !ok {
		note = func() string {
			if command := scoopCommand(); command != "" {
				return locales.SettingsGeneralToolsNoteScoop(cx, command)
			}
			return locales.SettingsGeneralToolsNoteManual(cx)
		}
	}
	tool := func(mark, name, use string, path func(media.Tools) string) core.Node {
		return toolRow{s: g.s, mark: mark, name: name, use: use, path: path}
	}
	return form.Page(
		form.Label(locales.SettingsGeneralLibrary(cx), 6),
		form.Card(libraryRows{g.s}),
		form.Note(static(locales.SettingsGeneralLibraryNote(cx)), 10),
		form.Card(language, form.Separator(true), notify, form.Separator(true), updateRows{g.s}),
		form.Label(locales.SettingsGeneralComponents(cx), 14, redetect),
		form.Card(
			tool("FF", "ffmpeg", locales.SettingsGeneralUseFFmpeg(cx), func(t media.Tools) string { return t.FFmpeg }),
			form.Separator(false),
			tool("YT", "yt-dlp", locales.SettingsGeneralUseYTDLP(cx), func(t media.Tools) string { return t.YTDLP }),
			form.Separator(false),
			tool("W", "whisper-cli", locales.SettingsGeneralUseWhisper(cx), func(t media.Tools) string { return t.Whisper }),
		),
		form.Note(note, 0),
		cli{g.s},
		browserExt{g.s},
	)
}

// scoopCommand is the command that installs every program Kando runs with Scoop, "" where Scoop names none of them, off Windows.
func scoopCommand() string {
	var names []string
	for _, program := range []string{"yt-dlp", "ffmpeg", "whisper-cli"} {
		if src, ok := media.SourceFor(program); ok && src.Scoop != "" {
			names = append(names, src.Scoop)
		}
	}
	if len(names) == 0 {
		return ""
	}
	return "scoop install " + strings.Join(names, " ")
}

// static is text as a binding that does not change.
func static(text string) func() string { return func() string { return text } }

// interfaceLanguageSelect chooses the interface's language among those Kando ships a catalog for (locales.Interface), each named in itself so the choice stays understandable after the app has switched languages. The choice is saved at once.
func interfaceLanguageSelect(cx *core.Scope, a App) core.Node {
	options := make([]ui.Option[string], len(locales.Interface))
	for i, tag := range locales.Interface {
		options[i] = ui.Option[string]{Value: tag, Label: InterfaceLanguageName(tag)}
	}
	return ui.Select[string]{Options: options, Label: locales.SettingsGeneralInterfaceLanguage(cx), TestID: "settings-interface-language",
		Value: live.Func(a, func() string {
			if tag := a.InterfaceLanguage(); locales.Supported(tag) {
				return tag
			}
			return locales.Default
		}),
		OnChange: live.On(a, func(tag string) {
			a.SetInterfaceLanguage(tag)
			a.Save()
		}),
		Style: core.Style{Width: core.Pt(160)}}
}

// InterfaceLanguageName turns a language tag of locales.Interface into the label shown in the settings pop-up, its name in itself, as 繁體中文 of zh-Hant.
func InterfaceLanguageName(tag string) string {
	if !locales.Supported(tag) {
		tag = locales.Default
	}
	return locales.In(tag).SettingsGeneralLanguageName()
}

// updateRows are the version with 检查更新…, which checks at once and shows what it finds.
type updateRows struct{ s screen }

// Render implements core.Node.
func (u updateRows) Render(cx *core.Scope) core.Node {
	a := u.s.a
	// TODO(nsgo#32): say under 版本 the newer version the last check found or when that check was, and add the switches 自动检查更新 and 自动下载并安装更新 (with its hint) where Kando can update itself, as MyGo's updater plugin reported and set them; NSGo's updater has no such state or settings https://github.com/nsgo-dev/nsgo/issues/32
	return form.Row{Title: locales.SettingsGeneralVersion(cx), Trailing: []core.Node{
		form.Value(static(u.s.version), 13),
		form.Button{Text: locales.UpdatesCheck(cx), OnPress: live.Do(a, a.CheckForUpdates), TestID: "settings-check-updates"},
	}}
}

// libraryRows choose the library, show where it is, and take it off the list. Without a library they offer to make or open one.
type libraryRows struct{ s screen }

// Render implements core.Node.
func (l libraryRows) Render(cx *core.Scope) core.Node {
	a := l.s.a
	return core.Show{When: live.Func(a, func() bool { return a.LibraryDir() == "" }),
		Then: func(cx *core.Scope) core.Node {
			return form.Row{Title: locales.SettingsGeneralCurrentLibrary(cx), Subtitle: locales.SettingsGeneralNoLibrary(cx), Trailing: []core.Node{
				form.Button{Text: locales.SettingsGeneralNewLibrary(cx), OnPress: live.Do(a, a.BeginNew), TestID: "settings-library-new"},
				form.Button{Text: locales.SettingsGeneralOpenLibrary(cx), OnPress: live.Do(a, a.PickLibrary), TestID: "settings-library-open"},
			}}
		},
		Else: func(cx *core.Scope) core.Node { return libraryShown(cx, a) }}
}

// libraryShown is the rows of the library open: the drop-down of the libraries Kando knows, where the open one is with 在访达中显示, and 移除 while there is another to go to.
func libraryShown(cx *core.Scope, a App) core.Node {
	// dirs is the libraries known, joined, so the drop-down is built again only when they change.
	dirs := core.NewMemo(cx, live.Func(a, func() string { return strings.Join(a.KnownLibraries(), "\x00") }))
	choose := core.Dynamic(func(cx *core.Scope) core.Node {
		var list []string
		if joined := dirs.Get(); joined != "" {
			list = strings.Split(joined, "\x00")
		}
		labels := a.LibraryLabels(list)
		options := make([]ui.Option[string], len(list))
		for i, dir := range list {
			options[i] = ui.Option[string]{Value: dir, Label: labels[i]}
		}
		return ui.Select[string]{Options: options, Label: locales.SettingsGeneralCurrentLibrary(cx), TestID: "settings-library",
			Value: live.Func(a, func() string {
				open := a.LibraryDir()
				for _, dir := range list {
					if filepath.Clean(dir) == filepath.Clean(open) {
						return dir
					}
				}
				return open
			}),
			OnChange: live.On(a, func(dir string) {
				if filepath.Clean(dir) != filepath.Clean(a.LibraryDir()) {
					a.Choose(dir)
				}
			}),
			Style: core.Style{Width: core.Pt(220)}}
	})
	location := form.Row{Title: locales.SettingsGeneralLocation(cx), Trailing: []core.Node{
		form.Value(live.Func(a, func() string { return a.DisplayPath(a.LibraryDir()) }), 12),
		form.Button{Text: locales.SettingsGeneralShowInFinder(cx, desktop.Platform(cx)), OnPress: live.Do(a, func() { a.Reveal(a.LibraryDir()) }), TestID: "settings-library-reveal"},
	}}
	forget := core.Show{When: live.Func(a, func() bool { return len(a.KnownLibraries()) > 1 }), Then: func(cx *core.Scope) core.Node {
		return core.Group([]core.Node{
			form.Separator(true),
			form.Row{Title: locales.SettingsGeneralForget(cx), Subtitle: locales.SettingsGeneralForgetHint(cx), Trailing: []core.Node{
				form.Button{Text: locales.SettingsGeneralForgetButton(cx), OnPress: live.Do(a, func() { a.Forget(a.LibraryDir()) }), TestID: "settings-library-forget"},
			}},
		})
	}}
	return core.Group([]core.Node{
		form.Row{Title: locales.SettingsGeneralCurrentLibrary(cx), NoGroup: true, Trailing: []core.Node{choose}},
		form.Separator(true),
		location,
		forget,
	})
}

// ToolFetch is Kando's own download of a program: whether it can download it here, how far one under way has come (Progress, from 0 to 1, or -1 before its size is known), and why the last one failed.
type ToolFetch struct {
	Available bool
	Running   bool
	Progress  float64
	Err       string
}

// toolState is what the end of a program's row shows.
type toolState uint8

const (
	toolInstalled toolState = iota
	toolDownloading
	toolDownloadable
	toolMissing
)

// toolRow is one of the programs Kando runs: its tile, its name over what it is for and where it was found, and a dot with its version. One the computer lacks shows Kando's download of it: 下载 or 重试, or its progress while it runs, or 未安装 where Kando cannot download it. path picks the program's path out of the tools.
type toolRow struct {
	s               screen
	mark, name, use string
	path            func(media.Tools) string
}

// Render implements core.Node.
func (r toolRow) Render(cx *core.Scope) core.Node {
	a := r.s.a
	colors := ui.ColorsOf(cx)
	path := func() string {
		tools, _ := a.Tools()
		return r.path(tools)
	}
	where := live.Func(a, func() string {
		if p := path(); p != "" {
			return a.DisplayPath(p)
		}
		switch fetch := a.ToolDownload(r.name); {
		case fetch.Running:
			return locales.SettingsGeneralDownloading(cx)
		case fetch.Err != "":
			return locales.SettingsGeneralDownloadFailed(cx, fetch.Err)
		}
		return locales.SettingsGeneralNotInstalled(cx)
	})
	state := core.NewMemo(cx, live.Func(a, func() toolState {
		if path() != "" {
			return toolInstalled
		}
		switch fetch := a.ToolDownload(r.name); {
		case fetch.Running:
			return toolDownloading
		case fetch.Available:
			return toolDownloadable
		}
		return toolMissing
	}))
	end := core.Dynamic(func(cx *core.Scope) core.Node {
		switch state.Get() {
		case toolInstalled:
			return form.Status(ui.Fixed(form.OK), colors.Muted, live.Func(a, func() string {
				if version := a.ToolVersion(path()); version != "" {
					return version
				}
				return locales.SettingsGeneralInstalled(cx)
			}))
		case toolDownloading:
			progress := live.Func(a, func() float64 { return a.ToolDownload(r.name).Progress })
			return ui.Progress{Label: locales.SettingsGeneralDownloading(cx), TestID: "settings-tool-progress-" + r.name, Style: core.Style{Width: core.Pt(72), FlexShrink: 0},
				Value:         func() float32 { return float32(max(progress(), 0) * 100) },
				Indeterminate: func() bool { return progress() < 0 }}
		case toolDownloadable:
			return form.Button{TestID: "settings-tool-download-" + r.name, OnPress: live.Do(a, func() { a.DownloadTool(r.name) }),
				TextFunc: live.Func(a, func() string {
					if a.ToolDownload(r.name).Err != "" {
						return locales.SettingsGeneralRetry(cx)
					}
					return locales.SettingsGeneralDownload(cx)
				})}
		}
		missing := form.Status(colors.Danger, colors.Muted, static(locales.SettingsGeneralNotInstalled(cx)))
		// Where Kando cannot download it, as on Windows, the program's own page of builds is where to get it.
		if src, ok := media.SourceFor(r.name); ok {
			return core.View{Style: core.Style{Direction: core.Row, Gap: 12, AlignItems: core.AlignCenter, FlexShrink: 0}, Children: []core.Node{
				missing,
				ui.Link{Text: locales.SettingsGeneralToolPage(cx), Label: locales.SettingsGeneralToolPageLabel(cx, r.name), Href: src.Page, TestID: "settings-tool-page-" + r.name},
			}}
		}
		return missing
	})
	k := theme.Of(cx)
	tile := core.View{Style: core.Style{Width: core.Pt(30), Height: core.Pt(30), FlexShrink: 0, CornerRadius: 8, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}, StyleFunc: ui.Filled(k.Tile),
		Children: []core.Node{core.Text{Content: r.mark, Role: core.RoleNone, Style: core.Style{FontFamily: "monospace", FontSize: 10, FontWeight: ui.Bold}, StyleFunc: ui.Colored(k.Strong)}}}
	texts := core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 1}, Children: []core.Node{
		core.Text{Content: r.name, Style: core.Style{FontSize: 13, FontWeight: ui.Semibold}, StyleFunc: ui.Colored(colors.Text)},
		core.Text{ContentFunc: func() string { return locales.SettingsGeneralToolDetail(cx, r.use, where()) }, Style: core.Style{FontSize: 11}, StyleFunc: ui.Colored(colors.Muted)},
	}}
	return core.View{Role: core.RoleGroup, Label: r.name, TestID: "settings-tool-" + r.name,
		Style:    core.Style{Direction: core.Row, FlexShrink: 0, MinHeight: core.Pt(48), Padding: core.Symmetric(6, 14), Gap: 12, AlignItems: core.AlignCenter},
		Children: []core.Node{tile, texts, end}}
}
