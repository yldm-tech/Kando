package remote

import "testing"

func TestParseClock(t *testing.T) {
	for in, want := range map[string]int64{
		"0":            0,
		"75":           75_000,
		"75.5":         75_500,
		"1:15":         75_000,
		"90:00":        5_400_000,
		"1:02:03":      3_723_000,
		"01:02:03,250": 3_723_250,
		" 12:34.05 ":   754_050,
	} {
		if got, err := ParseClock(in); err != nil || got != want {
			t.Errorf("ParseClock(%q) = %d, %v, want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", ":", "1:", "-5", "1:60", "1:2:3:4", "1.5:00", "1e3", "inf", "0x10", "1:00.", "soon"} {
		if got, err := ParseClock(in); err == nil {
			t.Errorf("ParseClock(%q) = %d, want an error", in, got)
		}
	}
}

func TestCueFormats(t *testing.T) {
	cues := []Cue{
		{StartMS: 1500, EndMS: 3000, Text: "Hello\nthere", Translation: "你好"},
		{StartMS: 4000, EndMS: 5000, Text: "  "},
		{StartMS: 3_725_250, EndMS: 3_727_000, Text: "Bye"},
	}
	if got, want := CuesText(cues), "[00:00:01] 你好 | Hello there\n[01:02:05] Bye\n"; got != want {
		t.Errorf("CuesText = %q, want %q", got, want)
	}
	if got, want := CuesSRT(cues), "1\n00:00:01,500 --> 00:00:03,000\n你好\nHello\nthere\n\n2\n01:02:05,250 --> 01:02:07,000\nBye\n\n"; got != want {
		t.Errorf("CuesSRT = %q, want %q", got, want)
	}
	if got := PickCues(cues, WhichTranslation); len(got) != 1 || got[0].Text != "" || got[0].Translation != "你好" {
		t.Errorf("PickCues translation = %+v", got)
	}
	if got := PickCues(cues, WhichOriginal); len(got) != 3 || got[0].Translation != "" {
		t.Errorf("PickCues original = %+v", got)
	}
	if got := PickCues(nil, WhichBoth); got == nil {
		t.Error("PickCues of none is nil")
	}
}
