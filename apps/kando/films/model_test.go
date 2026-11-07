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

func TestQueueMetaSaysHowFarTheStageHasGone(t *testing.T) {
	yt := "https://www.youtube.com/watch?v=jNQXAC9IVRw"
	download := core.Fraction(0.42)
	download.Speed, download.ETA = 3.2*1024*1024, 12*time.Second
	translate := core.Fraction(0.25)
	translate.Batch, translate.Batches, translate.BatchSize = 2, 5, 40
	waiting := translate
	waiting.RetryIn, waiting.Attempt, waiting.Attempts = 8*time.Second, 1, 2
	for _, tc := range []struct {
		name string
		item Film
		want string
	}{
		{"queued", Film{Source: yt, Stage: core.StageQueued}, "排队中 · 等待前面的任务"},
		{"download before yt-dlp says", Film{Source: yt, Stage: core.StageDownloading}, "下载中 · yt-dlp · YouTube"},
		{"download", Film{Source: yt, Stage: core.StageDownloading, Progress: download}, "下载中 · 42% · 3.2 MB/s · 还剩 0:12"},
		{"local file", Film{Source: "file:///v/a.mp4", Stage: core.StageDownloading}, "读取中 · 本地文件"},
		{"transcribe", Film{Source: yt, Stage: core.StageTranscribing, Progress: core.Fraction(0.23)}, "生成字幕中 · whisper 本机识别 · 23%"},
		{"translate", Film{Source: yt, Stage: core.StageTranslating, Progress: translate}, "翻译中 · 第 2/5 批 · 每批 40 句"},
		{"translate waits", Film{Source: yt, Stage: core.StageTranslating, Progress: waiting}, "翻译接口限流，8 秒后重试（1/2）"},
		{"export", Film{Source: yt, Stage: core.StageExporting, Progress: core.Fraction(0.5)}, "导出中 · 写入 video.srt 与 video.bilingual.srt"},
	} {
		if got := QueueMeta(tc.item); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
	if !RetryWaiting(Film{Stage: core.StageTranslating, Progress: waiting}) || RetryWaiting(Film{Stage: core.StageTranslating, Progress: translate}) {
		t.Fatal("RetryWaiting is wrong")
	}
}

func TestStepFillsFollowTheStage(t *testing.T) {
	yt := "https://www.youtube.com/watch?v=jNQXAC9IVRw"
	for _, tc := range []struct {
		item Film
		want []float64
	}{
		{Film{JobID: "1", Source: yt, Stage: core.StageQueued}, []float64{0, 0, 0, 0}},
		{Film{JobID: "1", Source: yt, Stage: core.StageDownloading, Progress: core.Fraction(0.42)}, []float64{0.42, 0, 0, 0}},
		{Film{JobID: "1", Source: yt, Stage: core.StageTranslating, Progress: core.Fraction(0.5)}, []float64{1, 1, 0.5, 0}},
		{Film{JobID: "1", Source: "file:///v/a.mp4", Stage: core.StageExporting, Progress: core.Fraction(1)}, []float64{1, 1, 1}},
		{Film{JobID: "1", Source: yt, Stage: core.StageComplete}, []float64{1, 1, 1, 1}},
	} {
		got := StepFills(tc.item)
		if len(got) != len(tc.want) {
			t.Fatalf("%s: %v, want %v", tc.item.Stage, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("%s: %v, want %v", tc.item.Stage, got, tc.want)
			}
		}
	}
}
