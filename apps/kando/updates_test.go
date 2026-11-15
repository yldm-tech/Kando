package main

import (
	"os"
	"slices"
	"strings"
	"testing"

	"kando/apps/kando/settings"

	ns "github.com/nsgo-dev/nsgo/core"
)

// TestAppMenuChecksForUpdates checks that 检查更新… is an enabled item of the app menu, after 设置… and before the platform's items, each in a section of its own.
func TestAppMenuChecksForUpdates(t *testing.T) {
	a, _, _ := newTestApp(t, &fakePipeline{})
	it := findMenuItem(t, a.menuBar(a.cx), "看懂", "检查更新…")
	if it.Disabled || it.OnSelect == nil {
		t.Fatalf("检查更新… is not a command: %+v", it)
	}
	var labels []string
	for _, m := range a.menuBar(a.cx) {
		if m.Label != "看懂" {
			continue
		}
		for _, it := range m.Items {
			if it.Separator {
				labels = append(labels, "-")
			} else {
				labels = append(labels, it.Label)
			}
		}
	}
	want := []string{"关于看懂", "-", "设置…", "-", "检查更新…", "-"}
	if len(labels) < len(want) || !slices.Equal(labels[:len(want)], want) {
		t.Fatalf("app menu %q, want it to start %q", labels, want)
	}
}

// TestSettingsChecksForUpdates checks that 通用's 检查更新… and the app menu's ask the updater to check, under the app's scope.
func TestSettingsChecksForUpdates(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	var checked []*ns.Scope
	a.updates.check = func(cx *ns.Scope) { checked = append(checked, cx) }
	tt = settingsOpenAt(a, tt, settings.SectionGeneral)
	tt.Press("id:settings-check-updates")
	choose(tt, a, findMenuItem(t, a.menuBar(a.cx), "看懂", "检查更新…"))
	if len(checked) != 2 || checked[0] != a.cx || checked[1] != a.cx {
		t.Fatalf("checked %d times, under %v, want twice under the app's scope", len(checked), checked)
	}
}

// TestChangelogHasTheVersion checks that CHANGELOG.md has notes for the version in nsgo.yaml: the lines under the first "## " heading whose first word is the version, with or without a v and in brackets or not, up to the next "## " heading. A release build signs the update manifests with those notes, which the update window shows, and fails when they are missing, so a version bumped without them fails here rather than in the Release workflow. The Release workflow writes them with .github/scripts/changelog.sh.
func TestChangelogHasTheVersion(t *testing.T) {
	data, err := os.ReadFile("nsgo.yaml")
	if err != nil {
		t.Fatal(err)
	}
	version := ""
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, "version:"); ok {
			version = strings.Trim(strings.TrimSpace(v), `"'`)
			break
		}
	}
	if version == "" {
		t.Fatal("nsgo.yaml names no version")
	}
	changelog, err := os.ReadFile("CHANGELOG.md")
	if err != nil {
		t.Fatalf("the update's release notes come from CHANGELOG.md: %v", err)
	}
	var notes []string
	in, found := false, false
	for _, line := range strings.Split(strings.ReplaceAll(string(changelog), "\r\n", "\n"), "\n") {
		if rest, ok := strings.CutPrefix(line, "## "); ok {
			if in {
				break
			}
			fields := strings.Fields(rest)
			if len(fields) > 0 {
				v := fields[0]
				if inner, ok := strings.CutPrefix(v, "["); ok {
					v, _, _ = strings.Cut(inner, "]")
				}
				in = strings.TrimPrefix(v, "v") == version
				found = found || in
			}
			continue
		}
		if in {
			notes = append(notes, line)
		}
	}
	if !found {
		t.Fatalf("CHANGELOG.md has no \"## %s\" section for nsgo.yaml's version", version)
	}
	if strings.TrimSpace(strings.Join(notes, "\n")) == "" {
		t.Fatalf("CHANGELOG.md's section for %s is empty", version)
	}
}
