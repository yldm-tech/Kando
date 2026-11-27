package main

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/aiusage"
	"kando/apps/kando/browserext"
	"kando/apps/kando/clitool"
	"kando/apps/kando/everyapi"
	"kando/apps/kando/hotkey"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/phonesync"
	"kando/apps/kando/settings"
	"kando/apps/kando/toolhealth"
	"kando/packages/media"
	psync "kando/packages/sync"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsSettings renders the settings window, one shot per category, at the prototype's 880 by 640 with the traffic lights in their 50-point band (screenshot_helpers_test.go). The window has a view of its own, so it is drawn here rather than through the fixture's shoot, which draws the main window.
func TestScreenshotsSettings(t *testing.T) {
	f := newScreenshotFixture(t)
	shown, models, turbo := settingsShotFolders(t, f)
	for _, section := range []string{settings.SectionGeneral, settings.SectionSpeech, settings.SectionAI, settings.SectionTranslate, settings.SectionLook, settings.SectionPrivacy, settings.SectionShortcuts, settings.SectionAccount, settings.SectionPhone} {
		shootSettings(t, f, section, "settings-"+section, shown, models, turbo, func(a *app) { shotCLITool(f, a, true) })
	}
	// The 命令行与 MCP card at the end of 通用: before the kando command is installed, with 安装; and once a terminal finds it, after 拷贝配置.
	shootSettingsScrolled(t, f, settings.SectionGeneral, "settings-general-cli", shown, models, turbo, func(a *app) { shotCLITool(f, a, false) })
	shootSettingsScrolled(t, f, settings.SectionGeneral, "settings-general-cli-installed", shown, models, turbo, func(a *app) {
		shotCLITool(f, a, true)
		a.cliTool.copied = clitool.MCPConfig(a.cliTool.status.MCPCommand())
	})
	// The 浏览器扩展 card at the very end of 通用: Chrome and Firefox on the computer, before Kando is connected to them, with 连接; and once it is, with 断开连接.
	shootSettingsScrolled(t, f, settings.SectionGeneral, "settings-general-browser", shown, models, turbo, func(a *app) { shotCLITool(f, a, true); shotBrowserExt(a, false) })
	shootSettingsScrolled(t, f, settings.SectionGeneral, "settings-general-browser-connected", shown, models, turbo, func(a *app) { shotCLITool(f, a, true); shotBrowserExt(a, true) })
	// The 浏览器登录 card above it: Chrome chosen, whose sign-in yt-dlp read; and Safari, which macOS keeps from Kando without Full Disk Access, with 打开系统设置 and 重新检查.
	shootSettingsScrolled(t, f, settings.SectionGeneral, "settings-general-signin", shown, models, turbo, func(a *app) { shotCLITool(f, a, true); shotSignIn(a, "chrome", "") })
	shootSettingsScrolled(t, f, settings.SectionGeneral, "settings-general-signin-safari", shown, models, turbo, func(a *app) {
		shotCLITool(f, a, true)
		shotSignIn(a, "safari", media.LoginNoAccess)
	})
	// Chrome, whose data folder macOS keeps from Kando without Full Disk Access, which yt-dlp takes for no sign-in data: the same 打开系统设置 and 重新检查.
	shootSettingsScrolled(t, f, settings.SectionGeneral, "settings-general-signin-chrome-access", shown, models, turbo, func(a *app) {
		shotCLITool(f, a, true)
		shotSignIn(a, "chrome", media.LoginNoAccess)
	})
	// 隐私 with offline mode on: every step over the internet off, speech recognition on this computer.
	shootSettings(t, f, settings.SectionPrivacy, "settings-privacy-offline", shown, models, turbo, func(a *app) { shotCLITool(f, a, true); a.SetOffline(true) })
	// 手机同步 on, while the page offers a code: its QR code with the address, the code and the fingerprint, and one phone paired.
	shootSettings(t, f, settings.SectionPhone, "settings-phone-paired", shown, models, turbo, shotPhoneSync)
	// EveryAPI without a key on a computer without the everyapi program, and Kando not signed in: the EveryAPI account row under API Key offers 登录, with the program as the other way.
	shootSettings(t, f, settings.SectionAI, "settings-ai-everyapi-missing", shown, models, turbo, func(a *app) {
		tools := a.pipeline.(media.Tools)
		tools.EveryAPI = ""
		a.syncLLM(tools.UseEveryAPICLI())
	})
	// The everyapi program is there but nobody signed in to it, as the sidebar's check found: the account row offers 登录, and the program's row says what to run in a terminal.
	shootSettings(t, f, settings.SectionAI, "settings-ai-everyapi-signed-out", shown, models, turbo, func(a *app) {
		a.side.ready.signInDone, a.side.ready.signInErr, a.side.ready.signInSource = true, media.ErrEveryAPISignedOut, media.CredentialCLI
	})
	// The end of AI 服务: 本月用量 with every pass but term suggestions, the plugins' estimated as their service said no usage; the plugins' row and the 80 requests are checked headless too.
	usage := testlocale.Messages()
	shootSettingsAt(t, f, settings.SectionAI, "settings-ai-usage", shown, models, turbo, true, []string{
		"exists:id:settings-aiusage-plugin >> " + whole(usage.AiusageTokensEstimated(3*(2200+540))),
		"exists:id:settings-aiusage-requests >> " + whole(usage.AiusageRequestsCount(80)),
	}, shotAIUsage)
	// Signed in to EveryAPI in Kando: the account's name and balance, with 充值 and 退出; and the same with little left.
	shootSettings(t, f, settings.SectionAI, "settings-ai-everyapi-account", shown, models, turbo, func(a *app) { shotEveryAPISignedIn(a, 12.4) })
	shootSettings(t, f, settings.SectionAI, "settings-ai-everyapi-low", shown, models, turbo, func(a *app) { shotEveryAPISignedIn(a, 0.42) })
	// The end of 语音识别, with Japanese named as the language spoken in the videos.
	shootSettingsScrolled(t, f, settings.SectionSpeech, "settings-speech-language", shown, models, turbo, func(a *app) {
		tools := a.pipeline.(media.Tools)
		tools.SpeechLanguage = "ja"
		a.pipeline = tools
	})
	// The confirmation of moving Base, which Kando downloaded and which is not in use, to the Trash, as ⋯ › 移到废纸篓… opens it.
	shootSettings(t, f, settings.SectionSpeech, "settings-speech-trash", shown, models, turbo, func(a *app) {
		a.prefs.trashModel, a.prefs.trashOpen = "base", true
	})
	// The first launch on a computer without ffmpeg and whisper-cli: Kando downloads ffmpeg, and offers 重试 for whisper-cli, whose download stalled.
	shootSettings(t, f, settings.SectionGeneral, "settings-general-downloading", shown, models, turbo, func(a *app) {
		shotKitsDownloading(t, f, a)
		a.kits.running = map[string]*kitDownload{"ffmpeg": {written: 13 << 20, total: 31 << 20, cancel: func() {}}}
		a.kits.errs = map[string]error{"whisper-cli": media.ErrStalled}
	})
	// A Homebrew ffmpeg that cannot load a library Homebrew upgraded under it: the row says it does not run and why, and offers Kando's own.
	shootSettings(t, f, settings.SectionGeneral, "settings-general-broken", shown, models, turbo, func(a *app) {
		shotOwnToolsDir(t, f)
		tools := a.pipeline.(media.Tools)
		a.tools.health[tools.FFmpeg] = media.Health{State: media.HealthBroken, Problem: media.ProblemLibrary, Library: "libbluray"}
	})
	// Kando runs its own ffmpeg, and the computer has one that works too: which copy runs is a choice under the name.
	shootSettings(t, f, settings.SectionGeneral, "settings-general-copy", shown, models, turbo, func(a *app) {
		dir := shotOwnToolsDir(t, f)
		tools := a.pipeline.(media.Tools)
		kit, _ := media.KitFor("ffmpeg")
		own := filepath.Join(dir, kit.ID, "ffmpeg")
		data, err := os.ReadFile(tools.FFmpeg)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(own), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(own, data, 0o755); err != nil {
			t.Fatal(err)
		}
		a.tools.health[own] = a.tools.health[tools.FFmpeg]
		a.tools.others["ffmpeg"] = tools.FFmpeg
		tools.FFmpeg = own
		a.pipeline = tools
	})
	// A speech model's download from Hugging Face that lost its connection: the page says so, with 从 hf-mirror.com 下载 next to it.
	shootSettingsScrolled(t, f, settings.SectionSpeech, "settings-speech-download-failed", shown, models, turbo, func(a *app) {
		a.prefs.downloadErr = settings.DownloadError{ID: "large-v3-turbo-q5", Err: fmt.Errorf("%w: unexpected EOF", media.ErrDisconnected)}
	})
	// The end of 语音识别 with four threads and two proper nouns as hints, and the whisper-cli command they make.
	shootSettingsScrolled(t, f, settings.SectionSpeech, "settings-speech-params", shown, models, turbo, func(a *app) {
		tools := a.pipeline.(media.Tools)
		tools.WhisperThreads, tools.WhisperHints = 4, "Monterey\nKando"
		a.pipeline = tools
	})
	// 快捷键 with the mpv preset: the line keys on Ctrl+← and Ctrl+→, the volume on 9 and 0.
	shootSettings(t, f, settings.SectionShortcuts, "settings-shortcuts-mpv", shown, models, turbo, func(a *app) { a.prefs.shortcutPreset = hotkey.PresetMPV })
}

