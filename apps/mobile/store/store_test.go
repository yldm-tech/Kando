package store

import (
	"bytes"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"kando/packages/sync"
)

func TestDesktopsAreKeptPrivately(t *testing.T) {
	dir := t.TempDir()
	st := Open(dir)
	if got, err := st.Desktops(); err != nil || got != nil {
		t.Fatalf("before any pairing: %v, %v", got, err)
	}
	fp := sync.Fingerprint([]byte("cert"))
	want := Desktop{ID: DesktopID(fp), Name: "Studio", Addr: netip.MustParseAddrPort("192.168.1.20:47123"), Fingerprint: fp, Token: "tok", DeviceID: "d1", Paired: time.Unix(1_700_000_000, 0).UTC()}
	if err := st.SaveDesktops([]Desktop{want}); err != nil {
		t.Fatal(err)
	}
	got, err := Open(dir).Desktops()
	if err != nil || len(got) != 1 || got[0].Addr != want.Addr || !bytes.Equal(got[0].Fingerprint, fp) || got[0].Token != "" || !got[0].Paired.Equal(want.Paired) {
		t.Fatalf("read back %+v, %v", got, err)
	}
	// The token is the secure store's to keep (the session's), never the file's.
	if data, err := os.ReadFile(filepath.Join(dir, desktopsFile)); err != nil || bytes.Contains(data, []byte("tok")) {
		t.Fatalf("desktops.json holds the token: %s, %v", data, err)
	}
	if len(want.ID) != 16 {
		t.Errorf("DesktopID %q is not the short fingerprint's 16 letters", want.ID)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(filepath.Join(dir, desktopsFile)); err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("desktops.json: %v, %v", info.Mode(), err)
		}
	}
}

func TestFilmsAreListedNewestFirstAndCounted(t *testing.T) {
	st := Open(t.TempDir())
	if films, err := st.Films(); err != nil || len(films) != 0 {
		t.Fatalf("no films: %v, %v", films, err)
	}
	old := Film{Desktop: "d", Film: sync.Film{ID: "a"}, Added: time.Unix(100, 0)}
	recent := Film{Desktop: "d", Film: sync.Film{ID: "b"}, Added: time.Unix(200, 0), Video: "video.mp4"}
	for _, f := range []Film{old, recent} {
		if err := st.SaveFilm(f); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(st.FilmDir("d", "b"), "video.mp4"), make([]byte, 1000), 0o600); err != nil {
		t.Fatal(err)
	}
	films, err := st.Films()
	if err != nil || len(films) != 2 || films[0].Key() != "d/b" || !films[0].Done() || films[1].Done() {
		t.Fatalf("films %+v, %v", films, err)
	}
	used, err := st.Used()
	if err != nil || used < 1000 {
		t.Fatalf("used %d, %v", used, err)
	}
	if err := st.DeleteFilm("d", "b"); err != nil {
		t.Fatal(err)
	}
	if films, _ := st.Films(); len(films) != 1 || films[0].Key() != "d/a" {
		t.Fatalf("after deleting: %+v", films)
	}
}

// A film's record saved from several goroutines at once, as the player keeping its position and Follow keeping the desktop's record do, is each time saved whole: every save succeeds, the record reads back as one of them, and no temporary file is left in the film's folder.
func TestAFilmSavedFromSeveralGoroutinesAtOnceIsKeptWhole(t *testing.T) {
	st := Open(t.TempDir())
	const saves = 32
	errs := make(chan error, saves)
	for i := range saves {
		go func() {
			errs <- st.SaveFilm(Film{Desktop: "d", Film: sync.Film{ID: "a", Title: strings.Repeat("x", 4096)}, PositionMS: int64(i)})
		}()
	}
	for range saves {
		if err := <-errs; err != nil {
			t.Errorf("a save failed: %v", err)
		}
	}
	films, err := st.Films()
	if err != nil || len(films) != 1 || films[0].PositionMS < 0 || films[0].PositionMS >= saves {
		t.Fatalf("read back %+v, %v", films, err)
	}
	entries, err := os.ReadDir(st.FilmDir("d", "a"))
	if err != nil || len(entries) != 1 || entries[0].Name() != filmFile {
		t.Fatalf("the film's folder holds %v, %v", entries, err)
	}
}

