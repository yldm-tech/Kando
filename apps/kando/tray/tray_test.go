package tray

import (
	"bytes"
	"image"
	"image/png"
	"strings"
	"testing"

	"kando/apps/kando/locales"
	"kando/packages/core"

	ns "github.com/nsgo-dev/nsgo/core"
)

func TestOverallCountsEachStageAsItsPartOfTheJob(t *testing.T) {
	for _, tc := range []struct {
		stage core.Stage
		p     core.Progress
		want  int
	}{
		{core.StageQueued, core.Fraction(0.5), 0},
		{core.StageDownloading, core.Progress{}, 0},
		{core.StageDownloading, core.Fraction(0.5), 10},
		{core.StageTranscribing, core.Progress{}, 20},
		{core.StageTranscribing, core.Fraction(1), 50},
		{core.StageTranslating, core.Fraction(0.3), 62},
		{core.StageExporting, core.Fraction(0.5), 95},
		{core.StageComplete, core.Progress{}, 100},
		{core.StageFailed, core.Fraction(0.9), 0},
	} {
		if got := Overall(tc.stage, tc.p); got != tc.want {
			t.Errorf("Overall(%s, %v) = %d, want %d", tc.stage, tc.p.Fraction, got, tc.want)
		}
	}
}

func TestTitleAndTooltipFollowTheQueue(t *testing.T) {
	m := locales.In("zh-Hans")
	running := Status{Running: &Job{Title: "Morning Market in Kyoto", Stage: core.StageTranslating, Progress: core.Fraction(0.3)}, Waiting: 2}
	if got := Title(m, running); got != "62%" {
		t.Fatalf("title while a job runs = %q, want 62%%", got)
	}
	if got := Tooltip(m, running); got != "看懂：Morning Market in Kyoto 62%" {
		t.Fatalf("tooltip while a job runs = %q", got)
	}
	paused := running
	paused.Paused = true
	if got := Title(m, paused); got != "已暂停" {
		t.Fatalf("title while paused = %q", got)
	}
	if got := Tooltip(m, paused); got != "看懂：队列已暂停" {
		t.Fatalf("tooltip while paused = %q", got)
	}
	if got := Title(m, Status{}); got != "" {
		t.Fatalf("title while idle = %q, want none", got)
	}
	if got := Tooltip(m, Status{}); got != "看懂" {
		t.Fatalf("tooltip while idle = %q, want the app's name", got)
	}
	long := Status{Running: &Job{Title: strings.Repeat("长", 60), Stage: core.StageDownloading}}
	if got := Tooltip(m, long); !strings.Contains(got, strings.Repeat("长", titleRunes-1)+"…") || strings.Contains(got, strings.Repeat("长", titleRunes)) {
		t.Fatalf("a long title is not cut: %q", got)
	}
}

// menuLabels is the menu's items as lines: a separator is "-", a disabled item is in brackets and a checked one starts with ✓.
func menuLabels(items []ns.MenuItem) []string {
	var lines []string
	for _, it := range items {
		switch {
		case it.Separator:
			lines = append(lines, "-")
		case it.Disabled:
			lines = append(lines, "["+it.Label+"]")
		case it.Checked:
			lines = append(lines, "✓"+it.Label)
		default:
			lines = append(lines, it.Label)
		}
	}
	return lines
}

func TestMenuTellsTheQueueAndOffersItsCommands(t *testing.T) {
	m := locales.In("zh-Hans")
	var ran []string
	act := Actions{
		Pause:    func() { ran = append(ran, "pause") },
		Open:     func() { ran = append(ran, "open") },
		Settings: func() { ran = append(ran, "settings") },
		Quit:     func() { ran = append(ran, "quit") },
	}
	s := Status{Running: &Job{Title: "Talk", Stage: core.StageTranscribing, Progress: core.Fraction(0.5)}, Waiting: 3, Hidden: true}
	items := Menu(m, s, act)
	want := []string{"[窗口已关，在后台处理]", "[Talk — 生成字幕中 · 总进度 35%]", "[3 个在排队]", "-", "暂停处理队列", "-", "打开看懂", "设置…", "退出看懂"}
	if got := menuLabels(items); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("menu = %q, want %q", got, want)
	}
	for _, it := range items {
		if it.OnSelect != nil {
			it.OnSelect()
		}
	}
	if strings.Join(ran, ",") != "pause,open,settings,quit" {
		t.Fatalf("the commands ran %q", ran)
	}

	paused := menuLabels(Menu(m, Status{Paused: true}, act))
	if want := []string{"[队列已暂停]", "[队列是空的]", "-", "✓暂停处理队列", "-", "打开看懂", "设置…", "退出看懂"}; strings.Join(paused, "|") != strings.Join(want, "|") {
		t.Fatalf("paused menu = %q, want %q", paused, want)
	}
	if idle := menuLabels(Menu(m, Status{Waiting: 1}, act)); idle[0] != "[空闲]" || idle[1] != "[1 个在排队]" {
		t.Fatalf("idle menu = %q", idle)
	}
	if running := menuLabels(Menu(m, Status{Running: s.Running}, act)); running[0] != "[正在处理]" {
		t.Fatalf("menu with the window open = %q", running)
	}
	if en := menuLabels(Menu(locales.In("en"), Status{Waiting: 1}, act)); en[1] != "[1 video waiting]" {
		t.Fatalf("English queue line = %q", en[1])
	}
}

// alphaAt is how opaque the icon is at pixel (x, y).
func alphaAt(t *testing.T, data []byte, x, y int) uint8 {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != image.Rect(0, 0, iconSize, iconSize) {
		t.Fatalf("icon is %v, want %d pixels square", img.Bounds(), iconSize)
	}
	_, _, _, a := img.At(x, y).RGBA()
	return uint8(a >> 8)
}

func TestIconFillsTheRingAsTheJobGoes(t *testing.T) {
	// Points on the ring: right of the center a quarter of the way round, and left of it three quarters.
	r := ringRadius
	right, left := [2]int{int(iconSize/2 + r), iconSize / 2}, [2]int{int(iconSize/2 - r), iconSize / 2}
	half := Icon(Status{Running: &Job{Stage: core.StageTranscribing, Progress: core.Fraction(1)}})
	if a := alphaAt(t, half, right[0], right[1]); a < 200 {
		t.Fatalf("at half way the ring's right side has alpha %d, want it filled", a)
	}
	if a := alphaAt(t, half, left[0], left[1]); a > 80 || a == 0 {
		t.Fatalf("at half way the ring's left side has alpha %d, want only the faint track", a)
	}
	for name, s := range map[string]Status{
		"idle":   {},
		"paused": {Paused: true, Running: &Job{Stage: core.StageExporting, Progress: core.Fraction(1)}},
	} {
		icon := Icon(s)
		if a := alphaAt(t, icon, right[0], right[1]); a > 80 || a == 0 {
			t.Fatalf("%s: the ring has alpha %d, want only the faint track", name, a)
		}
		if a := alphaAt(t, icon, iconSize/2, iconSize/2); a != 0 {
			t.Fatalf("%s: the center has alpha %d, want it empty", name, a)
		}
	}
}