// shotToolsChecked runs the copies of the programs a's pipeline names, the fixture's fake tools, as the check once the window is ready does (toolsCheck), but at once, as the scenes' updates are dropped: the rows show their versions rather than 检查中.
func shotToolsChecked(a *app) {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	for _, program := range toolhealth.Programs {
		if path := *toolPath(&tools, program); path != "" {
			a.tools.health[path] = media.Probe(context.Background(), program, path)
		}
	}
}

// shotOwnToolsDir gives Kando a folder of its own for the programs it downloads, in the fixture's fake home, until the scene ends, and returns it.
func shotOwnToolsDir(t *testing.T, f *screenshotFixture) string {
	t.Helper()
	saved := media.OwnToolsDir
	media.OwnToolsDir = filepath.Join(f.home, "Library", "Application Support", "Kando", "tools")
	t.Cleanup(func() { media.OwnToolsDir = saved })
	return media.OwnToolsDir
}

// shotKitsDownloading takes ffmpeg and whisper-cli out of a's tools and gives Kando a folder of its own in the fixture's fake home, as on a computer that lacks them, for the scenes of their downloads.
func shotKitsDownloading(t *testing.T, f *screenshotFixture, a *app) {
	t.Helper()
	shotOwnToolsDir(t, f)
	tools := a.pipeline.(media.Tools)
	tools.FFmpeg, tools.Whisper = "", ""
	a.pipeline = tools
}

