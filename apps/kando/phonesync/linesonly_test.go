package phonesync

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	psync "kando/packages/sync"
)

// linesSource is fakeSource's library with a film that keeps no video besides: its subtitles on disk, and nothing where its sound was.
type linesSource struct{ fakeSource }

func (f linesSource) Films(ctx context.Context, lib Library) ([]Film, error) {
	films, err := f.fakeSource.Films(ctx, lib)
	if err != nil {
		return nil, err
	}
	return append(films, Film{ID: "lines", Title: "Lines Only", Source: "youtube", SubtitlesOnly: true,
		Video: filepath.Join(f.dir, "lines.webm"), Subtitles: filepath.Join(f.dir, "lines.bilingual.srt")}), nil
}

// A film that keeps no video is listed, without a size, to a phone that asks for such films, and its lines and poster are served, but no video; a phone from before them, which does not ask, is listed the films with their video alone, and counted them alone.
func TestServesAFilmThatKeepsNoVideoToAPhoneThatAsks(t *testing.T) {
	lib := t.TempDir()
	for name, data := range map[string]string{"talk.mp4": "0123456789abcdefghij", "talk.bilingual.srt": "x", "lines.bilingual.srt": "1\n00:00:01,000 --> 00:00:02,000\n你好\nHello\n"} {
		if err := os.WriteFile(filepath.Join(lib, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	srv, err := Start(Config{Dir: t.TempDir(), Name: "Test Mac", Source: linesSource{fakeSource{dir: lib}}, Addrs: loopback, Logf: testLogf(t)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	ctx := context.Background()
	c := client(t, srv, "")
	res, err := pair(t, c, srv.NewOffer().Code)
	if err != nil {
		t.Fatal(err)
	}
	films, err := c.Films(ctx, "lib1")
	if err != nil || len(films) != 2 {
		t.Fatalf("films %+v, %v", films, err)
	}
	if f := films[1]; f.ID != "lines" || !f.SubtitlesOnly || f.VideoSize != 0 || f.Updated.IsZero() {
		t.Fatalf("film %+v", f)
	}
	if films[0].SubtitlesOnly {
		t.Fatalf("film %+v keeps its video", films[0])
	}
	cues, err := c.Subtitles(ctx, "lines")
	if err != nil || len(cues) != 1 {
		t.Fatalf("cues %+v, %v", cues, err)
	}
	_, err = c.Video(ctx, "lines", 0, -1, "")
	wantCode(t, err, http.StatusNotFound, psync.CodeNotFound)
	libs, err := c.Libraries(ctx)
	if err != nil || len(libs) != 1 || libs[0].Films != 1 {
		t.Fatalf("libraries %+v, %v", libs, err)
	}

	// The films' path as a phone from before films without a video asks for it.
	old := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	t.Cleanup(old.CloseIdleConnections)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+srv.Addrs()[0].String()+psync.FilmsPath("lib1"), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+res.Token)
	resp, err := old.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var listed psync.Films
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil || resp.StatusCode != http.StatusOK || len(listed.Films) != 1 || listed.Films[0].ID != "talk" {
		t.Fatalf("an older phone's films %d %+v, %v", resp.StatusCode, listed, err)
	}
}
