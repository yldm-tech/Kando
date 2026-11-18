package main

import (
	"encoding/json"
	"path/filepath"
	"slices"
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

// A film open while its job translates the chapters' titles shows them translated once the job completes, without being opened again, as a film open while its lines are translated shows them.
func TestOpenFilmTakesTheChaptersItsJobTranslated(t *testing.T) {
	a, tt, updates, item := playerFilm(t, 120, nil)
	site := media.FilmChapters{Chapters: []core.Chapter{{Start: 0, Title: "Intro"}, {Start: 60 * time.Second, Title: "Valleys"}}}
	if err := media.SaveChapters(item.VideoPath, site); err != nil {
		t.Fatal(err)
	}
	playerOpenFilm(t, a, tt, updates, item)
	waitUntil(t, tt, a, updates, func() bool { return len(a.Info().Chapters) == 2 })

	job := a.library.Jobs[0]
	job.Stage, job.Redo, job.RedoLanguage = core.StageTranslating, core.RedoTranslation, "简体中文"
	apply(tt, a, func() { a.show(job) })
	translated := media.FilmChapters{Chapters: []core.Chapter{{Start: 0, Title: "Intro", Translation: "开场"}, {Start: 60 * time.Second, Title: "Valleys", Translation: "山谷"}}, Language: "简体中文"}
	if err := media.SaveChapters(item.VideoPath, translated); err != nil {
		t.Fatal(err)
	}
	job.Stage = core.StageComplete
	apply(tt, a, func() { a.show(job) })
	waitUntil(t, tt, a, updates, func() bool { return len(a.Info().Chapters) == 2 && a.Info().Chapters[1].Translation == "山谷" })
}

// Turning 导入平台章节 off while a film with its site's chapters is open cuts its 片段 by time at once, and grabs the frames of the new cuts; turning it on again cuts it at the chapters again.
func TestImportChaptersRecutsTheOpenFilm(t *testing.T) {
	tools := media.Tools{FFmpeg: fakeTool(t, filepath.Join(t.TempDir(), "bin"), "ffmpeg")}
	a, tt, updates, item := playerFilm(t, 120, tools)
	kept := media.FilmChapters{Chapters: []core.Chapter{{Start: 0, Title: "Intro", Translation: "开场"}, {Start: 7 * time.Second, Title: "Valleys", Translation: "山谷"}}, Language: "简体中文"}
	if err := media.SaveChapters(item.VideoPath, kept); err != nil {
		t.Fatal(err)
	}
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 120*time.Second)
	cuts := func() []film.Chapter {
		return film.ChaptersFor(a.Info(), a.ImportChapters(), a.player.subs.cues, a.PlayLength(), a.Delay())
	}
	framed := func() bool {
		list := cuts()
		for _, ch := range list {
			if _, ok := a.player.film.clipFrames[ch.Start]; !ok {
				return false
			}
		}
		return len(list) > 0
	}
	waitUntil(t, tt, a, updates, func() bool { return len(a.Info().Chapters) == 2 && framed() })
	atSite := cuts()
	if want := film.SiteChapters(kept.Chapters, a.PlayLength(), a.Delay()); !slices.Equal(atSite, want) {
		t.Fatalf("cut at %+v, want the site's %+v", atSite, want)
	}

	apply(tt, a, func() { a.SetImportChapters(false) })
	even := cuts()
	if film.CutsAtSite(a.Info(), a.ImportChapters()) || slices.Equal(even, atSite) {
		t.Fatalf("导入平台章节 off still cuts at %+v", even)
	}
	waitUntil(t, tt, a, updates, framed)

	// Only the even cuts have frames, so turning it on again has to grab the site's.
	apply(tt, a, func() {
		for start := range a.player.film.clipFrames {
			if !slices.ContainsFunc(even, func(ch film.Chapter) bool { return ch.Start == start }) {
				delete(a.player.film.clipFrames, start)
			}
		}
	})
	apply(tt, a, func() { a.SetImportChapters(true) })
	if got := cuts(); !slices.Equal(got, atSite) {
		t.Fatalf("导入平台章节 on again cuts at %+v", got)
	}
	waitUntil(t, tt, a, updates, framed)
}
