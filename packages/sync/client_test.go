package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

// desktop is a stand-in for Kando's server on loopback: it pairs code 123456, then answers the read paths for the token it gave.
func desktop(t *testing.T) (*httptest.Server, netip.AddrPort, []byte) {
	t.Helper()
	video := []byte("0123456789abcdefghij")
	authed := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer tok" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(ErrorBody{Code: CodeUnauthorized, Message: "pair first"})
				return
			}
			next(w, r)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+PathPair, func(w http.ResponseWriter, r *http.Request) {
		var req PairRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code != "123456" || req.Device != "Pixel" || req.Platform != PlatformAndroid {
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(ErrorBody{Code: CodeWrongCode})
			return
		}
		_ = json.NewEncoder(w).Encode(PairResponse{Token: "tok", DeviceID: "d1", Desktop: "Mac"})
	})
	mux.HandleFunc("GET "+PathLibraries, authed(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Libraries{Libraries: []Library{{ID: "lib", Name: "My Films", Films: 1}}})
	}))
	mux.HandleFunc("GET "+PathFilms, authed(func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("lib") != "lib" {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(ErrorBody{Code: CodeNotFound})
			return
		}
		_ = json.NewEncoder(w).Encode(Films{Films: []Film{{ID: "f1", Title: "Talk", Poster: PosterPath("f1"), VideoSize: int64(len(video))}}})
	}))
	mux.HandleFunc("GET "+PathSubtitles, authed(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Subtitles{Cues: []Cue{{StartMS: 1000, EndMS: 2500, Original: "Hello", Translation: "你好"}}})
	}))
	mux.HandleFunc("GET "+PathPoster, authed(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("jpeg"))
	}))
	mux.HandleFunc("GET "+PathVideo, authed(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Content-Type", "video/mp4")
		http.ServeContent(w, r, "talk.mp4", time.Unix(1, 0), bytes.NewReader(video))
	}))
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	addr := netip.MustParseAddrPort(strings.TrimPrefix(srv.URL, "https://"))
	return srv, addr, Fingerprint(srv.Certificate().Raw)
}

func TestClientPairsAndReads(t *testing.T) {
	_, addr, fp := desktop(t)
	ctx := context.Background()
	c := NewClient(addr, fp, "")
	defer c.Close()
	if _, err := c.Libraries(ctx); !isCode(err, http.StatusUnauthorized, CodeUnauthorized) {
		t.Fatalf("libraries before pairing: %v", err)
	}
	if _, err := c.Pair(ctx, PairRequest{Code: "000000", Device: "Pixel", Platform: PlatformAndroid}); !isCode(err, http.StatusForbidden, CodeWrongCode) {
		t.Fatalf("a wrong code: %v", err)
	}
	res, err := c.Pair(ctx, PairRequest{Code: "123456", Device: "Pixel", Platform: PlatformAndroid})
	if err != nil || res.Token != "tok" || res.DeviceID != "d1" || res.Desktop != "Mac" || c.Token != "tok" {
		t.Fatalf("pair: %+v, %v; token kept %q", res, err, c.Token)
	}
	libs, err := c.Libraries(ctx)
	if err != nil || len(libs) != 1 || libs[0].ID != "lib" || libs[0].Films != 1 {
		t.Fatalf("libraries %+v, %v", libs, err)
	}
	films, err := c.Films(ctx, "lib")
	if err != nil || len(films) != 1 || films[0].ID != "f1" || films[0].Poster != "/v1/films/f1/poster" {
		t.Fatalf("films %+v, %v", films, err)
	}
	if _, err := c.Films(ctx, "gone"); !isCode(err, http.StatusNotFound, CodeNotFound) {
		t.Fatalf("films of a library not served: %v", err)
	}
	cues, err := c.Subtitles(ctx, "f1")
	if err != nil || len(cues) != 1 || cues[0].Original != "Hello" || cues[0].Translation != "你好" || cues[0].StartMS != 1000 {
		t.Fatalf("cues %+v, %v", cues, err)
	}
	poster, kind, err := c.Poster(ctx, "f1")
	if err != nil || string(poster) != "jpeg" || kind != "image/jpeg" {
		t.Fatalf("poster %q %q, %v", poster, kind, err)
	}
	part, err := c.Video(ctx, "f1", 5, 9, "")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(part.Body)
	part.Body.Close()
	if string(body) != "56789" || part.Start != 5 || part.End != 9 || part.Total != 20 || part.ETag != `"v1"` || part.ContentType != "video/mp4" {
		t.Fatalf("part %q %+v", body, part)
	}
	rest, err := c.Video(ctx, "f1", 15, -1, part.ETag)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(rest.Body)
	rest.Body.Close()
	if string(body) != "fghij" || rest.Start != 15 || rest.End != 19 {
		t.Fatalf("the rest %q %+v", body, rest)
	}
	// A part asked for with the ETag of a file that changed since comes whole.
	whole, err := c.Video(ctx, "f1", 15, -1, `"v0"`)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(whole.Body)
	whole.Body.Close()
	if len(body) != 20 || whole.Start != 0 || whole.End != 19 || whole.Total != 20 {
		t.Fatalf("a changed file %q %+v", body, whole)
	}
}