// shotCLITool shows Kando.app in /Applications with the kando command in /usr/local/bin when installed, or no kando a terminal finds, as the app's own look would find them. The look the window starts when it opens sees a folder of the fake home with no command, and its answer is dropped, as the scenes' updates are.
func shotCLITool(f *screenshotFixture, a *app, installed bool) {
	a.cliTool.env = func() clitool.Env {
		return clitool.Env{Exe: filepath.Join(f.home, "Kando"), Searched: []string{filepath.Join(f.home, "bin")}}
	}
	a.cliTool.known = true
	a.cliTool.status = clitool.Status{Exe: "/Applications/Kando.app/Contents/MacOS/Kando", Installable: true}
	if installed {
		a.cliTool.status.Command, a.cliTool.status.Ours = "/usr/local/bin/kando", true
	}
}

// shotBrowserExt shows Chrome and Firefox on the computer, connected or not, as the app's own look would find them. The look the window starts when it opens sees the fixture's fake home (shootSettingsAt), where no browser is, and its answer is dropped, as the scenes' updates are.
func shotBrowserExt(a *app, connected bool) {
	a.ext.known = true
	a.ext.status = browserext.Status{Exe: "/Applications/Kando.app/Contents/MacOS/Kando", Found: []string{"Chrome", "Firefox"}}
	if connected {
		a.ext.status.Connected = a.ext.status.Found
	}
}

