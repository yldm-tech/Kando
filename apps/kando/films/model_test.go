package films

import (
	"strings"
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
	"kando/packages/core"
	"kando/packages/media"
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

// TestAVideoWithoutAJobShowsTheSubtitlesBesideIt checks that a video no job holds takes the bilingual subtitles the library scan found beside it, and that its card says it has no subtitles only when it has none.
func TestAVideoWithoutAJobShowsTheSubtitlesBesideIt(t *testing.T) {
	testlocale.Use(t, "en")
	root := "/Movies/a.kando"
	videos := []media.Video{
		{Path: root + "/Copied/video.mp4", Name: "Copied.mp4", Subtitles: root + "/Copied/video.bilingual.srt"},
		{Path: root + "/Bare/video.mp4", Name: "Bare.mp4"},
	}
	name := func(core.Job) string { return "" }
	shelf := List(root, nil, videos, name, name)
	if len(shelf) != 2 || shelf[0].SubtitlePath != videos[0].Subtitles || shelf[1].SubtitlePath != "" {
		t.Fatalf("shelf = %#v", shelf)
	}
	a := clockApp{d: 42 * time.Second}
	if got, want := doneMeta(a, shelf[0]), "Local Files · 0:42"; got != want {
		t.Errorf("a video with subtitles beside it reads %q, want %q", got, want)
	}
	if got, want := doneMeta(a, shelf[1]), "Local Files · 0:42 · No subtitles yet"; got != want {
		t.Errorf("a video with no subtitles reads %q, want %q", got, want)
	}
	done := Film{JobID: "j", Source: "https://youtu.be/TRWgBxVAb1M", Stage: core.StageComplete, SubtitlePath: "/Movies/a.kando/Talk/video.bilingual.srt"}
	if got, want := doneMeta(a, done), "YouTube · 0:42"; got != want {
		t.Errorf("a finished job reads %q, want %q", got, want)
	}
}

// clockApp is an App that knows only how long every video is, which is all doneMeta asks of it.
type clockApp struct {
	App
	d time.Duration
}

func (a clockApp) Duration(string) time.Duration { return a.d }

// The queue holds the open library's jobs that are not finished and the other libraries' jobs the queue runs, each of those naming its library, in the shelf's order.
func TestQueueHoldsTheJobsOfOtherLibrariesTheQueueRuns(t *testing.T) {
	open, other := "/Movies/a.kando", "/Movies/b.kando"
	jobs := []core.Job{
		{ID: "waiting-other", Stage: core.StageQueued, LibraryDir: other},
		{ID: "waiting", Stage: core.StageQueued, LibraryDir: open},
		{ID: "done", Stage: core.StageComplete, VideoPath: open + "/done/video.mp4", LibraryDir: open},
		{ID: "done-other", Stage: core.StageComplete, VideoPath: other + "/done/video.mp4", LibraryDir: other},
		{ID: "running-other", Stage: core.StageTranscribing, VideoPath: other + "/talk/video.mp4", LibraryDir: other},
		{ID: "failed", Stage: core.StageFailed, LibraryDir: open},
	}
	runs := func(job core.Job) bool { return job.ID == "running-other" || job.ID == "waiting-other" }
	name := func(job core.Job) string { return job.ID }
	var got []string
	for _, item := range Queue(open, jobs, runs, name, name) {
		got = append(got, item.JobID+"@"+item.Library)
	}
	if strings.Join(got, ",") != "waiting-other@b,waiting@,running-other@b,failed@" {
		t.Fatalf("queue %q", got)
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
	unanswered := waiting
	unanswered.Unanswered = true
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
		{"translate waits after no answer", Film{Source: yt, Stage: core.StageTranslating, Progress: unanswered}, "翻译接口没有回应，8 秒后重试（1/2）"},
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

// TestLabelsInEnglish reads a job's line, its stage, where a film comes from and the summary of the queue in English, plurals included.
func TestLabelsInEnglish(t *testing.T) {
	testlocale.Use(t, "en")
	const yt = "https://youtu.be/TRWgBxVAb1M"
	download := core.Fraction(0.42)
	download.ETA = 12 * time.Second
	waiting := core.Fraction(0.25)
	waiting.RetryIn, waiting.Attempt, waiting.Attempts = time.Second, 1, 2
	batches := core.Fraction(0.25)
	batches.Batch, batches.Batches, batches.BatchSize = 1, 3, 1
	for _, tc := range []struct{ got, want string }{
		{QueueMeta(Film{Source: yt, Stage: core.StageQueued}), "Queued · Waiting for the jobs ahead"},
		{QueueMeta(Film{Source: yt, Stage: core.StageDownloading, Progress: download}), "Downloading · 42% · 0:12 left"},
		{QueueMeta(Film{Source: "https://www.douyin.com/video/1", Stage: core.StageDownloading}), "Downloading · yt-dlp · Douyin"},
		{QueueMeta(Film{Source: yt, Stage: core.StageTranslating, Progress: waiting}), "Translation API rate-limited, retrying in 1 second (1/2)"},
		{QueueMeta(Film{Source: yt, Stage: core.StageTranslating, Progress: batches}), "Translating · Batch 1/3 · 1 line per batch"},
		{StageLabel(core.StageComplete), "Done"},
		{SourceLabel(Film{Source: "https://www.bilibili.com/video/BV1GJ411x7h7"}), "Bilibili"},
		{SourceLabel(Film{Source: "https://vimeo.com/76979871"}), "Vimeo"},
		{SourceLabel(Film{Source: "https://example.com/v"}), "example.com"},
		{SourceLabel(Film{}), "Local Files"},
		{QueueSummary([]Film{{Stage: core.StageQueued}, {Stage: core.StageFailed}, {Stage: core.StageFailed}}), "1 in progress · 2 need attention"},
		{LanguageName("简体中文"), "Simplified Chinese"},
		{LanguageName("日本語"), "Japanese"},
	} {
		if tc.got != tc.want {
			t.Errorf("%q, want %q", tc.got, tc.want)
		}
	}
}