func TestClientRefusesAnotherCertificate(t *testing.T) {
	_, addr, fp := desktop(t)
	other := Fingerprint([]byte("another certificate"))
	c := NewClient(addr, other, "tok")
	defer c.Close()
	if _, err := c.Libraries(context.Background()); !errors.Is(err, ErrFingerprint) {
		t.Fatalf("a desktop with another certificate: %v", err)
	}
	got, err := FetchFingerprint(context.Background(), addr)
	if err != nil || !bytes.Equal(got, fp) {
		t.Fatalf("fetched fingerprint %x, %v; want %x", got, err, fp)
	}
}

func isCode(err error, status int, code string) bool {
	var e *Error
	return errors.As(err, &e) && e.Status == status && e.Code == code
}

// A film's source key is an addition to v1: a desktop that has none for a film leaves it out of the JSON, so an older phone reads the same film as before; an older desktop's film, without it, reads as one of no key; and a phone that does not know SourceKey, as a phone before it, reads a newer desktop's film with the fields it knows.
func TestSourceKeyIsAnAddition(t *testing.T) {
	data, err := json.Marshal(Film{ID: "f1", Title: "River"})
	if err != nil || strings.Contains(string(data), "source_key") {
		t.Fatalf("a film without a source key reads %s, %v", data, err)
	}
	var f Film
	if err := json.Unmarshal([]byte(`{"id":"f1","title":"River","source":"local","duration_ms":0,"spoken":"","translation":"","untranslated":0,"poster":"","video_size":7,"updated":"0001-01-01T00:00:00Z"}`), &f); err != nil || f.SourceKey != "" || f.VideoSize != 7 {
		t.Fatalf("an older desktop's film reads %+v, %v", f, err)
	}
	data, err = json.Marshal(Film{ID: "f1", Title: "River", Source: "local", SourceKey: "6aTKFGnkWxhnQ8vrl6Wj-G"})
	if err != nil || !strings.Contains(string(data), `"source_key":"6aTKFGnkWxhnQ8vrl6Wj-G"`) {
		t.Fatalf("a film with a source key reads %s, %v", data, err)
	}
	var older struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		Source string `json:"source"`
	}
	if err := json.Unmarshal(data, &older); err != nil || older.ID != "f1" || older.Title != "River" || older.Source != "local" {
		t.Fatalf("an older phone reads %+v, %v", older, err)
	}
	if err := json.Unmarshal(data, &f); err != nil || f.SourceKey != "6aTKFGnkWxhnQ8vrl6Wj-G" {
		t.Fatalf("a phone reads %+v, %v", f, err)
	}
}
