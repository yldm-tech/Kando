package main

import (
	"time"

	"lingocut/packages/core"
)

// The prototype's data, which the screenshot fixture (screenshot_helpers_test.go) turns into a library, so the scenes show what the design shows. It is the prototype's mock state before its first tick (spec/fixture.json beside the design's screenshots), written out here so the tests read nothing outside this repository. The scene pictures each film shows are in testdata/scenes.

// protoLibrary is one of the prototype's libraries, a folder ~/Movies/<Name>.lingocut in the fixture's fake home. Count is how many films the prototype's library switcher says it holds.
type protoLibrary struct {
	ID, Name string
	Count    int
}

// protoLibraries are the prototype's libraries; the first is the one open.
var protoLibraries = []protoLibrary{
	{ID: "main", Name: "我的影片", Count: 14},
	{ID: "listening", Name: "英语听力", Count: 1},
}

// protoSourceCounts are the counts the prototype's sidebar shows beside each source for 我的影片, by sidebar source.
var protoSourceCounts = map[string]int{"all": 14, "youtube": 9, "bilibili": 4, "local": 1, "web": 0}

// protoFilm is one of the prototype's films. ID is its job ID and its card's key, so a scene picks a film by the prototype's ID (j1 is How Rivers Shape Landscapes, the hero). Platform and VideoID make its link; a local film has none and is processed from its file. Seconds is the length of its video, 0 while it has none: a film that has not finished downloading has no video yet.
type protoFilm struct {
	ID, Library string
	Title       string
	Platform    core.Platform
	VideoID     string
	Stage       core.Stage
	Seconds     int
	// Scene is the prototype's picture of the film (testdata/scenes), which the fake ffmpeg shows as its video.
	Scene string
	// AudioOnly is a film the prototype downloaded as sound only (仅字幕); PlatformSubs one that used the platform's own subtitles; Series is the series it belongs to.
	AudioOnly    bool
	PlatformSubs bool
	Series       string
	// Watched is where the prototype's player stopped last time.
	Watched time.Duration
	// Queue is the film's card in the prototype's 处理中, for a film still being processed or failed.
	Queue *protoQueue
}

// protoQueue is a card in the prototype's 处理中: how far each stage after 排队 has gone, in percent, the line under the title, and for a failed job what went wrong (Kind), why, and what the card offers to fix it.
type protoQueue struct {
	Downloading, Transcribing, Translating, Exporting int
	Meta                                              string
	Kind, Why                                         string
	Remedies                                          []protoRemedy
	// Err is what the pipeline reports for this failure: the error yt-dlp gives for it, as media.Run wraps it.
	Err string
}

// protoRemedy is a button on a failed card.
type protoRemedy struct {
	Label, Action string
	Primary       bool
}

