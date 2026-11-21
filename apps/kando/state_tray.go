package main

import (
	"kando/apps/kando/live"
	"kando/apps/kando/settings"
	"kando/apps/kando/tray"
	"kando/packages/core"

	ns "github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
)

// Kando with its window closed: the status icon in the menu bar, the Windows notification area or the Linux system tray, which shows how far the running job has gone and offers to pause the queue, open Kando, its settings or quit (package tray); whether closing the main window on Windows and Linux keeps Kando running, as it always does on macOS (state_window.go); and, on macOS, whether the system opens Kando at login, without a window. Settings › 通用 › 后台运行 switches each.

// trayIcon is what the app does with the status icon: NSGo's desktop.Tray, whose methods do nothing on a nil one, or a test's stand-in.
type trayIcon interface {
	SetIcon(png []byte, template bool)
	SetTitle(title string)
	SetTooltip(tooltip string)
	Remove()
}

// trayUI is the status icon's state and the background settings.
type trayUI struct {
	// icon is whether the status icon shows (Settings › 通用); keepRunning whether closing the main window on Windows and Linux only puts it away while Kando runs on. Both are on by default.
	icon, keepRunning bool
	// login is whether the system opens Kando at login, as the system last said or the switch last set it, on macOS.
	login bool
	// shown is the icon while it shows, nil otherwise; title, tooltip and ring are what it was last told, so it is told only what changed, ring -1 before it was drawn.
	shown          trayIcon
	title, tooltip string
	ring           int
	// sys is how the area reaches the system. Every field is nil until main's window is ready (trayHooks), and in tests that do not set them, so nothing shows then.
	sys traySystem
}

// traySystem is how the status icon and launch at login reach the system.
type traySystem struct {
	// open adds the status icon; template says whether the icon is one macOS draws in the menu bar's colors (tray.Icon), which only macOS takes, the others showing the app's own icon.
	open     func(opts nsdesktop.TrayOptions) trayIcon
	template bool
	// readLogin asks the system whether it opens Kando at login; setLogin has it do so or no more. Both are nil where Kando cannot set it.
	readLogin func(done func(on bool))
	setLogin  func(on bool)
}

func newTrayUI() trayUI { return trayUI{icon: true, keepRunning: true, ring: -1} }

// savedTray is the background settings' part of settings.json: TrayIcon and KeepRunning are false when the user turned them off, and nil while they are on, as they are by default. Whether Kando opens at login is the system's to keep.
type savedTray struct {
	TrayIcon    *bool `json:"trayIcon,omitempty"`
	KeepRunning *bool `json:"keepRunningInBackground,omitempty"`
}

func (s *savedTray) clean() {}

// trayOff is the saved form of a setting that is on by default: nil while on.
func trayOff(on bool) *bool {
	if on {
		return nil
	}
	off := false
	return &off
}

// trayHooks: the background settings are kept in settings.json. Once the window is ready the status icon shows, when the user wants it, and follows every change of the queue; on macOS the system is asked whether it opens Kando at login, and a run it opened at login puts the main window away at once, so Kando runs in the menu bar.
func (a *app) trayHooks() areaHooks {
	return areaHooks{
		save: func(saved *savedSettings) {
			saved.savedTray = savedTray{TrayIcon: trayOff(a.tray.icon), KeepRunning: trayOff(a.tray.keepRunning)}
		},
		restore: func(saved savedSettings) {
			if saved.TrayIcon != nil {
				a.tray.icon = *saved.TrayIcon
			}
			if saved.KeepRunning != nil {
				a.tray.keepRunning = *saved.KeepRunning
			}
		},
		ready: func() {
			a.trayUseSystem(a.traySystemDesktop(a.cx))
			a.trayHideAtLogin()
		},
		afterChange: a.trayFollow,
	}
}

// trayHideAtLogin keeps the main window, which nsgo.Run opens hidden (nsgo.StartHidden in main), away in a run the system opened at login, so Kando runs in the menu bar without the window showing for a moment, and shows it in any other run.
func (a *app) trayHideAtLogin() {
	if nsdesktop.LaunchedAtLogin(a.cx) {
		a.windowHide()
		return
	}
	a.windowShowMain()
}

// traySystemDesktop reaches the system through NSGo, under the app's scope cx, which keeps the icon as long as the app runs. The menu bar's icon on macOS is drawn by tray.Icon; Windows shows the executable's own icon and Linux the app's.
func (a *app) traySystemDesktop(cx *ns.Scope) traySystem {
	mac := cx.Env().OS() == ns.OSMacOS
	s := traySystem{
		open: func(opts nsdesktop.TrayOptions) trayIcon {
			var t *nsdesktop.Tray
			cx.Run(func() { t = nsdesktop.NewTray(cx, opts) })
			if t == nil {
				return nil
			}
			return t
		},
		template: mac,
	}
	// 登录电脑时打开看懂 is the system's login item on macOS, the user's Run key on Windows and an XDG autostart entry on Linux.
	s.readLogin = func(done func(bool)) { nsdesktop.LaunchAtLogin(cx, done) }
	s.setLogin = func(on bool) { nsdesktop.SetLaunchAtLogin(cx, on) }
	return s
}

