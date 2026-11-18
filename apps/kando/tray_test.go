package main

import (
	"encoding/json"
	"strings"
	"testing"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/settings"
	"kando/packages/core"

	ns "github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
)

// fakeTrayIcon records what the app told the status icon.
type fakeTrayIcon struct {
	opts           nsdesktop.TrayOptions
	title, tooltip string
	icons          int
	removed        bool
}

func (f *fakeTrayIcon) SetIcon([]byte, bool)   { f.icons++ }
func (f *fakeTrayIcon) SetTitle(s string)      { f.title = s }
func (f *fakeTrayIcon) SetTooltip(s string)    { f.tooltip = s }
func (f *fakeTrayIcon) Remove()                { f.removed = true }
func (f *fakeTrayIcon) menu() []ns.MenuItem    { return f.opts.Menu() }
func (f *fakeTrayIcon) menuLabel(i int) string { return f.menu()[i].Label }

// item is the menu's item labelled label.
func (f *fakeTrayIcon) item(t *testing.T, label string) ns.MenuItem {
	t.Helper()
	for _, it := range f.menu() {
		if it.Label == label {
			return it
		}
	}
	t.Fatalf("no %q in the icon's menu", label)
	return ns.MenuItem{}
}

// fakeTraySystem stands in for NSGo's tray and launch at login, as macOS has them: it keeps the icons the app adds, the last first, and what the app asked of launch at login.
type fakeTraySystem struct {
	icons []*fakeTrayIcon
	login []bool
}

func (f *fakeTraySystem) system(loginOn bool) traySystem {
	return traySystem{
		open: func(opts nsdesktop.TrayOptions) trayIcon {
			icon := &fakeTrayIcon{opts: opts, title: opts.Title, tooltip: opts.Tooltip}
			f.icons = append([]*fakeTrayIcon{icon}, f.icons...)
			return icon
		},
		template:  true,
		readLogin: func(done func(bool)) { done(loginOn) },
		setLogin:  func(on bool) { f.login = append(f.login, on) },
	}
}

// TestPausingTheQueueStopsAndHoldsTheJobs checks that pausing the queue stops the running job, which goes back to the head of the queue as waiting, that its card and the one behind it say the queue is paused and nothing starts while it is, and that resuming starts that job again before the other.
func TestPausingTheQueueStopsAndHoldsTheJobs(t *testing.T) {
	g := newGatePipeline()
	a, tt, updates := newTestApp(t, g)
	t.Cleanup(a.stop)
	if err := a.Enqueue("https://example.com/a", "https://example.com/b"); err != nil {
		t.Fatal(err)
	}
	nextStarted(t, g, updates)
	ids := jobIDs(a)
	a.queueSetPaused(true)
	waitUntil(t, tt, a, updates, func() bool { return !a.running() })
	if job := stageOf(t, a, ids["https://example.com/a"]); job.Stage != core.StageQueued || job.Err != nil {
		t.Fatalf("a after the pause %+v, want it waiting again", job)
	}
	noneStarted(t, g, updates)
	refresh(tt, a)
	if !hasText(tt, testlocale.Messages().FilmsQueuePaused()) {
		t.Fatalf("the waiting cards do not say the queue is paused: %q", tt.Texts())
	}
	if a.queue.order[0] != ids["https://example.com/a"] {
		t.Fatalf("queue order %v, want a back at its head", a.queue.order)
	}
	a.queueSetPaused(false)
	if got := nextStarted(t, g, updates); got != "https://example.com/a" {
		t.Fatalf("after resuming, %s started, want a again", got)
	}
	g.release <- struct{}{}
	if got := nextStarted(t, g, updates); got != "https://example.com/b" {
		t.Fatalf("after a, %s started", got)
	}
	g.release <- struct{}{}
	settle(t, a, tt, updates)
	for _, source := range []string{"https://example.com/a", "https://example.com/b"} {
		if job := stageOf(t, a, ids[source]); job.Stage != core.StageComplete {
			t.Fatalf("%s ended %+v", source, job)
		}
	}
}

