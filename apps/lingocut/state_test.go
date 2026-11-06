package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"lingocut/apps/lingocut/sidebar"
	"lingocut/packages/core"
	"lingocut/packages/media"
)

func TestPlayerSettingsRoundTrip(t *testing.T) {
	a, _, _ := newTestApp(&fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.player.look.Translation.Size = 30
	a.player.look.Original.Color = "#FFD60A"
	a.player.subMode = subOriginal
	a.player.subOrder = subOriginalFirst
	a.player.untranslated = untranslatedBlank
	a.player.yieldSubs = false
	a.overlay.onboarded = true
	if err := a.saveSettings(); err != nil {
		t.Fatal(err)
	}
	saved := loadSavedSettings(a.settingsPath)
	b, _, _ := newTestApp(&fakePipeline{})
	b.restorePlayer(saved)
	b.overlay.onboarded = saved.Onboarded
	if b.player.look != a.player.look || b.player.subMode != subOriginal || b.player.subOrder != subOriginalFirst || b.player.untranslated != untranslatedBlank || b.player.yieldSubs || !b.overlay.onboarded {
		t.Fatalf("restored %+v onboarded %v from %+v", b.player, b.overlay.onboarded, saved)
	}
}

// TestPlayerSettingsDefaults checks that settings saved before these fields existed, and values this version does not know, give the defaults.
func TestPlayerSettingsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	data, err := json.Marshal(map[string]any{"language": "日本語", "subMode": "karaoke", "subOrder": "", "untranslated": "hide"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	a, _, _ := newTestApp(&fakePipeline{})
	a.restorePlayer(loadSavedSettings(path))
	def := newPlayerUI()
	if a.player.look != def.look || a.player.subMode != def.subMode || a.player.subOrder != def.subOrder || a.player.untranslated != def.untranslated || a.player.yieldSubs != def.yieldSubs {
		t.Fatalf("player %+v, want the defaults %+v", a.player, def)
	}
}

func TestResumeAtRemembersWhereTheVideoStopped(t *testing.T) {
	a, _, _ := newTestApp(&fakePipeline{})
	a.rememberPosition(&filmPlay{path: "/v.mp4", duration: 10 * time.Second, frames: &frameDecoder{shown: 3 * time.Second}})
	if at := a.ResumeAt("/v.mp4"); at != 3*time.Second {
		t.Fatalf("ResumeAt = %v", at)
	}
	a.rememberPosition(&filmPlay{path: "/v.mp4", duration: 10 * time.Second, finished: true})
	if at := a.ResumeAt("/v.mp4"); at != 0 {
		t.Fatalf("ResumeAt after the end = %v", at)
	}
}

func TestSourcesFilterTheLibrary(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	a.library.Jobs = []core.Job{
		{ID: "yt", Source: "https://youtu.be/TRWgBxVAb1M", Stage: core.StageComplete, VideoPath: "/videos/yt-clip/video.mp4"},
		{ID: "bili", Source: "https://www.bilibili.com/video/BV1GJ411x7h7", Stage: core.StageComplete, VideoPath: "/videos/bili-clip/video.mp4"},
		{ID: "web", Source: "https://vimeo.com/76979871", Stage: core.StageComplete, VideoPath: "/videos/web-clip/video.mp4"},
		{ID: "file", Source: "file:///videos/file-clip/video.mp4", Stage: core.StageComplete, VideoPath: "/videos/file-clip/video.mp4"},
	}
	a.edit(func(s *session) {
		s.Videos = []media.Video{{Name: "loose-clip.mp4", Path: "/videos/loose/loose-clip.mp4"}}
	})
	want := map[string]int{sidebar.SourceAll: 5, sidebar.SourceYouTube: 1, sidebar.SourceBilibili: 1, sidebar.SourceWeb: 1, sidebar.SourceLocal: 2}
	for source, n := range want {
		if got := a.SourceCount(source); got != n {
			t.Errorf("SourceCount(%q) = %d, want %d", source, got, n)
		}
	}
	a.openKey = "yt"
	a.SetSourceFilter(sidebar.SourceLocal)
	tt.Frame()
	if a.openKey != "" || a.page != pageFilms {
		t.Fatalf("open %q page %q", a.openKey, a.page)
	}
	if !tt.HasText("file-clip") || !tt.HasText("loose-clip") || tt.HasText("bili-clip") || tt.HasText("yt-clip") || tt.HasText("web-clip") {
		t.Fatalf("local films not shown alone: %q", tt.Texts())
	}
}

func TestOpenSettingsGoesToTheSection(t *testing.T) {
	a, _, _ := newTestApp(&fakePipeline{})
	a.openKey = "x"
	a.OpenSettings("translate")
	if a.page != pageTranslate || a.openKey != "" {
		t.Fatalf("page %q open %q", a.page, a.openKey)
	}
	for _, section := range []string{"", "look", "shortcuts", "nope"} {
		a.OpenSettings(section)
		if a.page != pageGeneral {
			t.Fatalf("OpenSettings(%q) went to %q", section, a.page)
		}
	}
}
