package main

import (
	"encoding/json"
	"strings"
	"testing"

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
