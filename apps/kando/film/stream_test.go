package film

import (
	"slices"
	"strings"
	"testing"

	"kando/apps/kando/locales/testlocale"
)

// fakeStreamer is the app's side of a film that keeps no video: whether it streams here, and the calls the player made.
type fakeStreamer struct {
	streams     bool
	streamAsked []string
}

func (f *fakeStreamer) FilmStreams() bool { return f.streams }
func (f *fakeStreamer) DownloadVideo()    { f.streamAsked = append(f.streamAsked, "download") }
func (f *fakeStreamer) StreamOnline()     { f.streamAsked = append(f.streamAsked, "online") }
func (f *fakeStreamer) StreamAgain()      { f.streamAsked = append(f.streamAsked, "again") }

// subtitlesOnly is a done YouTube film that keeps no video, whose stream would not play for err.
func subtitlesOnly(err string) *fakeApp {
	a := newFakeApp()
	a.item.Source = "https://www.youtube.com/watch?v=jNQXAC9IVRw"
	a.item.SubtitlesOnly = true
	a.path, a.err, a.video = "", err, Video{}
	return a
}

// TestOfflineStopsAStreamedFilm checks the card over the picture of a film that keeps no video opened offline: it says it cannot play offline and where it would stream from, in place of the stage's note, and offers to turn offline mode off and to download the video, which says it is scheduled, and takes no press, once it is.
func TestOfflineStopsAStreamedFilm(t *testing.T) {
	a := subtitlesOnly(ErrStreamOffline)
	app := start(t, a)
	msgs := testlocale.Messages()
	if got := app.Find("id:film-stream-title"); got == nil || got.Text != msgs.PlayerStreamOffline() {
		t.Fatalf("card %q", app.Texts())
	}
	if got := app.Find("id:film-stream-detail"); got == nil || got.Text != msgs.PlayerStreamOfflineDetail("YouTube") {
		t.Fatalf("detail %q", app.Texts())
	}
	if app.Exists("id:film-stage-note") || app.Exists("id:film-stream-retry") {
		t.Fatalf("the stage says more: %q", app.Texts())
	}
	if got := app.Find("id:film-stream-download"); got == nil || got.Props.Label != msgs.PlayerStreamDownloadLater() {
		t.Fatalf("download button %+v", got)
	}
	app.Press("id:film-stream-online")
	app.Press("id:film-stream-download")
	if !slices.Equal(a.streamAsked, []string{"online", "download"}) {
		t.Fatalf("asked %v", a.streamAsked)
	}

	change(app, a, func() { a.item.VideoComing = true })
	if got := app.Find("id:film-stream-download"); got.Props.Label != msgs.PlayerStreamQueued() || app.Enabled("id:film-stream-download") {
		t.Fatalf("scheduled download button %+v", got.Props)
	}
}

// TestAFilmThatCannotStreamOffersItsVideo checks the card of a film that keeps no video and cannot stream here, as a bilibili one: it names the site and offers to download the video alone; and the card of a stream that failed offers to look it up again.
func TestAFilmThatCannotStreamOffersItsVideo(t *testing.T) {
	a := subtitlesOnly(ErrStreamNone)
	a.item.Source = "https://www.bilibili.com/video/BV1xx411c7mD"
	app := start(t, a)
	msgs := testlocale.Messages()
	if got := app.Find("id:film-stream-detail"); got == nil || got.Text != msgs.PlayerStreamNoneDetail(msgs.SourceBilibili()) {
		t.Fatalf("detail %q", app.Texts())
	}
	if app.Exists("id:film-stream-online") || app.Exists("id:film-stream-retry") {
		t.Fatalf("buttons %q", app.Texts())
	}
	if got := app.Find("id:film-stream-download"); got.Props.Label != msgs.PlayerStreamDownload() {
		t.Fatalf("download button %+v", got.Props)
	}

	change(app, a, func() { a.err = ErrStreamFailed })
	if got := app.Find("id:film-stream-title"); got.Text != msgs.PlayerStreamFailed() {
		t.Fatalf("title %q", got.Text)
	}
	app.Press("id:film-stream-retry")
	if !slices.Equal(a.streamAsked, []string{"again"}) {
		t.Fatalf("asked %v", a.streamAsked)
	}

	change(app, a, func() { a.err = "" })
	if app.Exists("id:film-stream") {
		t.Fatal("the card stays once the film plays")
	}
}

// TestInfoSaysAFilmKeepsNoVideo checks the card of 信息 for a film that keeps no video: it streams when watched where it can, and has to be downloaded first where it cannot; its button downloads the video and says once it is scheduled.
func TestInfoSaysAFilmKeepsNoVideo(t *testing.T) {
	msgs := testlocale.Messages()
	for _, streams := range []bool{true, false} {
		a := subtitlesOnly("")
		a.side, a.streams = "info", streams
		app := start(t, a)
		texts := strings.Join(app.Texts(), "\n")
		want, text := msgs.FilmInfoStreamTitle(), msgs.FilmInfoStream()
		if !streams {
			want, text = msgs.FilmsSubtitlesOnly(), msgs.FilmInfoStreamNone()
		}
		if !strings.Contains(texts, want) || !strings.Contains(texts, text) {
			t.Fatalf("streams %v: %s", streams, texts)
		}
		app.Press("id:film-info-download")
		if !slices.Equal(a.streamAsked, []string{"download"}) {
			t.Fatalf("asked %v", a.streamAsked)
		}
		change(app, a, func() { a.item.VideoComing = true })
		if texts := strings.Join(app.Texts(), "\n"); !strings.Contains(texts, msgs.FilmInfoStreamQueued()) || app.Enabled("id:film-info-download") {
			t.Fatalf("scheduled: %s", texts)
		}
	}

	a := newFakeApp()
	a.side = "info"
	if app := start(t, a); app.Exists("id:film-info-stream") {
		t.Fatal("a film with its video says it keeps none")
	}
}