// shotAIUsage counts a month of requests in the ledger, at a fixed date, as the meter would have: translations, proofreading and the final proofread through EveryAPI, questions to a service on this computer, and plugins' requests whose service said no usage.
func shotAIUsage(a *app) {
	a.aiusage.now = func() time.Time { return time.Date(2026, 11, 20, 12, 0, 0, 0, time.Local) }
	month := aiusage.MonthOf(a.aiusage.now())
	for range 42 {
		a.aiusage.ledger.Add(month, media.PassTranslate, "api.everyapi.ai", 3120, 1480, false)
	}
	for range 18 {
		a.aiusage.ledger.Add(month, media.PassProofread, "api.everyapi.ai", 2650, 610, false)
	}
	for range 6 {
		a.aiusage.ledger.Add(month, media.PassReview, "api.everyapi.ai", 9800, 1200, false)
	}
	for range 11 {
		a.aiusage.ledger.Add(month, media.PassChat, "localhost", 1400, 380, false)
	}
	for range 3 {
		a.aiusage.ledger.Add(month, media.PassPlugin, "api.everyapi.ai", 2200, 540, true)
	}
}

// shotSignIn shows Chrome, Firefox and Safari on the computer, with browser chosen in 浏览器登录 and its check finished with trouble, as the app's own check would leave them.
func shotSignIn(a *app, browser string, trouble media.LoginTrouble) {
	a.queue.ytdlp.browsers = []media.Browser{media.Browsers[0], media.Browsers[1], media.Browsers[5]}
	a.signin.browser, a.signin.checked, a.signin.trouble = browser, browser, trouble
}

// shotPhoneSync shows Phone Sync on, offering a code at a fixed address with a fixed certificate's fingerprint, and one iPhone paired, as a running server would show them; no server starts, so nothing listens and the picture is the same each time.
func shotPhoneSync(a *app) {
	a.phone.on, a.phone.running = true, true
	a.phone.fingerprint = psync.Fingerprint([]byte("Kando screenshot certificate"))
	a.phone.name = "MacBook Pro"
	a.phone.addrs = []netip.AddrPort{netip.MustParseAddrPort("192.168.1.20:52817")}
	a.phone.offer = phonesync.Offer{Code: "482913", State: phonesync.OfferLive}
	seen := time.Date(2026, 11, 17, 9, 41, 0, 0, time.Local)
	a.phone.devices = []phonesync.Device{{ID: "iphone", Name: "iPhone 17", Platform: psync.PlatformIOS, Paired: seen.Add(-48 * time.Hour), LastSeen: seen}}
	a.phoneSyncQR()
}

// shotEveryAPISignedIn shows Kando signed in to EveryAPI as Mika, with balance dollars left, as the account summary said, without asking any server: the sign-in's tokens are made up and nothing sends them.
func shotEveryAPISignedIn(a *app, balance float64) {
	e := &a.everyapi
	cred := newEveryapiCred()
	cred.ready(everyapi.NewSession(everyapi.Config{}, everyapi.Tokens{Key: "sk-everyapi-screenshot", Refresh: "screenshot"}, everyapi.SessionHooks{}), nil)
	e.cred, e.signedIn, e.loaded, e.name = cred, true, true, "Mika"
	e.summary, e.haveSummary = everyapi.Summary{Name: "Mika", Balance: balance, Currency: "USD"}, true
	// Opening the page asks what the account has; the shot shows what it said, so it does not ask.
	e.pageOpen = true
	a.everyapiInstall()
}

// shootSettings renders the settings window open at section, at the prototype's 880 by 640, as name.png: the library shown, the whisper models in models with turbo in use, and EveryAPI as the AI service, without a key, through the everyapi program in ~/bin (writeFakeEveryAPI). setup, when given, changes the app before it is drawn.
func shootSettings(t *testing.T, f *screenshotFixture, section, name, shown, models, turbo string, setup ...func(*app)) {
	t.Helper()
	shootSettingsAt(t, f, section, name, shown, models, turbo, false, nil, setup...)
}

// shootSettingsScrolled is shootSettings with the page scrolled to its end, when it is longer than the window.
func shootSettingsScrolled(t *testing.T, f *screenshotFixture, section, name, shown, models, turbo string, setup ...func(*app)) {
	t.Helper()
	shootSettingsAt(t, f, section, name, shown, models, turbo, true, nil, setup...)
}

// shootSettingsPressed is shootSettings with the controls of press pressed, in order, before the shot, as a row's details opened on Settings › 插件; each must be on the page. An entry "scroll-end:<selector>" scrolls that scroll view to its end instead, and one "exists:<selector>" only checks that it is on the page.
func shootSettingsPressed(t *testing.T, f *screenshotFixture, section, name, shown, models, turbo string, press []string, setup ...func(*app)) {
	t.Helper()
	shootSettingsAt(t, f, section, name, shown, models, turbo, false, press, setup...)
}

