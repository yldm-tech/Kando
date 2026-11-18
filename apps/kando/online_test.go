package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/film"
	"kando/packages/core"
	"kando/packages/media"
)

// A link's job takes the source of its original lines and its quality from Settings › 通用 › 在线视频 unless it has its own, a video on disk takes neither, and the choices are saved in settings.json and taken back.
func TestLinksTakeTheOnlineVideoSettings(t *testing.T) {
	a := newApp(media.Tools{}, func(fn func()) { fn() })
	link := "https://www.youtube.com/watch?v=jNQXAC9IVRw"
	if job := a.onlineFor(core.Job{Source: link}); job.SubtitleSource != core.SubtitlesPlatform || job.Quality != core.QualityBest {
		t.Fatalf("by default %+v", job)
	}
	data, _ := json.Marshal(a.settingsFile())
	if strings.Contains(string(data), "subtitleSource") || strings.Contains(string(data), "downloadQuality") {
		t.Fatalf("the defaults were saved: %s", data)
	}

	a.SetPlatformSubtitles(false)
	a.SetDownloadQuality(core.Quality720)
	a.SetDownloadQuality("8k")
	if job := a.onlineFor(core.Job{Source: link}); job.SubtitleSource != core.SubtitlesLocal || job.Quality != core.Quality720 {
		t.Fatalf("after the change %+v", job)
	}
	own := core.Job{Source: link, SubtitleSource: core.SubtitlesPlatform, Quality: core.QualityBest}
	if job := a.onlineFor(own); job != own {
		t.Fatalf("a job's own choices became %+v", job)
	}
	if job := a.onlineFor(core.Job{Source: "file:///films/talk.mp4"}); job.SubtitleSource != "" || job.Quality != "" {
		t.Fatalf("a video on disk %+v", job)
	}

	data, _ = json.Marshal(a.settingsFile())
	var saved savedSettings
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	saved.clean()
	b := newApp(media.Tools{}, func(fn func()) { fn() })
	b.onlineHooks().restore(saved)
	if b.PlatformSubtitles() || b.DownloadQuality() != core.Quality720 {
		t.Fatalf("restored %+v from %s", b.online, data)
	}

	// A value another version saved that this one does not know is the default.
	saved = savedSettings{savedOnline: savedOnline{SubtitleSource: "reference", DownloadQuality: "4320"}}
	saved.clean()
	b.onlineHooks().restore(saved)
	if !b.PlatformSubtitles() || b.DownloadQuality() != core.QualityBest {
		t.Fatalf("restored %+v", b.online)
	}
}

// The queue gives a link's job the settings' choices as it first runs, and the job keeps them.
func TestQueueGivesALinkTheOnlineVideoSettings(t *testing.T) {
	a, tt, updates := newHiddenXApp(t)
	a.SetDownloadQuality(core.Quality720)
	if err := a.Enqueue(hiddenXPost); err != nil {
		t.Fatal(err)
	}
	settle(t, a, tt, updates)
	if job := stageOf(t, a, a.library.Jobs[0].ID); job.SubtitleSource != core.SubtitlesPlatform || job.Quality != core.Quality720 {
		t.Fatalf("job %+v", job)
	}
}

// A film whose job kept the chapters its site marks opens with them, cut at them while 导入平台章节 is on; cutting it by time is kept beside its video, so it opens that way again, and 导入平台章节 is saved in settings.json and taken back.
func TestFilmsCutAtTheChaptersTheirSiteMarks(t *testing.T) {
	a, tt, updates, item := playerFilm(t, 120, nil)
	kept := media.FilmChapters{Chapters: []core.Chapter{{Start: 0, Title: "Intro", Translation: "开场"}, {Start: 60 * time.Second, Title: "Valleys", Translation: "山谷"}}, Language: "简体中文"}
	if err := media.SaveChapters(item.VideoPath, kept); err != nil {
		t.Fatal(err)
	}
	playerOpenFilm(t, a, tt, updates, item)
	waitUntil(t, tt, a, updates, func() bool { return len(a.Info().Chapters) == 2 })
	if !film.CutsAtSite(a.Info(), a.ImportChapters()) {
		t.Fatalf("the film is not cut at its chapters: %+v", a.Info())
	}
	apply(tt, a, func() { a.SetFilmChapterUse(core.ChaptersEven) })
	if got, _ := media.ReadChapters(item.VideoPath); got.Use != core.ChaptersEven || film.CutsAtSite(a.Info(), a.ImportChapters()) {
		t.Fatalf("cut by time: on disk %+v, open %+v", got, a.Info())
	}

	if data, _ := json.Marshal(a.settingsFile()); strings.Contains(string(data), "evenChapters") {
		t.Fatalf("the default was saved: %s", data)
	}
	a.SetImportChapters(false)
	data, _ := json.Marshal(a.settingsFile())
	var saved savedSettings
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	b := newApp(media.Tools{}, func(fn func()) { fn() })
	b.onlineHooks().restore(saved)
	if b.ImportChapters() {
		t.Fatalf("restored %+v from %s", b.online, data)
	}
}
