package settings

import (
	"path/filepath"
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	"kando/apps/kando/sidebar"
	"kando/packages/media"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// general is the 通用 page: the library, the interface language, the version and its updates, the programs Kando runs, and its command line and MCP server (cli.go).
func (s screen) general(c *ui.Context, t *ui.Theme) {
	form.Label(c, locales.SettingsGeneralLibrary(), 6, nil)
	form.Card(c, func() { s.libraryRows(c) })
	form.Note(c, locales.SettingsGeneralLibraryNote(), 10)
	form.Card(c, func() {
		form.Row(c, locales.SettingsGeneralInterfaceLanguage(), "", func() {
			c.Key("interface-language")
			selected := InterfaceLanguageName(s.InterfaceLanguage())
			if form.Select(c, &selected, InterfaceLanguageChoices()).Label(locales.SettingsGeneralInterfaceLanguage()).Changed() {
				s.SetInterfaceLanguage(InterfaceLanguageTag(selected))
				s.Save()
			}
		})
		form.Separator(c, true)
		s.updateRows(c)
	})
	tools, known := s.Tools()
	var redetect func()
	if known {
		redetect = func() {
			c.Key("redetect")
			if form.Button(c, locales.SettingsGeneralRedetect(), true).Clicked() {
				s.Redetect()
			}
		}
	}
	form.Label(c, locales.SettingsGeneralComponents(), 14, redetect)
	form.Card(c, func() {
		s.toolRow(c, t, "FF", "ffmpeg", locales.SettingsGeneralUseFFmpeg(), tools.FFmpeg)
		form.Separator(c, false)
		s.toolRow(c, t, "YT", "yt-dlp", locales.SettingsGeneralUseYTDLP(), tools.YTDLP)
		form.Separator(c, false)
		s.toolRow(c, t, "W", "whisper-cli", locales.SettingsGeneralUseWhisper(), tools.Whisper)
	})
	note := locales.SettingsGeneralToolsNote()
	// Where Kando downloads no programs, as on Windows, they are the user's to install.
	if _, ok := media.KitFor("ffmpeg"); !ok {
		note = locales.SettingsGeneralToolsNoteManual()
	}
	form.Note(c, note, 0)
	s.cli(c, t)
}

// updateRows are the version with 检查更新…, under it the newer version the last check found or when that check was, and, where Kando can update itself, whether it checks and installs updates by itself. Where it cannot, as a development build or the Debian package, 检查更新… opens the update window, which says why.
func (s screen) updateRows(c *ui.Context) {
	u := s.Updates()
	note := ""
	if u.Available != "" {
		note = locales.SettingsGeneralUpdateAvailable(u.Available)
	} else if !u.LastCheck.IsZero() {
		note = locales.SettingsGeneralLastCheck(u.LastCheck)
	}
	form.Row(c, locales.SettingsGeneralVersion(), note, func() {
		form.Value(c, s.version, 13).Selectable()
		c.Key("check-updates")
		if form.Button(c, locales.UpdatesCheck(), false).Clicked() {
			s.CheckForUpdates()
		}
	})
	if !u.Enabled {
		return
	}
	form.Separator(c, true)
	c.Key("auto-check-updates")
	if on := u.AutoCheck; form.SwitchRow(c, locales.SettingsGeneralAutoCheck(), "", &on) {
		s.SetAutoCheckUpdates(on)
	}
	// Updates are downloaded only by the checks in the background, so the switch goes with them.
	if u.AutoCheck {
		form.Separator(c, true)
		c.Key("auto-download-updates")
		if on := u.AutoDownload; form.SwitchRow(c, locales.SettingsGeneralAutoDownload(), locales.SettingsGeneralAutoDownloadHint(), &on) {
			s.SetAutoDownloadUpdates(on)
		}
	}
}

// Updates is how Kando updates itself, as the 通用 page shows it: Enabled says this copy can (not a development build, nor one a package manager installed), AutoCheck and AutoDownload are the user's choices, Available is the newer version the last check found ("" for none, or one the user skipped) and LastCheck when that check was (zero before the first).
type Updates struct {
	Enabled, AutoCheck, AutoDownload bool
	Available                        string
	LastCheck                        time.Time
}

// InterfaceLanguageChoices are the languages for which Kando ships a catalog (locales.Interface).
// They are named in themselves so the choice remains understandable after the app has switched languages.
func InterfaceLanguageChoices() []string {
	names := make([]string, len(locales.Interface))
	for i, tag := range locales.Interface {
		names[i] = InterfaceLanguageName(tag)
	}
	return names
}

// InterfaceLanguageName turns a language tag of locales.Interface into the label shown in the settings pop-up, its name in itself, as 繁體中文 of zh-Hant.
func InterfaceLanguageName(tag string) string {
	if !locales.Supported(tag) {
		tag = locales.Default
	}
	return i18n.Autonym(tag)
}

// InterfaceLanguageTag is the inverse of InterfaceLanguageName.
func InterfaceLanguageTag(name string) string {
	for _, tag := range locales.Interface {
		if i18n.Autonym(tag) == name {
			return tag
		}
	}
	return locales.Default
}

// libraryRows choose the library, show where it is, and take it off the list. Without a library they offer to make or open one.
func (s screen) libraryRows(c *ui.Context) {
	dir := s.LibraryDir()
	if dir == "" {
		form.Row(c, locales.SettingsGeneralCurrentLibrary(), locales.SettingsGeneralNoLibrary(), func() {
			c.Key("library-new")
			if form.Button(c, locales.SettingsGeneralNewLibrary(), false).Clicked() {
				s.BeginNew()
			}
			c.Key("library-open")
			if form.Button(c, locales.SettingsGeneralOpenLibrary(), false).Clicked() {
				s.PickLibrary()
			}
		})
		return
	}
	dirs := s.KnownLibraries()
	labels := libraryLabels(dirs)
	chosen := ""
	for i, known := range dirs {
		if filepath.Clean(known) == filepath.Clean(dir) {
			chosen = labels[i]
		}
	}
	form.Row(c, locales.SettingsGeneralCurrentLibrary(), "", func() {
		c.Key("library")
		if form.Select(c, &chosen, labels).Label(locales.SettingsGeneralCurrentLibrary()).Changed() {
			for i, label := range labels {
				if label == chosen && filepath.Clean(dirs[i]) != filepath.Clean(dir) {
					s.Choose(dirs[i])
				}
			}
		}
	})
	form.Separator(c, true)
	form.Row(c, locales.SettingsGeneralLocation(), "", func() {
		form.Value(c, films.DisplayPath(dir), 12).Selectable()
		c.Key("library-reveal")
		if form.Button(c, locales.SettingsGeneralShowInFinder(), false).Clicked() {
			s.Reveal(dir)
		}
	})
	if len(dirs) > 1 {
		form.Separator(c, true)
		form.Row(c, locales.SettingsGeneralForget(), locales.SettingsGeneralForgetHint(), func() {
			c.Key("library-forget")
			if form.Button(c, locales.SettingsGeneralForgetButton(), false).Clicked() {
				s.Forget(dir)
			}
		})
	}
}

// libraryLabels names each library in the drop-down by its title, or by its path where two share a title.
func libraryLabels(dirs []string) []string {
	count := map[string]int{}
	for _, dir := range dirs {
		count[sidebar.Title(dir)]++
	}
	labels := make([]string, len(dirs))
	for i, dir := range dirs {
		labels[i] = sidebar.Title(dir)
		if count[labels[i]] > 1 {
			labels[i] = films.DisplayPath(dir)
		}
	}
	return labels
}

// ToolFetch is Kando's own download of a program: whether it can download it here, how far one under way has come (Progress, from 0 to 1, or -1 before its size is known), and why the last one failed.
type ToolFetch struct {
	Available bool
	Running   bool
	Progress  float64
	Err       string
}

// toolRow is one of the programs Kando runs: its tile, its name over what it is for and where it was found, and a dot with its version. One the computer lacks shows Kando's download of it: 下载 or 重试, or its progress while it runs, or 未安装 where Kando cannot download it.
func (s screen) toolRow(c *ui.Context, t *ui.Theme, mark, name, use, path string) {
	where := locales.SettingsGeneralNotInstalled()
	fetch := ToolFetch{}
	if path != "" {
		where = films.DisplayPath(path)
	} else {
		fetch = s.ToolDownload(name)
		switch {
		case fetch.Running:
			where = locales.SettingsGeneralDownloading()
		case fetch.Err != "":
			where = locales.SettingsGeneralDownloadFailed(fetch.Err)
		}
	}
	ui.Row(c).Shrink(0).MinHeight(48).Padding(6, 14).Gap(12).AlignItems(ui.Center).Role(ui.RoleGroup).Label(name).Children(func() {
		ui.Row(c).Size(30, 30).Shrink(0).Radius(8).Background(ui.Hex("#F0F1F4")).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			ui.Text(c, mark).Font("monospace").FontSize(10).FontWeight(700).TextColor(ui.Hex("#3A3A3F")).Role(ui.RoleNone)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, name).FontSize(13).FontWeight(600).TextColor(form.Text)
			ui.Text(c, locales.SettingsGeneralToolDetail(use, where)).FontSize(11).TextColor(form.Secondary).Wrap()
		})
		switch {
		case path != "":
			status := locales.SettingsGeneralInstalled()
			if version := s.ToolVersion(path); version != "" {
				status = version
			}
			form.Status(c, form.OK, form.Secondary, status)
		case fetch.Running:
			breeze.Progress(c, fetch.Progress).Width(72).Shrink(0).Label(locales.SettingsGeneralDownloading())
		case fetch.Available:
			label := locales.SettingsGeneralDownload()
			if fetch.Err != "" {
				label = locales.SettingsGeneralRetry()
			}
			c.Key("tool-download-" + name)
			if form.Button(c, label, false).Clicked() {
				s.DownloadTool(name)
			}
		default:
			form.Status(c, t.Danger, form.Secondary, locales.SettingsGeneralNotInstalled())
		}
	})
}
