package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"kando/apps/kando/account"
	"kando/apps/kando/cloud"
	"kando/apps/kando/film"
	"kando/apps/kando/films"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/settings"
	"kando/apps/kando/shelf"
	"kando/apps/kando/storage"
	"kando/packages/remotefs"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsStoredVideos renders moving a film's video to a storage location: ⋯ on the first poster with 移动视频到 and the two NAS locations of the settings (library-video-move-menu; the menu is the system's, which the tester lists rather than draws, so the picture shows the button pressed open), the message bar while the video is copied, with 取消 (library-video-moving), and the film page of a film whose video is on a NAS that cannot be reached, its lines listed beside the note (film-video-unreachable), and the marks of videos moved to Home NAS, to the R2 object store and to a location since removed from the settings, on the posters (library-video-stored-grid) and in the list (library-video-stored-list). The locations are names in the scene, never reached. It is skipped when KANDO_SCREENSHOTS is unset.
func TestScreenshotsStoredVideos(t *testing.T) {
	f := newScreenshotFixture(t)
	locations := func(a *app) {
		a.locations.list = []account.Location{
			{ID: "nas", Name: "Home NAS", Remote: remotefs.Remote{Kind: remotefs.KindSMB, Host: "nas.local", Share: "media", Root: "films", User: "reader", Password: "secret"}},
			{ID: "dav", Name: "Nextcloud", Remote: remotefs.Remote{Kind: remotefs.KindWebDAV, URL: "https://cloud.example.com/remote.php/dav/files/reader", User: "reader", Password: "secret"}},
		}
		a.stored.pool = storage.NewPool(func(context.Context, storage.Target) (remotefs.Provider, io.Closer, error) {
			return nil, nil, errors.New("dial tcp: connection refused")
		})
	}
	// stored records the videos of the first three films shown as moved: to Home NAS, to the R2 object store set up in 账号与上传, and to a location since removed from the settings, whose mark is the warning. The videos stay on disk; the marks read library.json's records and the settings alone.
	stored := func(t *testing.T, a *app) {
		a.cloud.r2Endpoint, a.cloud.r2Bucket, a.cloud.r2AccessKey, a.cloud.r2Secret = "acct", "clips", "key", "secret"
		shown := a.libraryShownFilms()
		if len(shown) < 3 {
			t.Fatalf("%d films shown", len(shown))
		}
		a.library.Stored = map[string]shelf.Stored{}
		for i, place := range []string{"nas", storedStorePrefix + cloud.KindR2, "old-nas"} {
			a.library.Stored[shown[i].VideoPath] = shelf.Stored{Location: place, Path: "Kando/films/video.mp4", Size: 1}
		}
	}
	// The grid scrolled as library-scrolled-grid is, with the message bar the prototype shows after its onboarding, and the marks on the first three posters.
	f.shoot(t, "library-video-stored-grid", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		locations(a)
		*a.Notice() = films.Notice{Kind: "warning", Text: "随时可以在「帮助 › 重新查看入门引导」回到这里。", Open: true}
		stored(t, a)
		refresh(tt, a)
		sel := whole(testlocale.Messages().FilmsGridCount(len(a.libraryShownFilms())))
		if !tt.Exists(sel) {
			t.Fatalf("no grid heading; texts %q", tt.Texts())
		}
		if d := pageFrame(tt.Find(sel)).Origin.Y - 315; d*d >= 0.25 {
			tt.Scroll(sel, d)
			refresh(tt, a)
		}
		if n := len(tt.Query("id:film-stored")); n != 3 {
			t.Fatalf("%d marks on the posters, want 3", n)
		}
	})
	// The list as library-list shows it, sorted by length, with the marks after the first three names.
	f.shoot(t, "library-video-stored-list", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		locations(a)
		a.SetLibraryView(films.ViewList)
		a.SetLibrarySort(films.Order{By: films.SortDuration})
		stored(t, a)
		refresh(tt, a)
		// Mounted rather than in view: every film lasts 3:00, so which rows the sort puts last, at the window's foot, differs from machine to machine.
		if n := len(tt.QueryMounted("id:films-list >> id:film-stored")); n != 3 {
			t.Fatalf("%d marks in the list, want 3", n)
		}
	})
	f.shoot(t, "library-video-move-menu", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		locations(a)
		refresh(tt, a)
		tt.Press("id:film-more >> nth=0")
		for _, it := range tt.PopupMenu {
			if it.Label == testlocale.Messages().StoredMoveTo() && len(it.Items) == 2 && it.Items[0].Label == "Home NAS" {
				return
			}
		}
		t.Fatalf("⋯ has no 移动视频到 with the locations: %+v", tt.PopupMenu)
	})
	f.shoot(t, "library-video-moving", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		locations(a)
		item := f.film(t, a, "j3")
		a.stored.run = &storedRun{video: item.VideoPath, name: films.Name(item), place: "nas", cancel: func() {}}
		a.storedShow()
		a.stored.run.percent = 42
		a.storedShow()
		if want := testlocale.Messages().StoredMovingTo(films.Name(item), "Home NAS", 42); a.lib.notice.Text != want || !a.lib.notice.Stop {
			t.Fatalf("the message bar says %+v", a.lib.notice)
		}
	})
	f.shoot(t, "film-video-unreachable", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		locations(a)
		item := f.film(t, a, "j3")
		// The video is moved aside for the scene, as it is not on disk once moved, and put back as it ends.
		aside := item.VideoPath + ".aside"
		if err := os.Rename(item.VideoPath, aside); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.Rename(aside, item.VideoPath); err != nil {
				t.Errorf("the scene's video was not put back: %v", err)
			}
		})
		if a.library.Stored == nil {
			a.library.Stored = map[string]shelf.Stored{}
		}
		a.library.Stored[item.VideoPath] = shelf.Stored{Location: "nas", Path: "Kando/films/video.mp4", Size: 1}
		onEngine(a, func() { a.Open(must(films.Find(a.films(), "j3"))) })
		screenshotWait(t, tt, a, updates, "the location's answer", func() bool { return a.player.playErr != "" })
		if a.player.playErr != playerErrStoredUnreachable {
			t.Fatalf("the player says %q", a.player.playErr)
		}
	})
}

