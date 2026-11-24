package main

import (
	"path/filepath"
	"testing"
	"time"

	"kando/apps/kando/account"
	"kando/apps/kando/mediasource"
	"kando/apps/kando/settings"
	"kando/apps/kando/storage"
	"kando/packages/core"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsMediaSources renders 设置 › 媒体来源 and the dialog it opens: two SMB sources, a share whose Test connected and a whole server whose Test listed its shares, and an SFTP server (settings-sources); the dialog adding an SMB source of a NAS without signing, with what that risks (settings-sources-add); the dialog adding an SFTP source whose server's key the Test trusted (settings-sources-add-sftp); the question the first Test of an SFTP source asks about its server's key (settings-sources-trust); and the dialog adding a WebDAV source at an http:// address, which says its password crosses the network unencrypted (settings-sources-add-webdav). The servers are names in the scene, never reached.
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
	sftp := func(a *app) {
		shotMediaSources(a)
		a.AddMediaSource()
		a.SetMediaSourceKind(mediasource.KindSFTP)
		d := a.MediaSourceDraft()
		d.Name = "Backup Server"
		d.Values = map[string]string{mediasource.SFTPHost: "backup.lan", mediasource.SFTPRoot: "/srv/video", mediasource.SFTPUser: "kando", mediasource.SFTPPassword: "secret"}
	}
	shootSettingsPressed(t, f, settings.SectionMediaSources, "settings-sources-add-sftp", shown, models, turbo, []string{"scroll-end:id:media-source-dialog-fields"}, func(a *app) {
		sftp(a)
		a.MediaSourceDraft().Values[mediasource.SFTPHostKey] = shotHostKey(7)
		a.sources.tests[""] = mediaSourceTest{ok: true}
	})
	shootSettings(t, f, settings.SectionMediaSources, "settings-sources-trust", shown, models, turbo, func(a *app) {
		sftp(a)
		a.mediaSourceAsk(mediaSourceAlert{kind: account.AlertTrust, hk: &storage.HostKeyError{Host: "backup.lan", Key: shotHostKey(7), Fingerprint: storage.Fingerprint(shotHostKey(7))}})
	})
	shootSettings(t, f, settings.SectionMediaSources, "settings-sources-add-webdav", shown, models, turbo, func(a *app) {
		shotMediaSources(a)
		a.AddMediaSource()
		a.SetMediaSourceKind(mediasource.KindWebDAV)
		d := a.MediaSourceDraft()
		d.Name = "Cloud Drive"
		d.Values = map[string]string{mediasource.WebDAVURL: "http://192.168.1.30:5005/dav", mediasource.WebDAVRoot: "Movies", mediasource.WebDAVUser: "kando", mediasource.WebDAVPassword: "secret"}
		a.sources.tests[""] = mediaSourceTest{ok: true}
	})
}

// shotMediaSources sets up three sources as their Tests left them: an SMB share, a whole SMB server, which listed its shares, and an SFTP server, whose key was trusted.
func shotMediaSources(a *app) {
	a.sources.list = []mediasource.Source{
		{ID: "0123456789abcdef", Kind: mediasource.KindSMB, Name: "Living Room NAS", Config: map[string]string{mediasource.SMBHost: "nas.local", mediasource.SMBShare: "media", mediasource.SMBRoot: "Films", mediasource.SMBUser: "kando"}},
		{ID: "fedcba9876543210", Kind: mediasource.KindSMB, Name: "Studio", Config: map[string]string{mediasource.SMBHost: "studio.lan", mediasource.SMBUser: "kando"}},
		{ID: "00112233aabbccdd", Kind: mediasource.KindSFTP, Name: "Backup Server", Config: map[string]string{mediasource.SFTPHost: "backup.lan", mediasource.SFTPRoot: "/srv/video", mediasource.SFTPUser: "kando", mediasource.SFTPHostKey: shotHostKey(7)}},
	}
	a.sources.tests["0123456789abcdef"] = mediaSourceTest{ok: true}
	a.sources.tests["fedcba9876543210"] = mediaSourceTest{ok: true, listed: true, shares: []string{"Anime", "Documentaries", "Movies"}}
}

