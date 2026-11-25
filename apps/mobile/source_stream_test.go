package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"kando/apps/mobile/nas"
	"kando/apps/mobile/store"
	"kando/packages/remotefs/davtest"
	"kando/packages/remotefs/sftptest"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// sftpFilms is an sftptest server whose home holds Films, with the talk's video as Holiday.mp4 and a folder Trip with Beach.mov, and the source of that folder, its key trusted.
func sftpFilms(t *testing.T) (*sftptest.Server, store.Source) {
	t.Helper()
	srv := sftptest.New(t)
	if err := os.MkdirAll(filepath.Join(srv.Home, "Films/Trip"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string][]byte{"Films/Holiday.mp4": talkVideo, "Films/Trip/Beach.mov": []byte("beach")} {
		if err := os.WriteFile(filepath.Join(srv.Home, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	host, port := srv.HostPort()
	return srv, store.Source{ID: "0123456789abcdef", Kind: nas.KindSFTP, Name: "Home NAS", Config: map[string]string{
		nas.Host: host, nas.Port: strconv.Itoa(port), nas.Root: "Films", nas.User: sftptest.User, nas.HostKey: srv.HostKey}}
}

// fetchRange is what the player reads at link asking for the bytes from to to: its status and body.
func fetchRange(t *testing.T, link string, from, to int) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, link, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", from, to))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

// openSource opens the folder of the source id from the Library's NAS group.
func (p *phone) openSource(id string) {
	p.t.Helper()
	p.app.WaitFor("id:films-source-"+id, wait)
	p.app.Press("id:films-source-" + id)
	p.app.WaitFor("id:browse-video-Holiday.mp4", wait)
}

// A video of a source plays at once as its row is pressed, nothing of it downloaded: the player opens a URL of the phone's loopback server, which serves the file's bytes from the source, a range of them as the player asks, and the film is kept to stream under the key a download of it takes.
func TestASourceVideoStreamsAtOnce(t *testing.T) {
	t.Run(nas.KindWebDAV, func(t *testing.T) {
		dir := t.TempDir()
		src := davSource(davFilms(t, false))
		streamsHoliday(t, startPhone(t, dir, keepSource(t, dir, src, davtest.Password)), dir, src)
	})
	t.Run(nas.KindSFTP, func(t *testing.T) {
		dir := t.TempDir()
		_, src := sftpFilms(t)
		streamsHoliday(t, startPhone(t, dir, keepSource(t, dir, src, sftptest.Password)), dir, src)
	})
}

// streamsHoliday presses Holiday.mp4 in the folder of src and checks what the player plays.
func streamsHoliday(t *testing.T, p *phone, dir string, src store.Source) {
	t.Helper()
	p.openSource(src.ID)
	p.app.Press("id:browse-video-Holiday.mp4")
	p.app.WaitFor("id:player-video", wait)
	link := p.playerSource()
	if !strings.HasPrefix(link, "http://127.0.0.1:") || !strings.HasSuffix(link, "/Holiday.mp4") {
		t.Fatalf("the player plays %q", link)
	}
	if code, body := fetch(t, link); code != http.StatusOK || body != string(talkVideo) {
		t.Fatalf("the URL answered %d, %d bytes", code, len(body))
	}
	if code, body := fetchRange(t, link, 500_000, 500_099); code != http.StatusPartialContent || body != string(talkVideo[500_000:500_100]) {
		t.Fatalf("a range of the URL answered %d %q", code, body)
	}
	id := store.SourceFilmID("Holiday.mp4")
	filmDir := filepath.Join(dir, "films", store.SourceOwner(src.ID), id)
	p.app.WaitUntil(wait, func() bool { _, err := os.Stat(filepath.Join(filmDir, "film.json")); return err == nil })
	films, err := store.Open(dir).Films()
	if err != nil || len(films) != 1 {
		t.Fatalf("the phone keeps %v, %v", films, err)
	}
	if f := films[0]; !f.Streamed || f.Source != src.ID || f.Path != "Holiday.mp4" || f.Film.ID != id || f.Film.Title != "Holiday" || f.HasVideo() {
		t.Errorf("the film is kept as %+v", f)
	}
	for _, name := range []string{"video.mp4", store.PartFile} {
		if _, err := os.Stat(filepath.Join(filmDir, name)); err == nil {
			t.Errorf("streaming wrote %s", name)
		}
	}
}

// The URL a film streams from and its connection to the source last while its player holds it: once another film plays, the URL is 404 and the connection is closed.
func TestASourceStreamEndsWithItsPlayer(t *testing.T) {
	srv, src := sftpFilms(t)
	dir := t.TempDir()
	p := startPhone(t, dir, keepSource(t, dir, src, sftptest.Password))
	p.openSource(src.ID)
	// The folder's listing holds a connection of its own.
	listing := srv.Open()
	p.app.Press("id:browse-video-Holiday.mp4")
	p.app.WaitFor("id:player-video", wait)
	link := p.playerSource()
	if code, _ := fetchRange(t, link, 0, 9); code != http.StatusPartialContent {
		t.Fatalf("the URL answered %d", code)
	}
	if got := srv.Open(); got != listing+1 {
		t.Fatalf("the server holds %d connections while the film streams, want %d", got, listing+1)
	}

	p.app.Press("id:player-minimize")
	p.app.WaitFor("id:browse-folder-Trip", wait)
	p.app.Press("id:browse-folder-Trip")
	p.app.WaitFor("id:browse-video-Beach.mov", wait)
	p.app.Press("id:browse-video-Beach.mov")
	p.app.WaitUntil(wait, func() bool { return strings.HasSuffix(p.playerSource(), "/Beach.mov") })
	if code, _ := fetch(t, link); code != http.StatusNotFound {
		t.Errorf("the URL of the film no longer playing answered %d", code)
	}
	p.app.WaitUntil(wait, func() bool { return srv.Open() == listing })
	if code, body := fetch(t, p.playerSource()); code != http.StatusOK || body != "beach" {
		t.Errorf("the film playing now answered %d %q", code, body)
	}
}

// wantSourceFailed waits for the player to say it could not play the film from src, which cannot be reached, and why.
func (p *phone) wantSourceFailed(src store.Source) {
	p.t.Helper()
	p.app.WaitFor("id:player-failed-why", wait)
	if got := p.texts("id:player-failed"); got != msgs.PlayerSourceFailed("Home NAS") {
		p.t.Errorf("the player says %q", got)
	}
	if got := p.texts("id:player-failed-why"); got != msgs.SourceFailUnreachable(nas.HostOf(src)) {
		p.t.Errorf("the player says why as %q", got)
	}
}

// A source that cannot be reached as the player opens a film of it says so over the player, with why.
func TestASourceUnreachableAtTheStartSaysSo(t *testing.T) {
	f := davFilms(t, false)
	dir := t.TempDir()
	src := davSource(f)
	p := startPhone(t, dir, keepSource(t, dir, src, davtest.Password))
	p.openSource(src.ID)
	// The NAS is switched off after its folder was read.
	f.Close()
	p.app.Press("id:browse-video-Holiday.mp4")
	p.app.WaitFor("id:player-video", wait)
	if code, _ := fetchRange(t, p.playerSource(), 0, 9); code != http.StatusBadGateway {
		t.Fatalf("the URL of a source switched off answered %d", code)
	}
	p.wantSourceFailed(src)
}

// A source that goes away midway says so over the player, with why, as the next range the player asks for fails.
func TestASourceUnreachableMidwaySaysSo(t *testing.T) {
	f := davFilms(t, false)
	dir := t.TempDir()
	src := davSource(f)
	p := startPhone(t, dir, keepSource(t, dir, src, davtest.Password))
	p.openSource(src.ID)
	p.app.Press("id:browse-video-Holiday.mp4")
	p.app.WaitFor("id:player-video", wait)
	link := p.playerSource()
	if code, _ := fetchRange(t, link, 0, 9); code != http.StatusPartialContent {
		t.Fatalf("the URL answered %d", code)
	}
	if p.app.Exists("id:player-failed") {
		t.Fatal("the player says it failed while the source answers")
	}
	f.Close()
	if code, _ := fetchRange(t, link, 600_000, 600_099); code != http.StatusBadGateway {
		t.Fatalf("the URL of a source switched off answered %d", code)
	}
	p.wantSourceFailed(src)
}

// A video downloaded from a source plays from the phone, though it was streamed first, and its download goes on from where its stream was left: a film streamed and downloaded is one film, whose position the download keeps.
func TestADownloadedSourceFilmPlaysTheLocalFile(t *testing.T) {
	f := davFilms(t, false)
	dir := t.TempDir()
	src := davSource(f)
	p := startPhone(t, dir, keepSource(t, dir, src, davtest.Password))
	p.openSource(src.ID)
	p.app.Press("id:browse-video-Holiday.mp4")
	p.app.WaitFor("id:player-video", wait)
	streamed := p.playerSource()
	if !strings.HasPrefix(streamed, "http://127.0.0.1:") {
		t.Fatalf("the player plays %q", streamed)
	}
	p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * sec})
	p.at(6 * sec)
	p.app.Press("id:player-minimize")
	p.app.WaitUntil(wait, func() bool { return p.keptPosition() == 6000 })

	p.app.WaitFor("id:browse-download-Holiday.mp4", wait)
	p.app.Press("id:browse-download-Holiday.mp4")
	id := store.SourceFilmID("Holiday.mp4")
	filmDir := filepath.Join("films", store.SourceOwner(src.ID), id)
	p.waitFile(filepath.Join(filmDir, "video.mp4"), int64(len(talkVideo)))
	p.app.WaitFor("id:browse-downloaded-Holiday.mp4", wait)
	films, _ := store.Open(dir).Films()
	if len(films) != 1 || films[0].Streamed || !films[0].HasVideo() || films[0].PositionMS != 6000 {
		t.Fatalf("the phone keeps %+v", films)
	}
	p.app.Press("id:browse-video-Holiday.mp4")
	p.app.WaitUntil(wait, func() bool { return strings.HasSuffix(p.playerSource(), filepath.Join(filmDir, "video.mp4")) })
}

// A source removed while a film of it streams is read no more: the connection made before is closed at the player's next request, which fails, and the player says the source was removed.
func TestASourceRemovedMidwayStopsItsStream(t *testing.T) {
	f := davFilms(t, false)
	dir := t.TempDir()
	src := davSource(f)
	p := startPhone(t, dir, keepSource(t, dir, src, davtest.Password), nsgotest.PendingAlerts())
	p.openSource(src.ID)
	p.app.Press("id:browse-video-Holiday.mp4")
	p.app.WaitFor("id:player-video", wait)
	link := p.playerSource()
	if code, _ := fetchRange(t, link, 0, 9); code != http.StatusPartialContent {
		t.Fatalf("the URL answered %d", code)
	}
	p.app.Press("id:player-minimize")
	p.openSettingsSources()
	p.app.Press("id:sources-source-" + src.ID)
	p.app.WaitFor("id:source-remove", wait)
	p.app.Press("id:source-remove")
	p.app.WaitUntil(wait, func() bool { return len(p.app.Alerts()) == 1 })
	p.app.Answer(1)
	p.app.WaitUntil(wait, func() bool { list, _ := store.Open(dir).Sources(); return len(list) == 0 })
	if code, _ := fetchRange(t, link, 10, 19); code != http.StatusBadGateway {
		t.Fatalf("the URL of a source removed answered %d", code)
	}
	p.app.Press("id:player-mini-open")
	p.app.WaitFor("id:player-failed-why", wait)
	if got := p.texts("id:player-failed-why"); got != msgs.SourceFailGone() {
		t.Errorf("the player says why as %q", got)
	}
}
