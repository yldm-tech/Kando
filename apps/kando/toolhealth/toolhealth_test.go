package toolhealth

import (
	"slices"
	"testing"

	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/packages/media"
)

func TestMain(m *testing.M) {
	testlocale.Pin(locales.Default)
	m.Run()
}

var (
	ready  = media.Health{State: media.HealthReady, Version: "9.0.2"}
	broken = media.Health{State: media.HealthBroken, Problem: media.ProblemLibrary, Library: "libbluray"}
)

// TestPick checks which copy runs: the first that works in the order Kando prefers them, Kando's own last when the user chose the computer's and first when they chose it, one not yet run while none is known to work, and the first when all are broken; and which copy on the computer the user may choose instead of Kando's own.
func TestPick(t *testing.T) {
	const own, brew, path = "/kando/ffmpeg", "/opt/homebrew/bin/ffmpeg", "/usr/local/bin/ffmpeg"
	cases := []struct {
		name          string
		candidates    []string
		own           string
		choice        string
		health        map[string]media.Health
		chosen, other string
	}{
		{"own works", []string{own, brew}, own, "", map[string]media.Health{own: ready, brew: ready}, own, brew},
		{"computer chosen", []string{own, brew}, own, Computer, map[string]media.Health{own: ready, brew: ready}, brew, brew},
		{"computer chosen but broken", []string{own, brew}, own, Computer, map[string]media.Health{own: ready, brew: broken}, own, ""},
		{"brew broken, none of Kando's own", []string{brew, path}, "", "", map[string]media.Health{brew: broken, path: ready}, path, ""},
		{"none answered yet", []string{brew, path}, "", "", map[string]media.Health{brew: broken}, path, ""},
		{"all broken", []string{brew, path}, "", "", map[string]media.Health{brew: broken, path: broken}, brew, ""},
		// On Linux Kando's own ffmpeg comes last, unless the user chose it.
		{"own chosen over the computer's", []string{brew, own}, own, Own, map[string]media.Health{own: ready, brew: ready}, own, brew},
		{"none", nil, "", "", nil, "", ""},
	}
	for _, c := range cases {
		chosen, other := Pick(c.candidates, c.own, c.choice, c.health)
		if chosen != c.chosen || other != c.other {
			t.Errorf("%s: Pick = %q, %q, want %q, %q", c.name, chosen, other, c.chosen, c.other)
		}
	}
}

// TestStageTools checks that every step of media.Make that runs a program names it, and the translation, which runs none, names none.
func TestStageTools(t *testing.T) {
	if got := StageTools("transcribe"); !slices.Equal(got, []string{"ffmpeg", "whisper-cli"}) {
		t.Errorf("transcribe runs %v", got)
	}
	if got := StageTools("download"); !slices.Contains(got, "ffmpeg") {
		t.Errorf("the download, which merges with ffmpeg, runs %v", got)
	}
	if got := StageTools("translate"); got != nil {
		t.Errorf("translate runs %v", got)
	}
}

func TestWhy(t *testing.T) {
	m := testlocale.Messages()
	cases := map[media.Health]string{
		broken: m.ToolProblemLibrary("libbluray"),
		{State: media.HealthBroken, Problem: media.ProblemLibrary}:     m.ToolProblemLibraryUnnamed(),
		{State: media.HealthBroken, Problem: media.ProblemCrashed}:     m.ToolProblemCrashed(),
		{State: media.HealthBroken, Problem: media.ProblemCannotStart}: m.ToolProblemCannotStart(),
		{State: media.HealthBroken, Problem: media.ProblemFailed}:      m.ToolProblemFailed(),
	}
	for h, want := range cases {
		if got := Why(m, h); got != want {
			t.Errorf("Why(%+v) = %q, want %q", h, got, want)
		}
	}
	if got := Why(m, broken); got != "启动时缺少它需要的 libbluray" {
		t.Errorf("Why says %q", got)
	}
}
