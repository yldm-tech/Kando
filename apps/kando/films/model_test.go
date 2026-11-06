package films

import (
	"strings"
	"testing"
	"time"

	"kando/packages/core"
)

func TestQueueOrderRunsOldestFirst(t *testing.T) {
	// The shelf lists jobs newest first.
	items := []Film{
		{Key: "failed-new", JobID: "1", Stage: core.StageFailed},
		{Key: "waiting-new", JobID: "2", Stage: core.StageQueued},
		{Key: "running", JobID: "3", Stage: core.StageTranslating},
		{Key: "waiting-old", JobID: "4", Stage: core.StageQueued},
		{Key: "failed-old", JobID: "5", Stage: core.StageFailed},
	}
	var keys []string
	for _, item := range QueueOrder(items) {
		keys = append(keys, item.Key)
	}
	if got := strings.Join(keys, ","); got != "running,waiting-old,waiting-new,failed-new,failed-old" {
		t.Fatalf("order %s", got)
	}
	if got := QueueSummary(items); got != "3 个进行中 · 2 个需要处理" {
		t.Fatalf("summary %q", got)
	}
	if got := QueueSummary(items[2:3]); got != "1 个进行中" {
		t.Fatalf("summary %q", got)
	}
}

func TestStepsLeaveOutDownloadingForLocalFiles(t *testing.T) {
	for _, tc := range []struct {
		source      string
		stage       core.Stage
		total, done int
	}{
		{"https://youtu.be/TRWgBxVAb1M", core.StageQueued, 4, 0},
		{"https://youtu.be/TRWgBxVAb1M", core.StageDownloading, 4, 0},
		{"https://youtu.be/TRWgBxVAb1M", core.StageTranslating, 4, 2},
		{"https://youtu.be/TRWgBxVAb1M", core.StageComplete, 4, 4},
		{"file:///v.mp4", core.StageDownloading, 3, 0},
		{"file:///v.mp4", core.StageTranscribing, 3, 0},
		{"file:///v.mp4", core.StageExporting, 3, 2},
	} {
		total, done := Steps(Film{JobID: "j", Source: tc.source, Stage: tc.stage})
		if total != tc.total || done != tc.done {
			t.Errorf("%s %s: steps %d/%d, want %d/%d", tc.source, tc.stage, done, total, tc.done, tc.total)
		}
	}
}

func TestLinkChipNamesThePlatform(t *testing.T) {
	for raw, want := range map[string]string{
		"https://www.youtube.com/watch?v=TRWgBxVAb1M":           "YouTube",
		"https://www.youtube.com/playlist?list=PL590L5WQmH8fJ5": "YouTube · 合集",
		"https://www.bilibili.com/video/BV1GJ411x7h7":           "哔哩哔哩",
		"https://space.bilibili.com/946974":                     "哔哩哔哩 · 频道",
		"https://vimeo.com/76979871":                            "Vimeo",
		"file:///v.mp4":                                         "",
		"hello":                                                 "",
	} {
		label, _, ok := linkChip(raw)
		if label != want || ok != (want != "") {
			t.Errorf("linkChip(%q) = %q %v, want %q", raw, label, ok, want)
		}
	}
}

func TestClock(t *testing.T) {
	for d, want := range map[time.Duration]string{0: "0:00", 100 * time.Second: "1:40", 3723 * time.Second: "1:02:03"} {
		if got := Clock(d); got != want {
			t.Errorf("Clock(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestALocalVideoFollowsTheLibraryItWasStartedIn(t *testing.T) {
	job := core.Job{ID: "j", Source: "file:///Users/me/Downloads/talk.mp4", Stage: core.StageComplete, VideoPath: "/Users/me/Downloads/talk.mp4", LibraryDir: "/Movies/a.kando"}
	if !InLibrary("/Movies/a.kando", job) || InLibrary("/Movies/b.kando", job) {
		t.Fatal("a local video kept outside the library does not follow the library it was started in")
	}
	job.Source = "https://youtu.be/TRWgBxVAb1M"
	if InLibrary("/Movies/a.kando", job) {
		t.Fatal("a downloaded video outside the library stayed on its shelf")
	}
}
