package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kando/apps/kando/account"
	"kando/apps/kando/films"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/mediasource"
	"kando/apps/kando/settings"
	"kando/apps/kando/shelf"
	"kando/packages/core"
	"kando/packages/remotefs"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsMediaSources renders 设置 › 媒体来源 and the dialog it opens: two SMB sources, a share whose Test connected and a whole server whose Test listed its shares, and an SFTP server (settings-sources); the dialog adding an SMB source of a NAS without signing, with what that risks (settings-sources-add); the dialog adding an SFTP source whose server's key the Test trusted (settings-sources-add-sftp); the question the first Test of an SFTP source asks about its server's key (settings-sources-trust); the dialog with Kando's official Plex plugin chosen as the kind before it is installed, which offers to install it (settings-sources-add-offer); and the dialog adding a WebDAV source at an http:// address, which says its password crosses the network unencrypted (settings-sources-add-webdav). The servers are names in the scene, never reached.
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
		a.mediaSourceAsk(mediaSourceAlert{kind: account.AlertTrust, hk: &remotefs.HostKeyError{Host: "backup.lan", Key: shotHostKey(7), Fingerprint: remotefs.Fingerprint(shotHostKey(7))}})
	})
	shootSettings(t, f, settings.SectionMediaSources, "settings-sources-add-offer", shown, models, turbo, func(a *app) {
		shotMediaSources(a)
		a.AddMediaSource()
		a.SetMediaSourceKind(mediaSourceInstallPrefix + "kando.plex")
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

// TestScreenshotsMediaSourcesBrowse renders the 媒体来源 page beside the sidebar: before any source was added, what a source is and 添加来源… (media-sources-none); a folder of a share with a video that has subtitles, one chosen and one already in the library (media-sources-browse); the root of a whole server, which lists its shares, one of which two films were imported from (media-sources-shares); and the dialog importing two videos chosen, copied (media-sources-import) or indexed and played from the source, the choice remembered (media-sources-import-link). The sources are a fake in memory, never reached.
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
	f.shoot(t, "media-sources-none", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		onEngine(a, func() { a.Select(pageMediaSources) })
		refresh(tt, a)
		screenshotWait(t, tt, a, updates, "the page", func() bool { return a.sources.browse.shown })
	})
	f.shoot(t, "media-sources-shares", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		open(t, a, tt, updates, server.ID, "")
		onEngine(a, func() {
			for _, name := range []string{"Akira (1988)", "Perfect Blue (1997)"} {
				ref := mediasource.Ref{Source: server.ID, Path: "Anime/" + name + ".mkv"}.String()
				a.library.Jobs = append(a.library.Jobs, core.Job{ID: "anime-" + name, Source: ref, Title: name, Stage: core.StageComplete, LibraryDir: f.lib,
					VideoPath: filepath.Join(f.lib, "Studio", "Anime", name+".mkv")})
			}
		})
	})
	f.shoot(t, "media-sources-import", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		open(t, a, tt, updates, share.ID, "Movies")
		onEngine(a, func() {
			a.mediaSourceToggle("Movies/Heat (1995).mkv")
			a.mediaSourceToggle("Movies/Collateral (2004).mkv")
			a.mediaSourceAskImport()
		})
	})
	f.shoot(t, "media-sources-import-link", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		open(t, a, tt, updates, share.ID, "Movies")
		onEngine(a, func() {
			a.mediaSourceToggle("Movies/Heat (1995).mkv")
			a.mediaSourceToggle("Movies/Collateral (2004).mkv")
			a.mediaSourceAskImport()
			a.ChooseMediaBrowseImportLink(true)
			a.RememberMediaBrowseImport(true)
		})
	})
}

// TestScreenshotsMediaSourcesPlay renders the film page of a film indexed from a media source: its source cannot be reached, which the page says with 重试 and the settings in place of the picture (media-sources-unreachable); and its file changed on the source since its lines were made, which the bar over the picture says with 重新做字幕 and 知道了 (media-sources-changed). The source is a fake in memory, never reached over a network.
func TestScreenshotsMediaSourcesPlay(t *testing.T) {
	f := newScreenshotFixture(t)
	share := mediasource.Source{ID: "0123456789abcdef", Kind: mediasource.KindSMB, Name: "Living Room NAS", Config: map[string]string{mediasource.SMBHost: "nas.local", mediasource.SMBShare: "media", mediasource.SMBUser: "kando"}}
	// index makes j3's film one indexed from share, its video moved aside for the scene, as an indexed film's is not on disk, and put back as it ends; src is the share as the scene finds it.
	index := func(t *testing.T, a *app, src *fakeSource) films.Film {
		item := f.film(t, a, "j3")
		aside := item.VideoPath + ".aside"
		if err := os.Rename(item.VideoPath, aside); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.Rename(aside, item.VideoPath); err != nil {
				t.Errorf("the scene's video was not put back: %v", err)
			}
		})
		onEngine(a, func() {
			a.sources.list = []mediasource.Source{share}
			a.sources.open = src.open
			a.library.Linked = map[string]shelf.Linked{item.VideoPath: {Source: share.ID, Path: "Talks/Rust.mp4", Size: 9, Version: "v1", Duration: 3 * time.Minute}}
			a.refreshLibrary()
		})
		return item
	}
	f.shoot(t, "media-sources-unreachable", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		src := newFakeSource(nil)
		src.unreachable = errors.New("dial tcp: connection refused")
		item := index(t, a, src)
		onEngine(a, func() { a.Open(must(films.Find(a.films(), item.Key))) })
		screenshotWait(t, tt, a, updates, "the source's answer", func() bool { return a.player.playErr != "" })
		screenshotWait(t, tt, a, updates, "the film's lines", func() bool { return a.player.subs.ready })
		wantText(t, tt, testlocale.Messages().MediaSourceUnreachable("Living Room NAS"))
	})
	f.shoot(t, "media-sources-changed", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		src := newFakeSource(map[string]fakeSourceFile{"Talks/Rust.mp4": {data: "cut anew, longer"}})
		item := index(t, a, src)
		onEngine(a, func() { a.Open(must(films.Find(a.films(), item.Key))) })
		screenshotWait(t, tt, a, updates, "the source's answer", func() bool { return a.player.linkedChanged != nil || a.player.playErr != "" })
		// TODO(nsgo#1057): show the film's own picture, as MyGo's player showed the frames ffmpeg decoded, once Kando plays what ffmpeg decodes (playerRemux in playback.go), which waits on an NSGo audio output fed decoded sound; the system's player cannot play the fake source's bytes, so the stage shows the film's poster under the bar https://github.com/nsgo-dev/nsgo/issues/1057
		onEngine(a, func() {
			a.player.playErr = ""
			a.player.play.source = ""
		})
		screenshotWait(t, tt, a, updates, "the film's lines", func() bool { return a.player.subs.ready })
		wantText(t, tt, testlocale.Messages().MediaSourceChanged("Living Room NAS"))
	})
}
