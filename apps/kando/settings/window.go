package settings

import (
	"kando/apps/kando/account"
	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/settings/form"
	"kando/packages/media"

	"github.com/egoist/mygo/ui"
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
	// LLMModels are the models the AI service offers, fetched while its page shows; LoadingLLMModels reports a fetch in flight, LLMModelsErr why the last one failed, and RetryLLMModels fetches again.
	LLMModels() []string
	LoadingLLMModels() bool
	LLMModelsErr() string
	RetryLLMModels()

	Language() *string
	TranslateContext() *bool
	TranslatePrompt() *string

	// InterfaceLanguage is the app UI language tag ("en" or "zh-Hans").
	// It is separate from Language, which is the language subtitles are translated into.
	InterfaceLanguage() string
	SetInterfaceLanguage(tag string)

	// Cue is the subtitle look the player draws with; the 字幕外观 page previews it and does not change it.
	Cue() *subtitle.Look
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

// The rail's look, as the prototype's settings sidebar: 220 points wide with its line along the end, a translucent gray over the window's, and the chosen category in Kando's blue.
const railWidth float32 = 220

var (
	railFill = ui.RGBA(236, 236, 240, 0.92)
	railLine = ui.RGBA(0, 0, 0, 0.1)
	chosen   = ui.Hex("#0A66D8")
)

// Window draws the settings window: the categories with a search field on the left, the chosen one's page on the right under its title. t is the window's theme and dark the player's, which the 字幕外观 preview is drawn in.
func Window(c *ui.Context, t, dark *ui.Theme, version string, a App) {
	s := screen{App: a, version: version}
	bar := c.TitleBar()
	ui.Row(c).Fill().AlignItems(ui.Stretch).Background(form.Window).Children(func() {
		s.rail(c, bar)
		ui.Column(c).Grow(1).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
			header(c, title(s.SettingsSection()))
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).WidthPercent(100).Padding(14, 28, 28, 28).Gap(6).AlignItems(ui.Stretch).Children(func() {
					s.page(c, t, dark)
					if s.SaveErr() != "" {
						ui.Text(c, s.SaveErr()).FontSize(11).TextColor(t.Danger).Wrap().Padding(0, 12)
					}
				})
			})
		})
	})
}

// header is the page's title bar: 50 points high with a line under it, the title 15-point bold where the page's content starts. It moves the window, as the title bar it stands in for does.
func header(c *ui.Context, text string) {
	ui.Column(c).Shrink(0).AlignItems(ui.Stretch).Children(func() {
		ui.Row(c).Height(50).Padding(0, 28).AlignItems(ui.Center).DragWindow().Children(func() {
			ui.Text(c, text).FontSize(15).FixedLineHeight(18).FontWeight(700).TextColor(form.Text).SingleLine().Role(ui.RoleHeading)
		})
		ui.Box(c).Height(1).Shrink(0).Background(form.Line)
	})
}

// rail is the window's sidebar: room for the traffic lights, the search field, and the categories it finds.
func (s screen) rail(c *ui.Context, bar ui.TitleBar) {
	ui.Column(c).Width(railWidth).Shrink(0).AlignItems(ui.Stretch).Padding(0, 10, 12, 10).Gap(2).Background(railFill).BorderEndWidth(1).BorderColor(railLine).Children(func() {
		top := ui.Row(c).Height(max(float32(bar.Height), 50)).Shrink(0).DragWindow()
		if bar.Left > 0 {
			top.PaddingStart(float32(bar.Left))
		}
		ui.SearchField(c, s.SettingsQuery()).Label("搜索设置").Placeholder("搜索").Height(28).Shrink(0).Radius(8).Border(0, ui.Transparent).Background(ui.RGBA(0, 0, 0, 0.06)).FontSize(13).Padding(0, 9, 0, 8).Margin(0, 0, 8, 0)
		query := *s.SettingsQuery()
		sel := s.SettingsSection()
		found := 0
		ui.Sidebar(c, &sel, func() {
			for _, sec := range sections {
				if !sec.matches(query) {
					continue
				}
				found++
				categoryItem(c, sec, sec.id == sel)
			}
		}).Background(ui.Transparent).Padding(0).Gap(2).Grow(1)
		if found == 0 {
			ui.Text(c, "没有匹配的设置").FontSize(12).TextColor(form.Secondary).Padding(10, 8)
		}
		if sel != s.SettingsSection() {
			s.SetSettingsSection(sel)
		}
	})
}

// categoryItem is a category of the rail: its colored tile and its title, 30 points high, filled in blue with a white title while chosen, as the prototype's settings sidebar draws it whether or not it has the focus.
func categoryItem(c *ui.Context, sec section, on bool) {
	look := *c.Theme()
	look.SurfaceHover, look.SurfacePressed = ui.Transparent, ui.Transparent
	var item ui.Element
	ui.Themed(c, &look).Background(ui.Transparent).Children(func() { item = ui.SidebarItem(c, sec.id, nil, sec.title) })
	item.MinHeight(30).Padding(0, 8).PaddingStart(8 + 22 + 9).Radius(8).FontSize(13).FontWeight(400)
	if on {
		item.Background(chosen).TextColor(ui.Hex("#FFFFFF"))
	} else {
		item.Background(ui.Transparent).TextColor(form.Text)
	}
	item.Children(func() {
		ui.Column(c).Absolute().Top(0).Bottom(0).Start(8).Justify(ui.Center).PassThrough().Role(ui.RoleNone).Children(func() {
			ui.Image(c, sec.icon).Size(22, 22).PassThrough()
		})
	})
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
