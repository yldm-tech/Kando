package store

import (
	"bytes"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
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
