package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"kando/apps/kando/cloud"
	"kando/apps/kando/phonesync"
	"kando/apps/kando/settings"
	psync "kando/packages/sync"
)

// filmStore is an S3-compatible store in memory, path style on plain http, that keeps what it is given: a PUT stores, a GET or HEAD reads, a listing lists by prefix, and a DELETE deletes, which is all the relay and the films ask of a store whose files are small.
type filmStore struct {
	url string

	mu      sync.Mutex
	objects map[string][]byte
}

func newFilmStore(t *testing.T) *filmStore {
	t.Helper()
	s := &filmStore{objects: map[string][]byte{}}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	s.url = srv.URL
	return s
}

func (s *filmStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, key, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	type entry struct {
		Key  string
		Size int
	}
	switch {
	case r.Method == http.MethodGet && key == "":
		var out struct {
			XMLName  xml.Name `xml:"ListBucketResult"`
			Contents []entry
		}
		for k, body := range s.objects {
			if strings.HasPrefix(k, r.URL.Query().Get("prefix")) {
				out.Contents = append(out.Contents, entry{Key: k, Size: len(body)})
			}
		}
		slices.SortFunc(out.Contents, func(a, b entry) int { return strings.Compare(a.Key, b.Key) })
		w.Header().Set("Content-Type", "application/xml")
		_ = xml.NewEncoder(w).Encode(out)
	case r.Method == http.MethodPut:
		body, _ := io.ReadAll(r.Body)
		s.objects[key] = body
		w.Header().Set("ETag", fmt.Sprintf(`"%d"`, len(body)))
	case r.Method == http.MethodDelete:
		delete(s.objects, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		body, ok := s.objects[key]
		if !ok {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			if r.Method == http.MethodGet {
				_, _ = io.WriteString(w, `<Error><Code>NoSuchKey</Code><Message>none</Message></Error>`)
			}
			return
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		if r.Method == http.MethodGet {
			_, _ = w.Write(body)
		}
	}
}

// films are the names of the films' objects the store holds, below the relay's folder.
func (s *filmStore) films() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for k := range s.objects {
		if _, name, ok := strings.Cut(k, "/"+psync.RelayFilmsFolder); ok {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// With the relay on, Phone Sync puts the library's films in the storage, their videos too, and 存储用量 says what they take; 只同步字幕 is kept in settings.json, and so is the cleanup. 换一个存储 moves to another storage set up in 账号与上传, which becomes the one in use, and the films go up there.
func TestPhoneFilmsGoToTheStorageAndMove(t *testing.T) {
	a, tt, updates, _, _ := phoneSyncApp(t)
	first := newFilmStore(t)
	a.cloud.storage = cloud.KindCompat
	a.cloud.compatEndpoint, a.cloud.compatRegion, a.cloud.compatBucket = first.url, "us-east-1", "films"
	a.cloud.compatAccessKey, a.cloud.compatSecret, a.cloud.compatPath = "key", "secret-value", true
	a.phone.relay = true
	phoneSyncOn(t, a, tt, updates)
	waitUntil(t, tt, a, updates, func() bool {
		st := a.PhoneSync().Relay.Storage
		return st.Measured && st.Films == 2 && st.WithVideo == 2
	})
	videos := 0
	for _, name := range first.films() {
		if strings.HasSuffix(name, "/video.mp4") {
			videos++
		}
	}
	if videos != 2 || !tt.Exists("id:settings-phone-storage-total") {
		t.Fatalf("the storage holds %q; texts %q", first.films(), tt.Texts())
	}
	if st := a.PhoneSync().Relay.Storage; st.Videos != int64(2*len("video")) || len(st.Targets) != 0 {
		t.Fatalf("storage %+v", st)
	}

	tt.Press("settings-phone-relay-subtitles-only")
	a.SetPhoneRelayClean(7)
	a.SetPhoneRelayClean(5)
	data, err := os.ReadFile(a.settingsPath)
	if err != nil || !strings.Contains(string(data), `"phoneRelaySubtitlesOnly": true`) || !strings.Contains(string(data), `"phoneRelayCleanDays": 7`) {
		t.Fatalf("settings.json: %s, %v", data, err)
	}
	if st := a.PhoneSync().Relay; !st.SubtitlesOnly || st.Storage.CleanDays != 7 {
		t.Fatalf("relay %+v", st)
	}
	tt.Press("settings-phone-relay-subtitles-only")

	second := newFilmStore(t)
	a.cloud.r2Endpoint, a.cloud.r2Bucket, a.cloud.r2AccessKey, a.cloud.r2Secret = second.url, "films-2", "key-2", "secret-2"
	refresh(tt, a)
	if targets := a.PhoneSync().Relay.Storage.Targets; !slices.Equal(targets, []settings.PhoneStore{{Kind: cloud.KindR2, Name: "R2"}}) {
		t.Fatalf("targets %+v", targets)
	}
	running := a.phone.server
	a.MovePhoneStorage(cloud.KindR2)
	waitUntil(t, tt, a, updates, func() bool {
		return a.phone.server != nil && a.phone.server != running && a.PhoneSync().Relay.Storage.Moved
	})
	if a.cloud.storage != cloud.KindR2 || a.phone.relaySig != a.phoneRelaySig() {
		t.Fatalf("after the move the storage in use is %q", a.cloud.storage)
	}
	if got := len(second.films()); got != len(first.films()) {
		t.Fatalf("the new storage holds %q, the old %q", second.films(), first.films())
	}
	m := a.messages()
	if st := a.PhoneSync().Relay.Storage; st.MoveTo != "R2" || st.MoveFrom != m.SettingsPhoneRelayStoreName(m.AccountStoreCompat(), "films") || !tt.Exists("id:settings-phone-storage-move-status") {
		t.Fatalf("storage after the move %+v", st)
	}
	// The old storage keeps its files for the user to delete.
	if len(first.films()) == 0 {
		t.Fatal("the move deleted the old storage's files")
	}
}

// A film watched to its end on this computer is one the films through the storage may clean up, as of when it last played.
func TestPhoneSourceSaysWhichFilmsWereWatched(t *testing.T) {
	a, tt, updates, dir, _ := phoneSyncApp(t)
	talk := filepath.Join(dir, "Rust Talk", "talk.mp4")
	played := time.Date(2026, 11, 1, 20, 0, 0, 0, time.UTC)
	if a.lib.durations == nil {
		a.lib.durations = map[string]time.Duration{}
	}
	a.lib.durations[talk] = time.Minute
	a.palette.furthest = map[string]time.Duration{talk: time.Minute}
	a.player.played = map[string]time.Time{talk: played}
	list, err := phoneCall(t, a, tt, updates, func(ctx context.Context) ([]phonesync.Film, error) {
		libs, err := phoneSource{a}.Libraries(ctx)
		if err != nil || len(libs) == 0 {
			return nil, fmt.Errorf("libraries %v, %v", libs, err)
		}
		return phoneSource{a}.Films(ctx, libs[0])
	})
	if err != nil || len(list) == 0 {
		t.Fatalf("films %v, %v", list, err)
	}
	for _, f := range list {
		if want := f.Video == talk; f.Watched != want || (want && !f.WatchedAt.Equal(played)) {
			t.Errorf("%s watched %v at %v", f.Title, f.Watched, f.WatchedAt)
		}
	}
}
