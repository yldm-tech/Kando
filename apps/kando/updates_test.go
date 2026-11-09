package main

import (
	"slices"
	"testing"

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