// TestPausingTheQueueFromTheMenuBar checks that 文件 › 暂停处理队列 pauses the queue with a ✓ and resumes it, and that a job added while it is paused waits.
func TestPausingTheQueueFromTheMenuBar(t *testing.T) {
	g := newGatePipeline()
	a, tt, updates := newTestApp(t, g)
	t.Cleanup(a.stop)
	m := testlocale.Messages()
	item := func() nsdesktop.MenuItem {
		return findMenuItem(t, a.menuBar(a.cx), groupTitle(a.cx, groupFile), m.QueuePause())
	}
	if it := item(); !it.Checkable || it.Checked {
		t.Fatalf("暂停处理队列 checkable %v checked %v before any pause", it.Checkable, it.Checked)
	}
	choose(tt, a, item())
	if !a.queue.paused || !item().Checked {
		t.Fatalf("paused %v, checked %v after choosing it", a.queue.paused, item().Checked)
	}
	if err := a.Enqueue("https://example.com/a"); err != nil {
		t.Fatal(err)
	}
	noneStarted(t, g, updates)
	choose(tt, a, item())
	if got := nextStarted(t, g, updates); got != "https://example.com/a" || a.queue.paused {
		t.Fatalf("after resuming from the menu bar %q started, paused %v", got, a.queue.paused)
	}
	g.release <- struct{}{}
	settle(t, a, tt, updates)
}

// TestTrayIconFollowsTheQueue checks the status icon: it shows once the system is there, with the running job's progress beside it, in its tooltip and in its menu, says when the queue is paused, its menu pauses the queue, opens the settings and quits through the quit that asks first, and the switch of Settings › 通用 takes it away and brings it back.
func TestTrayIconFollowsTheQueue(t *testing.T) {
	g := newGatePipeline()
	a, tt, updates := newTestApp(t, g)
	t.Cleanup(a.stop)
	m := testlocale.Messages()
	sys := &fakeTraySystem{}
	a.trayUseSystem(sys.system(false))
	if len(sys.icons) != 1 {
		t.Fatalf("%d icons once the system is there, want 1", len(sys.icons))
	}
	icon := sys.icons[0]
	if !icon.opts.Template || len(icon.opts.Icon) == 0 || icon.title != "" || icon.tooltip != m.AppName() {
		t.Fatalf("idle icon: template %v, %d bytes, title %q, tooltip %q", icon.opts.Template, len(icon.opts.Icon), icon.title, icon.tooltip)
	}
	if err := a.Enqueue("https://example.com/a", "https://example.com/b"); err != nil {
		t.Fatal(err)
	}
	nextStarted(t, g, updates)
	refresh(tt, a)
	if icon.title != "0%" || !strings.HasPrefix(icon.tooltip, m.AppName()) || icon.icons == 0 {
		t.Fatalf("icon while a job runs: title %q, tooltip %q, redrawn %d times", icon.title, icon.tooltip, icon.icons)
	}
	if got := icon.menuLabel(0); got != m.TrayStateRunning() {
		t.Fatalf("menu says %q while a job runs", got)
	}
	if got := icon.menuLabel(2); got != m.TrayQueue(1) {
		t.Fatalf("menu's queue line %q, want one waiting", got)
	}

	// Pausing from the icon's menu.
	icon.item(t, m.QueuePause()).OnSelect()
	waitUntil(t, tt, a, updates, func() bool { return !a.running() })
	if !a.queue.paused || icon.title != m.TrayPaused() || icon.menuLabel(0) != m.TrayStatePaused() || !icon.item(t, m.QueuePause()).Checked {
		t.Fatalf("after pausing from the icon: paused %v, title %q, menu %q", a.queue.paused, icon.title, icon.menuLabel(0))
	}

	// 设置… opens 通用.
	icon.item(t, m.AppSettings()).OnSelect()
	refresh(tt, a)
	wantSettings(t, a, settings.SectionGeneral)

	// The switch takes the icon away and brings a new one back.
	tt.Press("settings-tray-icon")
	if a.tray.icon || !icon.removed || a.tray.shown != nil {
		t.Fatalf("switched off: setting %v, removed %v", a.tray.icon, icon.removed)
	}
	tt.Press("settings-tray-icon")
	if !a.tray.icon || len(sys.icons) != 2 || sys.icons[0].title != m.TrayPaused() {
		t.Fatalf("switched on again: setting %v, %d icons", a.tray.icon, len(sys.icons))
	}

	// 退出看懂 quits through the app's own quit, which goes ahead as nothing runs.
	a.queueSetPaused(false)
	nextStarted(t, g, updates)
	refresh(tt, a)
	sys.icons[0].item(t, m.MenuQuit()).OnSelect()
	refresh(tt, a)
	if !a.queue.quitAsk || tt.Quits != 0 {
		t.Fatalf("退出看懂 while a job runs: asked %v, quits %d", a.queue.quitAsk, tt.Quits)
	}
	a.queue.quitAsk = false
	refresh(tt, a)
}