// protoFilms are the prototype's films in its order. 处理中 lists them running first (j2, j6, j12), then failed (j5, j13, j14); the grid lists the finished ones of 我的影片 (j1, j3, j4, j8, j9, j10, j11, j15).
var protoFilms = []protoFilm{
	{ID: "j2", Library: "main", Title: "Morning Market in Kyoto", Platform: core.PlatformYouTube, VideoID: "k9Yt2LmQ0pX", Stage: core.StageTranslating, Seconds: 180, Scene: "market",
		Queue: &protoQueue{Downloading: 100, Transcribing: 100, Translating: 30, Meta: "翻译中 · 即时译文已就绪 · 上下文翻译 30%"}},
	{ID: "j6", Library: "main", Title: "Ocean Currents Explained", Platform: core.PlatformYouTube, VideoID: "Hq2Rt9sK4mV", Stage: core.StageQueued, Scene: "ocean",
		Queue: &protoQueue{Meta: "排队中 · 等待前面的任务"}},
	{ID: "j5", Library: "main", Title: "Street Food Night Tour", Platform: core.PlatformYouTube, VideoID: "Zp4Qw7nB1cE", Stage: core.StageFailed, Scene: "street",
		Queue: &protoQueue{Meta: "YouTube 改版了，下载组件要更新", Kind: "ytdlp", Why: "内置 yt-dlp 是 2026.09.14 版，更新几秒就好。",
			Remedies: []protoRemedy{{Label: "更新并重试", Action: "update-ytdlp-and-retry", Primary: true}, {Label: "移除", Action: "remove"}},
			Err:      "download: yt-dlp: ERROR: [youtube] Zp4Qw7nB1cE: Unable to extract nsig function code"}},
	{ID: "j13", Library: "main", Title: "日语听力精讲 · 第 3 讲", Platform: core.PlatformBilibili, VideoID: "BV1Hx411c7Pq", Stage: core.StageFailed, Scene: "podcast",
		Queue: &protoQueue{Meta: "哔哩哔哩登录已过期", Kind: "cookie", Why: "先在 Safari 里重新登录 B 站，再回来重试。",
			Remedies: []protoRemedy{{Label: "已重新登录，重试", Action: "retry", Primary: true}, {Label: "改用 Chrome", Action: "switch-cookie-browser:Chrome"}, {Label: "移除", Action: "remove"}},
			Err:      "download: yt-dlp: ERROR: [BiliBili] BV1Hx411c7Pq: The cookies from Safari have expired; sign in again"}},
	{ID: "j14", Library: "main", Title: "Coastal Erosion Documentary", Platform: core.PlatformYouTube, VideoID: "Rg5Lk2Pz8Wd", Stage: core.StageFailed, Scene: "ocean",
		Queue: &protoQueue{Meta: "你所在的地区不能看", Kind: "region", Why: "上传者限制了地区。别处有这个视频的话，换个链接。",
			Remedies: []protoRemedy{{Label: "换个链接", Action: "swap-link", Primary: true}, {Label: "移除", Action: "remove"}},
			Err:      "download: yt-dlp: ERROR: [youtube] Rg5Lk2Pz8Wd: The uploader has not made this video available in your country"}},
	{ID: "j1", Library: "main", Title: "How Rivers Shape Landscapes", Platform: core.PlatformYouTube, VideoID: "a1B2c3D4e5F", Stage: core.StageComplete, Seconds: 180, Scene: "river", Series: "How Rivers Shape Landscapes", Watched: 100 * time.Second},
	{ID: "j3", Library: "main", Title: "Linear Algebra, Lecture 1", Platform: core.PlatformYouTube, VideoID: "Lx0v8RtE3aQ", Stage: core.StageComplete, Seconds: 180, Scene: "lecture"},
	{ID: "j4", Library: "main", Title: "会议录屏 2026-10-28", Platform: core.PlatformLocal, Stage: core.StageComplete, Seconds: 180, Scene: "meeting"},
	{ID: "j8", Library: "main", Title: "The Tide Pools of Monterey", Platform: core.PlatformYouTube, VideoID: "Tp8mNq3wRz1", Stage: core.StageComplete, Seconds: 180, Scene: "ocean"},
	{ID: "j9", Library: "main", Title: "How Rivers Shape Landscapes · Part 2", Platform: core.PlatformYouTube, VideoID: "b2C3d4E5f6G", Stage: core.StageComplete, Seconds: 180, Scene: "river", Series: "How Rivers Shape Landscapes"},
	{ID: "j10", Library: "main", Title: "How Rivers Shape Landscapes · Part 3", Platform: core.PlatformYouTube, VideoID: "c3D4e5F6g7H", Stage: core.StageComplete, Seconds: 180, Scene: "river", Series: "How Rivers Shape Landscapes", AudioOnly: true},
	{ID: "j11", Library: "main", Title: "线性代数的几何意义 · P2 向量与基", Platform: core.PlatformBilibili, VideoID: "BV1xK4y1A7nM", Stage: core.StageComplete, Seconds: 180, Scene: "lecture", PlatformSubs: true, Series: "线性代数的几何意义"},
	{ID: "j15", Library: "main", Title: "日语听力每日一练 · 第 1 练", Platform: core.PlatformBilibili, VideoID: "BV1Tz4y1M7kd", Stage: core.StageComplete, Seconds: 180, Scene: "podcast", AudioOnly: true, PlatformSubs: true, Series: "日语听力每日一练"},
	{ID: "j12", Library: "main", Title: "日常日语会话 第 5 集", Platform: core.PlatformBilibili, VideoID: "BV1Qe4y1J7aV", Stage: core.StageTranscribing, Seconds: 180, Scene: "podcast",
		Queue: &protoQueue{Downloading: 100, Transcribing: 40, Meta: "转录中 · Apple 语音识别 · 本机"}},
	{ID: "j7", Library: "listening", Title: "Daily English Podcast Ep.12", Platform: core.PlatformYouTube, VideoID: "Pd12xYz9AbC", Stage: core.StageComplete, Seconds: 180, Scene: "podcast"},
}