// TestScreenshotsMediaSourcesBrowse renders the 媒体来源 page beside the sidebar: a folder of a share with a video that has subtitles, one chosen and one already in the library (media-sources-browse); the root of a whole server, which lists its shares (media-sources-shares); and the dialog importing two videos chosen (media-sources-import). The sources are a fake in memory, never reached.
func TestScreenshotsMediaSourcesBrowse(t *testing.T) {
	f := newScreenshotFixture(t)
	at := time.Date(2026, 10, 3, 21, 14, 0, 0, time.Local)
	files := map[string]fakeSourceFile{
		"Movies/Heat (1995).mkv":           {size: 4_612_734_976, modified: at},
		"Movies/Heat (1995).srt":           {data: "1", modified: at},
		"Movies/Ronin (1998).mp4":          {size: 2_147_483_648, modified: at.Add(-72 * time.Hour)},
		"Movies/The Insider (1999).mkv":    {size: 3_865_470_566, modified: at.Add(-240 * time.Hour)},
		"Movies/Collateral (2004).mkv":     {size: 3_221_225_472, modified: at.Add(-500 * time.Hour)},
		"Movies/Collateral (2004).en.srt":  {data: "1", modified: at},
		"Movies/Extras/Making of Heat.mp4": {size: 734_003_200, modified: at},
		"Movies/Thief (1981).avi":          {size: 1_395_864_371, modified: at.Add(-800 * time.Hour)},
		"Documentaries/Planet Ocean.mkv":   {size: 5_368_709_120, modified: at},
		"Anime/Akira (1988).mkv":           {size: 4_294_967_296, modified: at},
		"Concerts/Live at Budokan.mp4":     {size: 1_073_741_824, modified: at},
		"Movies/poster.jpg":                {size: 40_000, modified: at},
	}
	share := mediasource.Source{ID: "0123456789abcdef", Kind: mediasource.KindSMB, Name: "Living Room NAS", Config: map[string]string{mediasource.SMBHost: "nas.local", mediasource.SMBShare: "media", mediasource.SMBUser: "kando"}}
	server := mediasource.Source{ID: "fedcba9876543210", Kind: mediasource.KindSMB, Name: "Studio", Config: map[string]string{mediasource.SMBHost: "studio.lan", mediasource.SMBUser: "kando"}}
	setUp := func(a *app) {
		a.sources.list = []mediasource.Source{share, server}
		a.sources.open = newFakeSource(files).open
	}
	open := func(t *testing.T, a *app, tt *nsgotest.App, updates chan func(), source, dir string) {
		onEngine(a, func() {
			setUp(a)
			a.sources.browse.source, a.sources.browse.dir = source, dir
			a.Select(pageMediaSources)
		})
		refresh(tt, a)
		screenshotWait(t, tt, a, updates, "the folder", func() bool { return a.sources.browse.shown && !a.sources.browse.loading })
	}
	f.shoot(t, "media-sources-browse", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		open(t, a, tt, updates, share.ID, "Movies")
		onEngine(a, func() {
			ref := mediasource.Ref{Source: share.ID, Path: "Movies/Ronin (1998).mp4"}.String()
			a.library.Jobs = append(a.library.Jobs, core.Job{ID: "ronin", Source: ref, Title: "Ronin (1998)", Stage: core.StageComplete, LibraryDir: f.lib,
				VideoPath: filepath.Join(f.lib, "Living Room NAS", "Movies", "Ronin (1998).mp4")})
			a.mediaSourceToggle("Movies/Heat (1995).mkv")
		})
	})
	f.shoot(t, "media-sources-shares", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		open(t, a, tt, updates, server.ID, "")
	})
	f.shoot(t, "media-sources-import", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		open(t, a, tt, updates, share.ID, "Movies")
		onEngine(a, func() {
			a.mediaSourceToggle("Movies/Heat (1995).mkv")
			a.mediaSourceToggle("Movies/Collateral (2004).mkv")
			a.mediaSourceAskImport()
		})
	})
}
