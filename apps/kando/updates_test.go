package main

import (
	"slices"
	"testing"
	"time"

	"kando/apps/kando/locales"
	"kando/apps/kando/settings"

	"github.com/egoist/mygo"
)

// TestAppMenuChecksForUpdates checks that 检查更新… is an enabled item of the app menu, after 设置… and before 服务, each in a section of its own.
func TestAppMenuChecksForUpdates(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	tt.Frame()
	it := findMenuItem(t, a.menuBar(), "看懂", "检查更新…")
	if it.Disabled || it.Click == nil {
		t.Fatalf("检查更新… is not a command: %+v", it)
	}
	var labels []string
	for _, m := range a.menuBar() {
		if m.Label != "看懂" {
			continue
		}
		for _, it := range m.Submenu {
			if it.Type == mygo.MenuItemSeparator {
				labels = append(labels, "-")
			} else {
				labels = append(labels, it.Label)
			}
		}
	}
	want := []string{"关于看懂", "-", "设置…", "-", "检查更新…", "-", "服务"}
	if len(labels) < len(want) || !slices.Equal(labels[:len(want)], want) {
		t.Fatalf("app menu %q, want it to start %q", labels, want)
	}
}

// fakeUpdater stands in for the updater plugin, which only main uses: state is what it reports, and it records what the app asked of it.
type fakeUpdater struct {
	state     settings.Updates
	checks    int
	setChecks []bool
	setDowns  []bool
	changed   func()
}

func useFakeUpdater(a *app, state settings.Updates) *fakeUpdater {
	f := &fakeUpdater{state: state}
	a.updates.read = func() settings.Updates { return f.state }
	a.updates.check = func() { f.checks++ }
	a.updates.setChecks = func(on bool) { f.setChecks = append(f.setChecks, on); f.state.AutoCheck = on }
	a.updates.setDownloads = func(on bool) { f.setDowns = append(f.setDowns, on); f.state.AutoDownload = on }
	a.updates.follow = func(fn func()) { f.changed = fn }
	return f
}

// TestSettingsChecksForUpdates checks that 通用's 检查更新… asks the updater to check, and that a copy that cannot update itself, as in development, shows no switches, as the update window says why instead.
func TestSettingsChecksForUpdates(t *testing.T) {
	a, _, _ := newTestApp(&fakePipeline{})
	f := useFakeUpdater(a, settings.Updates{})
	a.updatesFollow()
	tt := settingsOpenAt(a, nil, settings.SectionGeneral)
	if err := tt.Click("检查更新…"); err != nil {
		t.Fatal(err)
	}
	if f.checks != 1 {
		t.Fatalf("checked %d times", f.checks)
	}
	for _, text := range []string{"自动检查更新", "自动下载并安装更新"} {
		if tt.HasText(text) {
			t.Fatalf("%q shows where Kando cannot update itself", text)
		}
	}
}

// TestSettingsUpdateSwitches checks the switches of a copy that can update itself: they change the updater's choices, the download switch goes away with the checks, and what the updater reports after a check, the newer version or when it checked, shows under 版本.
func TestSettingsUpdateSwitches(t *testing.T) {
	a, _, updates := newTestApp(&fakePipeline{})
	f := useFakeUpdater(a, settings.Updates{Enabled: true, AutoCheck: true, Available: "0.5.0"})
	a.updatesFollow()
	tt := settingsOpenAt(a, nil, settings.SectionGeneral)
	for _, text := range []string{"有新版本 0.5.0", "自动检查更新", "自动下载并安装更新", "新版本在下次打开看懂时生效。"} {
		if !tt.HasText(text) {
			t.Fatalf("no %q: %q", text, tt.Texts())
		}
	}
	if err := tt.Click("自动下载并安装更新"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !slices.Equal(f.setDowns, []bool{true}) || !a.Updates().AutoDownload {
		t.Fatalf("download switch set %v, shown %+v", f.setDowns, a.Updates())
	}
	if err := tt.Click("自动检查更新"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !slices.Equal(f.setChecks, []bool{false}) || tt.HasText("自动下载并安装更新") {
		t.Fatalf("checks set %v; download switch shown: %t", f.setChecks, tt.HasText("自动下载并安装更新"))
	}
	f.state.Available = ""
	f.state.LastCheck = time.Date(2026, 11, 10, 9, 30, 0, 0, time.Local)
	// The change reaches the app as a background job's report does; the window's own lookups may be queued before it.
	f.changed()
	for len(updates) > 0 {
		(<-updates)()
	}
	tt.Frame()
	if want := locales.SettingsGeneralLastCheck(f.state.LastCheck); !tt.HasText(want) || tt.HasText("有新版本 0.5.0") {
		t.Fatalf("no %q after the check: %q", want, tt.Texts())
	}
}