// trayUseSystem has the area reach the system through s from now on: the icon shows when the user wants it, and the switch of launch at login reads what the system says.
func (a *app) trayUseSystem(s traySystem) {
	a.tray.sys = s
	a.trayShowIcon()
	a.trayReadLogin()
}

// trayReadLogin has the switch of launch at login show what the system says it does.
func (a *app) trayReadLogin() {
	if a.tray.sys.readLogin != nil {
		a.tray.sys.readLogin(live.On(a, func(on bool) { a.tray.login = on }))
	}
}

// trayKeepsRunning says closing the main window on Windows and Linux only puts it away (windowClosing): while 关掉窗口后继续在后台运行 is on and the status icon shows, which brings the window back. With the icon off, a window put away would leave Kando running with nothing to show it.
func (a *app) trayKeepsRunning() bool {
	return a.tray.keepRunning && a.tray.shown != nil
}

// trayShowIcon adds the status icon when the user wants it and it does not show yet, or takes it away when they no longer do.
func (a *app) trayShowIcon() {
	switch {
	case a.tray.icon && a.tray.shown == nil && a.tray.sys.open != nil:
		s := a.trayStatus()
		m := a.messages()
		opts := nsdesktop.TrayOptions{Template: a.tray.sys.template, Title: tray.Title(m, s), Tooltip: tray.Tooltip(m, s), Menu: a.trayMenu}
		a.tray.ring = -1
		if opts.Template {
			opts.Icon, a.tray.ring = tray.Icon(s), trayRing(s)
		} else if a.cx != nil && a.cx.Env().OS() == ns.OSLinux {
			opts.Icon = appIconPNG
		}
		a.tray.shown = a.tray.sys.open(opts)
		a.tray.title, a.tray.tooltip = opts.Title, opts.Tooltip
	case !a.tray.icon && a.tray.shown != nil:
		a.tray.shown.Remove()
		a.tray.shown = nil
	}
}

// trayRing is the percent the menu bar icon's ring is drawn at for s, -1 while it is empty.
func trayRing(s tray.Status) int {
	if s.Running == nil || s.Paused {
		return -1
	}
	return tray.Overall(s.Running.Stage, s.Running.Progress)
}

// trayFollow tells the status icon what changed since it was last told: its text, its tooltip and, in the menu bar, its ring. It runs after every change of the app's state (areaHooks.afterChange) and changes nothing a binding reads.
func (a *app) trayFollow() bool {
	if a.tray.shown == nil {
		return false
	}
	s := a.trayStatus()
	m := a.messages()
	if title := tray.Title(m, s); title != a.tray.title {
		a.tray.title = title
		a.tray.shown.SetTitle(title)
	}
	if tooltip := tray.Tooltip(m, s); tooltip != a.tray.tooltip {
		a.tray.tooltip = tooltip
		a.tray.shown.SetTooltip(tooltip)
	}
	if ring := trayRing(s); a.tray.sys.template && ring != a.tray.ring {
		a.tray.ring = ring
		a.tray.shown.SetIcon(tray.Icon(s), true)
	}
	return false
}

// trayStatus is the queue as the status icon tells of it. A job of a locked library is not told of, as the app's icon leaves it out too (queueOutstanding).
func (a *app) trayStatus() tray.Status {
	s := tray.Status{Paused: a.queue.paused, Hidden: a.window.hidden}
	for _, job := range a.lockVisible(a.library.Jobs) {
		switch {
		case job.ID == a.queue.active && !job.Stage.Done():
			s.Running = &tray.Job{Title: jobName(job), Stage: job.Stage, Progress: job.Progress}
		case job.ID != a.queue.active && job.Stage == core.StageQueued && !queueHeld(job) && a.queueCanRun(job):
			s.Waiting++
		}
	}
	return s
}

// trayMenu is the status icon's menu, built as it opens.
func (a *app) trayMenu() []ns.MenuItem {
	return tray.Menu(a.messages(), a.trayStatus(), tray.Actions{
		Pause: live.Do(a, a.queueTogglePause),
		Open:  live.Do(a, a.remoteRaise),
		Settings: live.Do(a, func() {
			a.remoteRaise()
			a.OpenSettings(settings.SectionGeneral)
		}),
		Quit: live.Do(a, a.requestQuit),
	})
}

// The background settings' part of settings.App, for Settings › 通用 › 后台运行.

func (a *app) Background() settings.Background {
	return settings.Background{LaunchAtLogin: a.tray.login, CanLaunchAtLogin: a.tray.sys.setLogin != nil, KeepRunning: a.tray.keepRunning, TrayIcon: a.tray.icon}
}

func (a *app) SetLaunchAtLogin(on bool) {
	if a.tray.sys.setLogin == nil {
		return
	}
	a.tray.login = on
	a.tray.sys.setLogin(on)
	// The system may refuse, as macOS does for an app outside a bundle or one waiting for approval in Login Items, and NSGo only logs it, so the switch reads back what it did.
	a.trayReadLogin()
}

func (a *app) SetKeepRunning(on bool) {
	a.tray.keepRunning = on
}

func (a *app) SetTrayIcon(on bool) {
	a.tray.icon = on
	a.trayShowIcon()
}
