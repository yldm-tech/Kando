package films

import (
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/sidebar"
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
	if got := QueueSummary(testlocale.Messages(), items); got != "1 个进行中 · 2 个排队 · 2 个需要处理" {
		t.Fatalf("summary %q", got)
	}
	if got := QueueSummary(testlocale.Messages(), items[2:3]); got != "1 个进行中" {
		t.Fatalf("summary %q", got)
	}
	// A cancelled job needs nothing, as the sidebar's count and the bulk buttons say.
	stopped := append(items, Film{Key: "stopped", JobID: "6", Stage: core.StageFailed, Stopped: true})
	if got := QueueSummary(testlocale.Messages(), stopped); got != "1 个进行中 · 2 个排队 · 2 个需要处理" {
		t.Fatalf("summary with a cancelled job %q", got)
	}
}

// TestHeardFallsBackToTheLanguageNamed checks that a film the speech recognition has not told the language of, as one transcribed before Kando kept it, is filed under the language the user named for it (Film.SpeechLanguage): the language filter, its counts, the order by language and the poster's tag take it, and the language heard wins over it once there is one.
func TestHeardFallsBackToTheLanguageNamed(t *testing.T) {
	items := []Film{
		{Key: "named", SpeechLanguage: "ja", Language: "简体中文"},
		{Key: "heard", Spoken: "en", SpeechLanguage: "ja", Language: "简体中文"},
		{Key: "unknown", Language: "简体中文"},
	}
	if got := BySpoken(items, "ja"); len(got) != 1 || got[0].Key != "named" {
		t.Fatalf("films in ja %+v, want the one named ja", got)
	}
	if got := SpokenCounts(items); !slices.Equal(got, []sidebar.SpokenCount{{Code: "en", Count: 1}, {Code: "ja", Count: 1}}) {
		t.Fatalf("counts %+v", got)
	}
	if got := keysOf(Sorted(items, SortSpoken, testlocale.Current(), neverPlayed)); got != "named,heard,unknown" {
		t.Fatalf("by spoken language %s", got)
	}
	if got := PosterLanguages(testlocale.Messages(), items[0], "简体中文"); got != "JA" {
		t.Fatalf("poster tag %q, want JA", got)
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
		// A video of a list is that video.
		"https://www.youtube.com/watch?v=TRWgBxVAb1M&list=PL590L5WQmH8fJ5": "YouTube",
		"https://www.bilibili.com/video/BV1GJ411x7h7?p=3":                  "哔哩哔哩",
		// Links pasted together are counted.
		"https://youtu.be/TRWgBxVAb1M https://vimeo.com/76979871\nhttps://youtu.be/x": testlocale.Messages().FilmsLinkCount(3),
	} {
		label, _, ok := linkChip(testlocale.Messages(), raw)
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
	shelf := List(root, nil, videos, nil, name, name)
	if len(shelf) != 2 || shelf[0].SubtitlePath != videos[0].Subtitles || shelf[1].SubtitlePath != "" {
		t.Fatalf("shelf = %#v", shelf)
	}
	// How long a film is shows on its poster, not in the line under it.
	if got, want := doneMeta(testlocale.Messages(), shelf[0]), "Local Files"; got != want {
		t.Errorf("a video with subtitles beside it reads %q, want %q", got, want)
	}
	if got, want := doneMeta(testlocale.Messages(), shelf[1]), "Local Files · No subtitles yet"; got != want {
		t.Errorf("a video with no subtitles reads %q, want %q", got, want)
	}
	done := Film{JobID: "j", Source: "https://youtu.be/TRWgBxVAb1M", Stage: core.StageComplete, SubtitlePath: "/Movies/a.kando/Talk/video.bilingual.srt"}
	if got, want := doneMeta(testlocale.Messages(), done), "YouTube"; got != want {
		t.Errorf("a finished job reads %q, want %q", got, want)
	}
}

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
	translate.Batch, translate.Batches, translate.BatchSize, translate.Lines, translate.Of = 2, 5, 40, 40, 160
	waiting := translate
	waiting.RetryIn, waiting.Attempt, waiting.Attempts = 8*time.Second, 1, 2
	unanswered := waiting
	unanswered.Unanswered = true
	for _, tc := range []struct {
		name string
		item Film
		want string
	}{
		{"next", Film{Source: yt, Stage: core.StageQueued}, "排队中"},
		{"queued", Film{Source: yt, Stage: core.StageQueued, Ahead: 2}, "排队中 · 前面还有 2 个"},
		{"download before yt-dlp says", Film{Source: yt, Stage: core.StageDownloading}, "下载中 · YouTube"},
		{"download", Film{Source: yt, Stage: core.StageDownloading, Progress: download}, "下载中 · 42% · 还剩 0:12"},
		{"local file", Film{Source: "file:///v/a.mp4", Stage: core.StageDownloading}, "读取中 · 本地文件"},
		{"transcribe", Film{Source: yt, Stage: core.StageTranscribing, Progress: core.Fraction(0.23)}, "生成字幕中 · 23%"},
		{"translate", Film{Source: yt, Stage: core.StageTranslating, Progress: translate}, "翻译中 · 40/160 句"},
		{"translate waits", Film{Source: yt, Stage: core.StageTranslating, Progress: waiting}, "翻译接口限流，8 秒后重试（1/2）"},
		{"translate waits after no answer", Film{Source: yt, Stage: core.StageTranslating, Progress: unanswered}, "翻译接口没有回应，8 秒后重试（1/2）"},
		{"export", Film{Source: yt, Stage: core.StageExporting, Progress: core.Fraction(0.5)}, "导出字幕中"},
	} {
		if got := QueueMeta(testlocale.Messages(), tc.item); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
	if !RetryWaiting(Film{Stage: core.StageTranslating, Progress: waiting}) || RetryWaiting(Film{Stage: core.StageTranslating, Progress: translate}) {
		t.Fatal("RetryWaiting is wrong")
	}
}

// The time a stage has left shows only once there is a rate to tell it from: 10 seconds and 5% into the stage since it first said how far it had gone, which a translation resumed halfway counts from where it resumed.
func TestTimeLeftIsToldOnlyFromARate(t *testing.T) {
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	at := func(fraction, from float64, ago time.Duration) Film {
		p := core.Fraction(fraction)
		p.From, p.Started, p.Lines, p.Of = from, now.Add(-ago), int(fraction*480), 480
		return Film{JobID: "j", Source: "https://youtu.be/x", Stage: core.StageTranslating, Progress: p}
	}
	m := testlocale.Messages()
	for _, tc := range []struct {
		name string
		item Film
		want string
	}{
		{"too soon", at(0.5, 0, 5*time.Second), "翻译中 · 240/480 句"},
		{"too little done", at(0.04, 0, time.Minute), "翻译中 · 19/480 句"},
		{"minutes", at(0.25, 0, 2*time.Minute), "翻译中 · 120/480 句 · 约剩 6 分钟"},
		{"resumed halfway", at(0.6, 0.5, time.Minute), "翻译中 · 288/480 句 · 约剩 4 分钟"},
		{"under a minute", at(0.9, 0, 3*time.Minute), "翻译中 · 432/480 句 · 不到 1 分钟"},
		{"not timed", Film{JobID: "j", Source: "https://youtu.be/x", Stage: core.StageTranscribing, Progress: core.Fraction(0.5)}, "生成字幕中 · 50%"},
	} {
		if got := queueMetaAt(m, tc.item, now); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
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
	m := testlocale.Messages()
	const yt = "https://youtu.be/TRWgBxVAb1M"
	download := core.Fraction(0.42)
	download.ETA = 12 * time.Second
	waiting := core.Fraction(0.25)
	waiting.RetryIn, waiting.Attempt, waiting.Attempts = time.Second, 1, 2
	batches := core.Fraction(0.25)
	batches.Lines, batches.Of = 120, 480
	for _, tc := range []struct{ got, want string }{
		{QueueMeta(m, Film{Source: yt, Stage: core.StageQueued, Ahead: 2}), "Queued · 2 ahead"},
		{QueueMeta(m, Film{Source: yt, Stage: core.StageDownloading, Progress: download}), "Downloading · 42% · 0:12 left"},
		{QueueMeta(m, Film{Source: "https://www.douyin.com/video/1", Stage: core.StageDownloading}), "Downloading · Douyin"},
		{QueueMeta(m, Film{Source: yt, Stage: core.StageTranslating, Progress: waiting}), "Translation API rate-limited, retrying in 1 second (1/2)"},
		{QueueMeta(m, Film{Source: yt, Stage: core.StageTranslating, Progress: batches}), "Translating · 120 of 480 lines"},
		{QueueMeta(m, Film{Source: yt, Stage: core.StageExporting}), "Exporting subtitles"},
		{StageLabel(m, core.StageComplete), "Done"},
		{SourceLabel(m, Film{Source: "https://www.bilibili.com/video/BV1GJ411x7h7"}), "Bilibili"},
		{SourceLabel(m, Film{Source: "https://vimeo.com/76979871"}), "Vimeo"},
		{SourceLabel(m, Film{Source: "https://example.com/v"}), "example.com"},
		{SourceLabel(m, Film{}), "Local Files"},
		{QueueSummary(m, []Film{{Stage: core.StageTranscribing}, {Stage: core.StageQueued}, {Stage: core.StageQueued}, {Stage: core.StageFailed}}), "1 running · 2 queued · 1 needs attention"},
		{LanguageName(m, "简体中文"), "Simplified Chinese"},
		{LanguageName(m, "日本語"), "Japanese"},
	} {
		if tc.got != tc.want {
			t.Errorf("%q, want %q", tc.got, tc.want)
		}
	}
}

// TestMatchesTitleReadsTheWholeName checks that the search matches the film's whole name, not its shortened title, whatever the case and the spaces around the query, and that an empty query keeps every film.
func TestMatchesTitleReadsTheWholeName(t *testing.T) {
	rivers := Film{Key: "a", Title: Shorten("How Rivers Shape Landscapes", 22), Name: "How Rivers Shape Landscapes"}
	found := Film{Key: "b", Title: "会议录屏 2026-10-28"}
	for _, tc := range []struct {
		item  Film
		query string
		want  bool
	}{
		{rivers, "landscapes", true},
		{rivers, "  RIVERS shape ", true},
		{rivers, "ocean", false},
		{found, "会议", true},
		{found, "", true},
	} {
		if got := MatchesTitle(tc.item, tc.query); got != tc.want {
			t.Errorf("MatchesTitle(%q, %q) = %v, want %v", filmName(tc.item), tc.query, got, tc.want)
		}
	}
	items := []Film{rivers, found}
	if got := ByTitle(items, " "); len(got) != 2 {
		t.Fatalf("ByTitle with a blank query kept %d films", len(got))
	}
	if got := ByTitle(items, "rivers"); len(got) != 1 || got[0].Key != "a" {
		t.Fatalf("ByTitle(rivers) = %+v", got)
	}
}

// TestListShowsTheTitlesGivenInKando checks that a title given in Kando takes the place of a job's name and of a loose video's, in full and shortened, and that films without one keep their own.
func TestListShowsTheTitlesGivenInKando(t *testing.T) {
	jobs := []core.Job{
		{ID: "a", Source: "https://youtu.be/x", Stage: core.StageComplete, VideoPath: "/lib/A/video.mp4", LibraryDir: "/lib"},
		{ID: "b", Source: "https://youtu.be/y", Stage: core.StageComplete, VideoPath: "/lib/B/video.mp4", LibraryDir: "/lib"},
		{ID: "q", Source: "https://youtu.be/z", Stage: core.StageQueued, LibraryDir: "/lib"},
	}
	videos := []media.Video{{Path: "/lib/C/video.mp4", Name: "C.mp4"}, {Path: "/lib/D.mp4", Name: "D.mp4"}}
	names := map[string]string{"/lib/A/video.mp4": "A title given in Kando that is long", "/lib/C/video.mp4": "Loose", "": "never"}
	nameOf := func(job core.Job) string { return "site " + job.ID }
	got := map[string]Film{}
	for _, item := range List("/lib", jobs, videos, names, nameOf, func(core.Job) string { return "" }) {
		got[item.Key] = item
	}
	if a := got["a"]; a.Name != "A title given in Kando that is long" || a.Title != Shorten(a.Name, 22) {
		t.Fatalf("a = %+v", a)
	}
	if got["b"].Name != "site b" || got["q"].Name != "site q" {
		t.Fatalf("b = %+v, q = %+v", got["b"], got["q"])
	}
	if c := got["/lib/C/video.mp4"]; c.Name != "Loose" || c.Title != "Loose" {
		t.Fatalf("c = %+v", c)
	}
	if d := got["/lib/D.mp4"]; d.Name != "D" {
		t.Fatalf("d = %+v", d)
	}
}