// protoFilmByID is the prototype's film with ID id; it panics for an ID the prototype does not have, which is a mistake in the scene asking for it.
func protoFilmByID(id string) protoFilm {
	for _, film := range protoFilms {
		if film.ID == id {
			return film
		}
	}
	panic("the prototype has no film " + id)
}

// protoCue is one of the prototype's 18 subtitles, which every finished film carries. Translation is the line the prototype shows. Machine is the prototype's zhOriginal, the translation before the user proofread it: it differs on cue 8, which the user edited, and on cue 13, whose translation failed, so it shows none.
type protoCue struct {
	Start, End            time.Duration
	Original, Translation string
	Machine               string
	Failed                bool
}

// cueAt is cue i: it starts 10 seconds after the one before, from 0.4 seconds in, and lasts 8.2 seconds, as the prototype times them. An empty translation is a failed one.
func cueAt(i int, original, translation, machine string) protoCue {
	start := time.Duration(i)*10*time.Second + 400*time.Millisecond
	if machine == "" {
		machine = translation
	}
	return protoCue{Start: start, End: start + 8200*time.Millisecond, Original: original, Translation: translation, Machine: machine, Failed: translation == ""}
}

// protoCues are the prototype's subtitles, about rivers.
var protoCues = []protoCue{
	cueAt(0, "Every river begins as a trickle high in the mountains.", "每条河都始于高山上的一股细流。", ""),
	cueAt(1, "As water flows downhill, it picks up sand and stones.", "水往低处流，沿途裹挟起泥沙和石块。", ""),
	cueAt(2, "Those particles act like sandpaper on the riverbed.", "这些颗粒就像砂纸一样打磨着河床。", ""),
	cueAt(3, "Over thousands of years, the channel cuts deeper.", "数千年间，河道被切割得越来越深。", ""),
	cueAt(4, "That is how steep V-shaped valleys are formed.", "陡峭的 V 形谷就是这样形成的。", ""),
	cueAt(5, "Lower down, the land flattens and the river slows.", "到了下游，地势变缓，河流也慢了下来。", ""),
	cueAt(6, "Instead of cutting down, it starts to swing sideways.", "它不再向下切割，而是开始左右摆动。", ""),
	cueAt(7, "These sweeping curves are called meanders.", "这些大幅度的弯道被称为河曲。", ""),
	cueAt(8, "The outer bank erodes while the inner bank grows.", "河曲外侧的河岸不断被侵蚀，内侧则不断淤积。", "外侧河岸不断被侵蚀，内侧则不断淤积。"),
	cueAt(9, "Sometimes a meander loops so far it cuts itself off.", "有时河曲弯得太远，会把自己截断。", ""),
	cueAt(10, "What remains is a crescent-shaped oxbow lake.", "留下来的就是一个新月形的牛轭湖。", ""),
	cueAt(11, "During floods, the river spills over its banks.", "洪水期间，河水会漫过河岸。", ""),
	cueAt(12, "It leaves behind fine, fertile silt across the plain.", "它在平原上留下细腻而肥沃的淤泥。", ""),
	cueAt(13, "That is why so many early farms grew along rivers.", "", "这就是为什么早期农田多沿河分布。"),
	cueAt(14, "Near the sea, the current finally loses its energy.", "接近大海时，水流终于失去了力量。", ""),
	cueAt(15, "Sediment drops out and builds a delta, layer by layer.", "泥沙沉积下来，一层层堆积成三角洲。", ""),
	cueAt(16, "From mountain to sea, the river never stops reshaping the land.", "从高山到大海，河流从未停止重塑大地。", ""),
	cueAt(17, "Next time you cross a bridge, look at the water below.", "下次过桥时，不妨看看桥下的流水。", ""),
}