// A record that is being read while it is saved is replaced once the reader is done with it. Windows refuses to replace a file another handle holds open, as Go's os.ReadFile does without letting it be deleted, so on Windows the save waits for the reader rather than failing and losing the record, as it once lost a played position (TestAFilmFollowedLateKeepsThePositionPlayedMeanwhile); elsewhere it replaces the file at once.
func TestARecordBeingReadIsReplacedOnceTheReaderIsDone(t *testing.T) {
	st := Open(t.TempDir())
	f := Film{Desktop: "d", Film: sync.Film{ID: "a"}, PositionMS: 1000}
	if err := st.SaveFilm(f); err != nil {
		t.Fatal(err)
	}
	reader, err := os.Open(filepath.Join(st.FilmDir("d", "a"), filmFile))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(100 * time.Millisecond)
		reader.Close()
	}()
	f.PositionMS = 7000
	if err := st.SaveFilm(f); err != nil {
		t.Fatalf("the save failed while the record was read: %v", err)
	}
	<-done
	if films, err := st.Films(); err != nil || len(films) != 1 || films[0].PositionMS != 7000 {
		t.Fatalf("read back %+v, %v", films, err)
	}
}

// The phone's own preferences start empty and read back as kept.
func TestPrefsAreKept(t *testing.T) {
	dir := t.TempDir()
	if got, err := Open(dir).Prefs(); err != nil || got.Onboarded || got.Target != "" || got.Spoken != nil {
		t.Fatalf("before any: %+v, %v", got, err)
	}
	want := Prefs{Onboarded: true, Spoken: []string{"en", "ja"}, Target: "zh-Hans"}
	if err := Open(dir).SavePrefs(want); err != nil {
		t.Fatal(err)
	}
	got, err := Open(dir).Prefs()
	if err != nil || !got.Onboarded || got.Target != "zh-Hans" || len(got.Spoken) != 2 || got.Spoken[1] != "ja" {
		t.Fatalf("read back %+v, %v", got, err)
	}
}

func TestLinksNotTakenYetAreKept(t *testing.T) {
	dir := t.TempDir()
	if got, err := Open(dir).Links(); err != nil || got != nil {
		t.Fatalf("before any link: %v, %v", got, err)
	}
	want := []Link{{ID: "c1", Desktop: "d1", URL: "https://youtu.be/abc", Added: time.Unix(1_700_000_000, 0).UTC()}}
	if err := Open(dir).SaveLinks(want); err != nil {
		t.Fatal(err)
	}
	if got, err := Open(dir).Links(); err != nil || len(got) != 1 || got[0].ID != "c1" || got[0].URL != want[0].URL || !got[0].Added.Equal(want[0].Added) {
		t.Fatalf("read back %+v, %v", got, err)
	}
}

// A video looked at again in Browse moves to the front, keeping its title when the page has not given one yet, and only the last MaxVisits are kept.
func TestVisitedKeepsTheLastVideosNewestFirst(t *testing.T) {
	var visits []Visit
	for _, u := range []string{"a", "b", "c", "d", "e", "f"} {
		visits = Visited(visits, Visit{URL: "https://example.com/" + u, Title: strings.ToUpper(u)})
	}
	if len(visits) != MaxVisits || visits[0].URL != "https://example.com/f" || visits[MaxVisits-1].URL != "https://example.com/b" {
		t.Fatalf("visits %+v", visits)
	}
	visits = Visited(visits, Visit{URL: "https://example.com/c"})
	if len(visits) != MaxVisits || visits[0] != (Visit{URL: "https://example.com/c", Title: "C"}) || visits[1].URL != "https://example.com/f" {
		t.Fatalf("looked at again: %+v", visits)
	}
	for i, v := range visits[1:] {
		if v.URL == "https://example.com/c" {
			t.Fatalf("visit %d is the one moved to the front: %+v", i+1, visits)
		}
	}
	// The oldest kept, looked at again before the page names it, keeps its title.
	oldest := visits[MaxVisits-1]
	visits = Visited(visits, Visit{URL: oldest.URL})
	if visits[0] != oldest || len(visits) != MaxVisits {
		t.Fatalf("the oldest looked at again: %+v", visits)
	}
}
