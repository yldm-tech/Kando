package main

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"kando/apps/kando/account"
	"kando/apps/kando/films"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/shelf"
	"kando/apps/kando/storage"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsStoredVideos renders moving a film's video to a storage location: ⋯ on the first poster with 移动视频到 and the two NAS locations of the settings (library-video-move-menu; the menu is the system's, which the tester lists rather than draws, so the picture shows the button pressed open), the message bar while the video is copied, with 取消 (library-video-moving), and the film page of a film whose video is on a NAS that cannot be reached, its lines listed beside the note (film-video-unreachable). The locations are names in the scene, never reached. It is skipped when KANDO_SCREENSHOTS is unset.
func TestScreenshotsStoredVideos(t *testing.T) {
	f := newScreenshotFixture(t)
	locations := func(a *app) {
		a.locations.list = []account.Location{
			{ID: "nas", Name: "Home NAS", Remote: storage.Remote{Kind: storage.KindSMB, Host: "nas.local", Share: "media", Root: "films", User: "reader", Password: "secret"}},
			{ID: "dav", Name: "Nextcloud", Remote: storage.Remote{Kind: storage.KindWebDAV, URL: "https://cloud.example.com/remote.php/dav/files/reader", User: "reader", Password: "secret"}},
		}
		a.stored.pool = storage.NewPool(func(context.Context, storage.Target) (storage.Provider, io.Closer, error) {
			return nil, nil, errors.New("dial tcp: connection refused")
		})
	}
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

// film is the shelf's film of job id.
func (f *screenshotFixture) film(t *testing.T, a *app, id string) films.Film {
	t.Helper()
	item, ok := films.Find(a.films(), id)
	if !ok {
		t.Fatalf("no %s on the shelf", id)
	}
	return item
}
