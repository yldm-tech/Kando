package settings

import (
	"lingocut/apps/lingocut/account"
	"lingocut/apps/lingocut/film"
	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// Preset is one translation service the AI page can select.
type Preset struct {
	ID, Name, Blurb, Group string
}

// Shortcut is one row of the app's shortcut table, as the 快捷键 page lists it.
type Shortcut struct {
	Group, Label string
	Mods         ui.Modifiers
	Key          ui.Key
}

// App is the settings the window reads and the edits it reports. The 账号与上传 page is the account package's.
type App interface {
	account.App

	// SettingsSection is the Section value the window shows.
	SettingsSection() string
	SetSettingsSection(section string)
	// SettingsQuery is the text of the window's search field.
	SettingsQuery() *string
	SaveErr() string

	LibraryDir() string
	KnownLibraries() []string
	Choose(dir string)
	BeginNew()
	PickLibrary()
	Reveal(path string)
	Forget(dir string)
	Tools() (media.Tools, bool)
	Redetect()
	ToolVersion(path string) string

	// InstalledSpeechModel is the file on disk of the whisper catalog model id, "" when it is not downloaded. The app looks the models up when the window opens, when the model in use changes and on 重新检测, never while the page is drawn.
	InstalledSpeechModel(id string) string
	UseSpeechModel(path string)
	Downloading() string
	DownloadWritten() int64
	DownloadTotal() int64
	DownloadErr() string
	CancelDownload()
	DownloadModel(id string)

	CurrentPreset() (Preset, bool)
	Presets() []Preset
	LLMProvider() string
	SetLLMProvider(id string)
	ProviderID(base string) string
	LLMBase() *string
	LLMKey() *string
	LLMModel() *string
	ApplyLLM()
	TestingLLM() bool
	TestLLM()
	LLMResult() string
	SelectLLM(id string)

	Language() *string
	TranslateContext() *bool
	TranslatePrompt() *string

	// Cue is the subtitle look the player draws with; the 字幕外观 page previews it and does not change it.
	Cue() *film.Look
	SubMode() string
	SetSubMode(mode string)
	SubOrder() string
	SetSubOrder(order string)
	Untranslated() string
	SetUntranslated(mode string)
	YieldSubs() bool
	SetYieldSubs(on bool)

	Shortcuts() []Shortcut
	// ShortcutQuery is the text of the 快捷键 page's search field.
	ShortcutQuery() *string
}

type screen struct {
	App
	version string
}

// railWidth is the width of the window's category sidebar, as the prototype's.
const railWidth float32 = 220

// Window draws the settings window: the categories with a search field on the left, the chosen one's page on the right. t is the window's theme and dark the player's, which the 字幕外观 preview is drawn in.
func Window(c *ui.Context, t, dark *ui.Theme, version string, a App) {
	s := screen{App: a, version: version}
	vibrancy := c.Vibrancy()
	if vibrancy {
		c.Root().Background(ui.Transparent)
	}
	bar := c.TitleBar()
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		s.rail(c, t, bar)
		ui.Box(c).Width(1).Shrink(0).Background(t.Border)
		pane := ui.Column(c).Grow(1).MinWidth(0).AlignItems(ui.Stretch)
		if vibrancy {
			pane.Background(t.Background)
		}
		pane.Children(func() {
			breeze.HeaderBar(c, title(s.SettingsSection()), "", nil, nil)
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).WidthPercent(100).Padding(14, 28, 28, 28).Gap(18).Children(func() {
					s.page(c, t, dark)
					if s.SaveErr() != "" {
						breeze.Text(c, s.SaveErr()).TextColor(t.Danger).Wrap()
					}
				})
			})
		})
	})
}

// rail is the window's sidebar: room for the traffic lights, the search field, and the categories it finds.
func (s screen) rail(c *ui.Context, t *ui.Theme, bar ui.TitleBar) {
	ui.Column(c).Width(railWidth).Shrink(0).AlignItems(ui.Stretch).Children(func() {
		top := ui.Row(c).Height(max(float32(bar.Height), 12)).DragWindow()
		if bar.Left > 0 {
			top.PaddingStart(float32(bar.Left))
		}
		ui.Column(c).Padding(0, 10, 8, 10).Children(func() {
			breeze.SearchField(c, s.SettingsQuery()).Label("搜索设置").Placeholder("搜索")
		})
		query := *s.SettingsQuery()
		sel := s.SettingsSection()
		found := 0
		breeze.Sidebar(c, &sel, func() {
			for _, sec := range sections {
				if !sec.matches(query) {
					continue
				}
				found++
				categoryItem(c, sec)
			}
		}).Background(ui.Transparent).Grow(1)
		if found == 0 {
			breeze.Text(c, "没有匹配的设置", breeze.TextCaption, breeze.TextSecondary).Padding(0, 18)
		}
		if sel != s.SettingsSection() {
			s.SetSettingsSection(sel)
		}
	})
}

// categoryItem is a category of the rail, with its colored tile drawn untinted, as the prototype's settings sidebar shows it.
func categoryItem(c *ui.Context, sec section) ui.Element {
	return breeze.SidebarItemImage(c, sec.id, sec.icon, sec.title)
}

// page draws the chosen category.
func (s screen) page(c *ui.Context, t, dark *ui.Theme) {
	switch s.SettingsSection() {
	case SectionSpeech:
		s.speech(c, t)
	case SectionAI:
		s.ai(c, t)
	case SectionTranslate:
		s.translate(c, t)
	case SectionLook:
		s.look(c, t, dark)
	case SectionShortcuts:
		s.shortcuts(c, t)
	case SectionAccount:
		account.Show(c, t, s.App)
	default:
		s.general(c, t)
	}
}
