package main

import (
	"testing"

	"kando/apps/kando/mediasource"
	"kando/apps/kando/settings"
)

// TestScreenshotsMediaSources renders 设置 › 媒体来源 and the dialog it opens: two SMB sources, a share whose Test connected and a whole server whose Test listed its shares (settings-sources), and the dialog adding an SMB source of a NAS without signing, with what that risks (settings-sources-add). The servers are names in the scene, never reached.
func TestScreenshotsMediaSources(t *testing.T) {
	f := newScreenshotFixture(t)
	shown, models, turbo := settingsShotFolders(t, f)
	shootSettings(t, f, settings.SectionMediaSources, "settings-sources", shown, models, turbo, shotMediaSources)
	shootSettingsPressed(t, f, settings.SectionMediaSources, "settings-sources-add", shown, models, turbo, []string{"scroll-end:id:media-source-dialog-fields"}, func(a *app) {
		shotMediaSources(a)
		a.AddMediaSource()
		d := a.MediaSourceDraft()
		d.Name = "Old NAS"
		d.Values = map[string]string{mediasource.SMBHost: "192.168.1.20", mediasource.SMBShare: "video", mediasource.SMBUser: "guest", mediasource.SMBNoSigning: "true"}
		a.sources.tests[""] = mediaSourceTest{ok: true}
	})
}

// shotMediaSources sets up two SMB sources as their Tests left them: one share, and one whole server, which listed its shares.
func shotMediaSources(a *app) {
	a.sources.list = []mediasource.Source{
		{ID: "0123456789abcdef", Kind: mediasource.KindSMB, Name: "Living Room NAS", Config: map[string]string{mediasource.SMBHost: "nas.local", mediasource.SMBShare: "media", mediasource.SMBRoot: "Films", mediasource.SMBUser: "kando"}},
		{ID: "fedcba9876543210", Kind: mediasource.KindSMB, Name: "Studio", Config: map[string]string{mediasource.SMBHost: "studio.lan", mediasource.SMBUser: "kando"}},
	}
	a.sources.tests["0123456789abcdef"] = mediaSourceTest{ok: true}
	a.sources.tests["fedcba9876543210"] = mediaSourceTest{ok: true, listed: true, shares: []string{"Anime", "Documentaries", "Movies"}}
}