func shootSettingsAt(t *testing.T, f *screenshotFixture, section, name, shown, models, turbo string, end bool, press []string, setup ...func(*app)) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		a := newApp(f.pipeline, func(func()) {})
		a.tools.candidates = noCandidates
		a.prefs.listModels = offlineModels
		a.prefs.interfaceLanguage = testlocale.Current()
		a.edit(func(s *session) {
			s.LibraryDir = shown
			s.Libraries = []string{shown}
		})
		a.prefs.modelsDir = models
		// The 浏览器扩展 card looks for browsers in the fixture's fake home, not the machine's.
		a.ext.env = func(dir string) browserext.Env {
			return browserext.Env{GOOS: "darwin", Home: f.home, Getenv: func(string) string { return "" }, Exe: filepath.Join(f.home, "Kando"), Dir: dir}
		}
		if tools, ok := f.pipeline.(media.Tools); ok {
			tools.Translator = media.ChatTranslator{BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel}
			tools.WhisperModel = turbo
			tools.EveryAPI = writeFakeEveryAPI(t, filepath.Join(f.home, "bin"), true)
			a.syncLLM(tools.UseEveryAPICLI())
		}
		shotToolsChecked(a)
		for _, fn := range setup {
			fn(a)
		}
		a.OpenSettings(section)
		env := screenshotEnvironment()
		env.TitleBar = ns.Edges{Top: 50, Left: 72}
		// The settings window's own view, as openSettingsWindow opens it, on the test's graph.
		tt := nsgotest.Start(t, func(cx *ns.Scope) ns.Node {
			a.attach(cx)
			return a.settingsView(cx)
		}, nsgotest.Size(880, 640), nsgotest.Environment(env))
		for _, selector := range press {
			// "scroll-end:<selector>" scrolls that scroll view to its end rather than pressing it, as a dialog whose fields run past the window.
			if scroll, ok := strings.CutPrefix(selector, "scroll-end:"); ok {
				tt.Scroll(scroll, 4000)
				refresh(tt, a)
				continue
			}
			// "exists:<selector>" only checks that the selector is on the page, so a scene fails headless when what it shows is missing.
			if check, ok := strings.CutPrefix(selector, "exists:"); ok {
				if !tt.Exists(check) {
					t.Fatalf("nothing at %s; texts %q", check, tt.Texts())
				}
				continue
			}
			if !tt.Exists(selector) {
				t.Fatalf("nothing to press at %s; texts %q", selector, tt.Texts())
			}
			tt.Press(selector)
			refresh(tt, a)
		}
		if end {
			// A page that fits in the window has no end to scroll to: the headless tester's approximate text engine fits the whole of 语音识别 in it, which a platform's fonts make longer.
			if page := tt.Find("id:settings-page"); page.Content.Height > page.Frame.Size.Height {
				tt.Scroll("id:settings-page", 4000)
			}
		}
		refresh(tt, a)
		if nsgotest.Target() == "" {
			// Headless runs have no pixels, so the scenes run their steps; the pictures come from `nsgo test -target`, which saves them under .nsgo/test/screenshots/<target> (README's Screenshots).
			return
		}
		tt.Screenshot(screenshotName(env, name))
	})
}

// settingsShotFolders makes what the settings window shows in the fixture's fake home, and returns the library it names, the folder of whisper models and the model in use.
func settingsShotFolders(t *testing.T, f *screenshotFixture) (shown, models, turbo string) {
	t.Helper()
	// The window only shows the library's path, so it names one in the fixture's fake home, which the window shows as ~/Movies/….
	shown = filepath.Join(f.home, "Movies", protoLibraries[0].Name+".kando")
	// Two whisper models on disk, the recommended one in use, so the 语音识别 page shows models that can be chosen beside ones still to download.
	models = filepath.Join(f.home, "models")
	if err := os.MkdirAll(models, 0o755); err != nil {
		t.Fatal(err)
	}
	turbo = filepath.Join(models, "ggml-large-v3-turbo-q5_0.bin")
	for _, file := range []string{"ggml-base.bin", "ggml-large-v3-turbo-q5_0.bin"} {
		if err := os.WriteFile(filepath.Join(models, file), []byte("ggml"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return shown, models, turbo
}