// TestScreenshotsStoredSeveral renders what this file's earlier scenes do not: the menu of three posters selected, with 移动视频到 and the locations (library-video-move-several-menu; the menu is the system's, which the tester lists rather than draws), the message bar while the second of their videos is copied (library-video-moving-several), the film page while the video of a film on Home NAS, which the system's player cannot open, is copied for it from there (film-video-remux-stored), and of one in the library's folder (film-video-remux-local), and the confirmation of removing Home NAS while three films' videos are on it (settings-account-location-remove-stored). The locations are names in the scene, never reached. It is skipped when KANDO_SCREENSHOTS is unset.
func TestScreenshotsStoredSeveral(t *testing.T) {
	f := newScreenshotFixture(t)
	locations := func(a *app) {
		a.locations.list = []account.Location{
			{ID: "nas", Name: "Home NAS", Remote: remotefs.Remote{Kind: remotefs.KindSMB, Host: "nas.local", Share: "media", Root: "films", User: "reader", Password: "secret"}},
			{ID: "dav", Name: "Nextcloud", Remote: remotefs.Remote{Kind: remotefs.KindWebDAV, URL: "https://cloud.example.com/remote.php/dav/files/reader", User: "reader", Password: "secret"}},
		}
		a.stored.pool = storage.NewPool(func(context.Context, storage.Target) (remotefs.Provider, io.Closer, error) {
			return nil, nil, errors.New("dial tcp: connection refused")
		})
	}
	// The first three posters selected, as library-selected-grid shows them, the grid scrolled up so they show whole when scroll is set.
	selectThree := func(t *testing.T, a *app, tt *nsgotest.App, scroll bool) []films.Film {
		shown := a.libraryShownFilms()
		if len(shown) < 3 {
			t.Fatalf("%d films shown", len(shown))
		}
		a.lib.selection.All(films.Keys(shown)[:3])
		refresh(tt, a)
		sel := whole(testlocale.Messages().FilmsGridCount(len(shown)))
		if d := pageFrame(tt.Find(sel)).Origin.Y - 315; scroll && d*d >= 0.25 {
			tt.Scroll(sel, d)
			refresh(tt, a)
		}
		return shown[:3]
	}
	f.shoot(t, "library-video-move-several-menu", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		locations(a)
		refresh(tt, a)
		selectThree(t, a, tt, true)
		tt.Press("id:film-more >> nth=0")
		m := testlocale.Messages()
		for _, it := range tt.PopupMenu {
			if it.Label == m.StoredMoveTo() && len(it.Items) == 2 && it.Items[0].Label == "Home NAS" && it.Items[1].Label == "Nextcloud" {
				if it := tt.PopupMenu[0]; it.Label != m.FilmsSelectedCount(3) {
					t.Fatalf("the menu is not the selection's: %+v", tt.PopupMenu)
				}
				return
			}
		}
		t.Fatalf("the selection's menu has no 移动视频到 with the locations: %+v", tt.PopupMenu)
	})
	f.shoot(t, "library-video-moving-several", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		locations(a)
		refresh(tt, a)
		three := selectThree(t, a, tt, false)
		b := &storedBatch{items: three[2:], place: "nas", at: 2, total: 3, moved: 1}
		a.stored.run = &storedRun{video: three[1].VideoPath, name: films.Name(three[1]), place: "nas", cancel: func() {}, batch: b}
		a.storedShow()
		a.stored.run.percent = 42
		a.storedShow()
		refresh(tt, a)
		if want := testlocale.Messages().StoredMovingToMany(2, 3, "Home NAS", 42); a.lib.notice.Text != want || !a.lib.notice.Stop {
			t.Fatalf("the message bar says %+v", a.lib.notice)
		}
		wantText(t, tt, testlocale.Messages().StoredMovingToMany(2, 3, "Home NAS", 42))
	})
	f.shoot(t, "film-video-remux-stored", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		locations(a)
		item := f.film(t, a, "j3")
		// The video is moved aside for the scene, as it is not on disk once moved, and put back as it ends.
		aside := item.VideoPath + ".aside"
		if err := os.Rename(item.VideoPath, aside); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.Rename(aside, item.VideoPath); err != nil {
				t.Errorf("the scene's video was not put back: %v", err)
			}
		})
		if a.library.Stored == nil {
			a.library.Stored = map[string]shelf.Stored{}
		}
		a.library.Stored[item.VideoPath] = shelf.Stored{Location: "nas", Path: "Kando/films/video.mp4", Size: 1, Duration: 3 * time.Minute}
		onEngine(a, func() { a.Open(must(films.Find(a.films(), "j3"))) })
		screenshotWait(t, tt, a, updates, "the location's answer", func() bool { return a.player.playErr != "" })
		// The location is never reached in the scene: the page is set as it is while ffmpeg copies the video from there, 42% of its length done.
		onEngine(a, func() {
			p := a.player.play
			a.player.playErr = ""
			p.storedURL, p.remux, p.source = "http://127.0.0.1:1/v", film.RemuxCopy, ""
			p.remuxLength, p.remuxAt = 3*time.Minute, 3*time.Minute*42/100
		})
		screenshotWait(t, tt, a, updates, "the film's lines", func() bool { return a.player.subs.ready })
		wantText(t, tt, testlocale.Messages().PlayerRemuxingFrom("Home NAS", 42))
	})
	// The same for a video in the library's folder the system's player cannot open, copied for it from the file.
	f.shoot(t, "film-video-remux-local", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		screenshotOpen(t, a, tt, updates, "j3", 0)
		onEngine(a, func() {
			p := a.player.play
			p.remux, p.source, p.loaded = film.RemuxCopy, "", false
			p.remuxLength, p.remuxAt = 3*time.Minute, 3*time.Minute*42/100
		})
		refresh(tt, a)
		wantText(t, tt, testlocale.Messages().PlayerRemuxing(42))
	})
	shown, models, turbo := settingsShotFolders(t, f)
	shootSettings(t, f, settings.SectionAccount, "settings-account-location-remove-stored", shown, models, turbo, func(a *app) {
		shotLocations(a)
		a.library.Stored = map[string]shelf.Stored{}
		for i := range 3 {
			a.library.Stored[filepath.Join(shown, "Film", "video"+strconv.Itoa(i)+".mp4")] = shelf.Stored{Location: "nas", Path: "video.mp4", Size: 1}
		}
		a.askLocation(locationAlert{kind: account.AlertRemove, target: "nas"})
	})
}

// film is the shelf's film of job id.
func (f *screenshotFixture) film(t *testing.T, a *app, id string) films.Film {
	t.Helper()
	item, ok := films.Find(a.films(), id)
	if !ok {
		t.Fatalf("no %s on the shelf", id)
	}
	return item
}