// TestClosingTheWindowElsewhereKeepsKandoRunning checks that on Linux and Windows, while 关掉窗口后继续在后台运行 is on, as it is by default, the main window's close button only puts the window away, the job running on, and launching Kando again brings it back.
func TestClosingTheWindowElsewhereKeepsKandoRunning(t *testing.T) {
	setWindowHidesOnClose(t, false)
	g := newGatePipeline()
	a, tt, updates := newTestApp(t, g)
	t.Cleanup(a.stop)
	if !a.tray.keepRunning {
		t.Fatal("closing the window does not keep Kando running by default")
	}
	if err := a.Enqueue("https://example.com/a"); err != nil {
		t.Fatal(err)
	}
	nextStarted(t, g, updates)
	e := &ns.WindowClosing{}
	a.windowClosing(e)
	tt.Settle()
	if !windowClosingCanceled(e) || !a.window.hidden || tt.Quits != 0 || !a.running() {
		t.Fatalf("close button: canceled %v, hidden %v, quits %d, job running %v", windowClosingCanceled(e), a.window.hidden, tt.Quits, a.running())
	}
	a.windowShowMain()
	if a.window.hidden {
		t.Fatal("launching Kando again left the window hidden")
	}
	g.release <- struct{}{}
	settle(t, a, tt, updates)
}

// TestBackgroundSettings checks Settings › 通用 › 后台运行 on each system: on macOS 登录电脑时打开看懂 reads and sets what the system does and closing the window has no switch, as it never quits there; on Windows and Linux there is no login switch yet and 关掉窗口后继续在后台运行 is one; the status icon's switch names where each system shows it; and the switches are kept in settings.json.
func TestBackgroundSettings(t *testing.T) {
	m := testlocale.Messages()
	t.Run("macOS", func(t *testing.T) {
		a, tt, _ := newTestApp(t, &fakePipeline{})
		sys := &fakeTraySystem{}
		a.trayUseSystem(sys.system(true))
		settingsOpenAt(a, tt, settings.SectionGeneral)
		if !hasText(tt, m.SettingsGeneralBackground()) || !hasText(tt, m.SettingsGeneralTrayIcon("macOS")) {
			t.Fatalf("no 后台运行 card with the menu bar's switch: %q", tt.Texts())
		}
		if tt.Exists("id:settings-keep-running") {
			t.Fatal("closing the window has a switch on macOS")
		}
		if !a.Background().LaunchAtLogin {
			t.Fatal("the switch does not read that the system opens Kando at login")
		}
		tt.Press("settings-launch-at-login")
		if a.Background().LaunchAtLogin || len(sys.login) != 1 || sys.login[0] {
			t.Fatalf("switched off: on %v, asked the system %v", a.Background().LaunchAtLogin, sys.login)
		}
	})
	for name, os := range map[string]ns.OS{"Windows": ns.OSWindows, "Linux": ns.OSLinux} {
		t.Run(name, func(t *testing.T) {
			usePlatform(t, os)
			a, tt, _ := newTestApp(t, &fakePipeline{})
			settingsOpenAt(a, tt, settings.SectionGeneral)
			if tt.Exists("id:settings-launch-at-login") {
				t.Fatal("login switch offered where the system cannot open Kando at login")
			}
			if !hasText(tt, m.SettingsGeneralTrayIcon(platformName())) {
				t.Fatalf("the icon's switch does not name %s's place: %q", platformName(), tt.Texts())
			}
			tt.Press("settings-keep-running")
			tt.Press("settings-tray-icon")
			if a.tray.keepRunning || a.tray.icon {
				t.Fatalf("switched off: keep running %v, icon %v", a.tray.keepRunning, a.tray.icon)
			}
			var saved savedSettings
			a.areasSave(&saved)
			raw, err := json.Marshal(saved)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), `"trayIcon":false`) || !strings.Contains(string(raw), `"keepRunningInBackground":false`) {
				t.Fatalf("settings.json keeps %s", raw)
			}
			back := newApp(&fakePipeline{}, nil)
			back.areasRestore(saved)
			if back.tray.keepRunning || back.tray.icon {
				t.Fatalf("restored: keep running %v, icon %v", back.tray.keepRunning, back.tray.icon)
			}
			if fresh := newApp(&fakePipeline{}, nil); !fresh.tray.keepRunning || !fresh.tray.icon {
				t.Fatal("a fresh settings.json does not keep Kando running with its icon")
			}
		})
	}
}
