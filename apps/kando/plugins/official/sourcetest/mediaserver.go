package sourcetest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Item is an item of a MediaServer: a library (CollectionType set), a series, a season, a folder or a video, under its parent, with the fields the Jellyfin and Emby APIs answer of it; Data is a video's file.
type Item struct {
	ID, Parent, Name, Type string
	CollectionType         string
	IsFolder               bool
	MediaType              string
	Path, Container        string
	Year, Season, Episode  int
	EpisodeEnd             int
	SeriesID               string
	Data                   string
}

// MediaServer is a fake Jellyfin or Emby, the few endpoints the official plugins use, in memory: /Library/MediaFolders, /Items (by ParentId, recursively by type, or by Ids, in pages), /Shows/<id>/Seasons and /Shows/<id>/Episodes?SeasonId=, and /Videos/<id>/stream?static=true, which serves the original file with Range; every request but one that Authorized takes answers 401. It keeps what it heard, by method, path and query.
type MediaServer struct {
	*httptest.Server
	// Authorized reports whether a request carries the API key, as the server reads it.
	Authorized func(r *http.Request) bool

	mu    sync.Mutex
	items []Item
	heard []string
}

// NewMediaServer starts a MediaServer of items, which takes the requests authorized reports.
func NewMediaServer(t testing.TB, authorized func(r *http.Request) bool, items ...Item) *MediaServer {
	s := &MediaServer{Authorized: authorized, items: items}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

// Heard are the requests the server heard since the last Heard, by method, path and query.
func (s *MediaServer) Heard() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.heard
	s.heard = nil
	return out
}

// Add adds items.
func (s *MediaServer) Add(items ...Item) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = append(s.items, items...)
}

func (s *MediaServer) item(id string) (Item, bool) {
	i := slices.IndexFunc(s.items, func(it Item) bool { return it.ID == id })
	if i < 0 {
		return Item{}, false
	}
	return s.items[i], true
}

// dto is the item as the servers answer it, with its MediaSources when sources.
func (s *MediaServer) dto(it Item, sources bool) map[string]any {
	d := map[string]any{"Id": it.ID, "Name": it.Name, "Type": it.Type, "IsFolder": it.IsFolder, "DateCreated": "2026-11-03T10:20:30.0000000Z", "Etag": "etag-" + it.ID}
	if it.CollectionType != "" {
		d["CollectionType"] = it.CollectionType
	}
	if it.MediaType != "" {
		d["MediaType"] = it.MediaType
	}
	if it.Path != "" {
		d["Path"] = it.Path
	}
	if it.Container != "" {
		d["Container"] = it.Container
	}
	if it.Year != 0 {
		d["ProductionYear"] = it.Year
	}
	if it.Type == "Episode" {
		d["IndexNumber"] = it.Episode
		d["ParentIndexNumber"] = it.Season
		d["SeriesId"] = it.SeriesID
		if it.EpisodeEnd != 0 {
			d["IndexNumberEnd"] = it.EpisodeEnd
		}
	}
	if it.Type == "Season" {
		d["IndexNumber"] = it.Season
		d["SeriesId"] = it.SeriesID
	}
	if sources {
		d["MediaSources"] = []map[string]any{{"Id": it.ID, "Size": len(it.Data), "Container": it.Container, "MediaStreams": []map[string]any{{"Type": "Video", "Codec": "h264"}}}}
	}
	return d
}

// under reports whether it is below the item parent, at any depth.
func (s *MediaServer) under(it Item, parent string) bool {
	for p := it.Parent; p != ""; {
		if p == parent {
			return true
		}
		up, ok := s.item(p)
		if !ok {
			return false
		}
		p = up.Parent
	}
	return false
}

func (s *MediaServer) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.heard = append(s.heard, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
	if !s.Authorized(r) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	q := r.URL.Query()
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	var list []Item
	sources := strings.Contains(q.Get("Fields"), "MediaSources")
	switch {
	case r.URL.Path == "/Library/MediaFolders":
		for _, it := range s.items {
			if it.CollectionType != "" || it.Type == "CollectionFolder" {
				list = append(list, it)
			}
		}
	case r.URL.Path == "/Items" && q.Get("Ids") != "":
		for _, id := range strings.Split(q.Get("Ids"), ",") {
			if it, ok := s.item(id); ok {
				list = append(list, it)
			}
		}
	case r.URL.Path == "/Items":
		parent := q.Get("ParentId")
		if _, ok := s.item(parent); !ok {
			http.NotFound(w, r)
			return
		}
		types := q.Get("IncludeItemTypes")
		for _, it := range s.items {
			switch {
			case q.Get("Recursive") == "true" && s.under(it, parent) && (types == "" || slices.Contains(strings.Split(types, ","), it.Type)):
			case q.Get("Recursive") != "true" && it.Parent == parent:
			default:
				continue
			}
			list = append(list, it)
		}
	case len(parts) == 3 && parts[0] == "Shows" && (parts[2] == "Seasons" || parts[2] == "Episodes"):
		series, ok := s.item(parts[1])
		if !ok || series.Type != "Series" {
			http.NotFound(w, r)
			return
		}
		for _, it := range s.items {
			if parts[2] == "Seasons" && it.Type == "Season" && it.SeriesID == series.ID || parts[2] == "Episodes" && it.Type == "Episode" && it.SeriesID == series.ID && (q.Get("SeasonId") == "" || it.Parent == q.Get("SeasonId")) {
				list = append(list, it)
			}
		}
	case len(parts) == 3 && parts[0] == "Videos" && parts[2] == "stream":
		it, ok := s.item(parts[1])
		if !ok || it.Data == "" {
			http.NotFound(w, r)
			return
		}
		if q.Get("static") != "true" {
			http.Error(w, "transcoding", http.StatusBadRequest)
			return
		}
		http.ServeContent(w, r, it.Name, time.Time{}, strings.NewReader(it.Data))
		return
	default:
		http.NotFound(w, r)
		return
	}
	total := len(list)
	start, _ := strconv.Atoi(q.Get("StartIndex"))
	start = min(max(start, 0), total)
	end := total
	if limit, err := strconv.Atoi(q.Get("Limit")); err == nil && limit > 0 {
		end = min(start+limit, total)
	}
	out := []map[string]any{}
	for _, it := range list[start:end] {
		out = append(out, s.dto(it, sources))
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]any{"Items": out, "TotalRecordCount": total, "StartIndex": start})
}

// Library is a sample library tree of a media server: a film library (two films, one in a folder of its own, and two of the same name and year), a TV library (a series of two seasons, an episode of which is a double one), a home-video library (a folder holding a video, and a photo, which is no video), and a music library, which is not shown.
func Library() []Item {
	items := []Item{
		{ID: "lib-movies", Name: "Films", Type: "CollectionFolder", CollectionType: "movies", IsFolder: true},
		{ID: "m1", Parent: "lib-movies", Name: "Talk", Type: "Movie", MediaType: "Video", Year: 2024, Path: "/media/films/Talk (2024)/Talk.mkv", Container: "mkv", Data: "0123456789abcdefghij"},
		{ID: "m3", Parent: "lib-movies", Name: "Twin", Type: "Movie", MediaType: "Video", Year: 2020, Path: "/media/films/Twin.mp4", Container: "mov,mp4,m4a,3gp,3g2,mj2", Data: "twin b"},
		{ID: "m2", Parent: "lib-movies", Name: "Twin", Type: "Movie", MediaType: "Video", Year: 2020, Container: "mov,mp4,m4a,3gp,3g2,mj2", Data: "twin a"},
		{ID: "lib-tv", Name: "Shows", Type: "CollectionFolder", CollectionType: "tvshows", IsFolder: true},
		{ID: "s1", Parent: "lib-tv", Name: "Night/Day", Type: "Series", IsFolder: true},
		{ID: "s1-1", Parent: "s1", Name: "Season 1", Type: "Season", IsFolder: true, Season: 1, SeriesID: "s1"},
		{ID: "s1-2", Parent: "s1", Name: "Season 2", Type: "Season", IsFolder: true, Season: 2, SeriesID: "s1"},
		{ID: "e1", Parent: "s1-1", Name: "Pilot", Type: "Episode", MediaType: "Video", Season: 1, Episode: 1, SeriesID: "s1", Path: "/media/tv/s01e01.mp4", Data: "pilot"},
		{ID: "e2", Parent: "s1-1", Name: "Two Parter", Type: "Episode", MediaType: "Video", Season: 1, Episode: 2, EpisodeEnd: 3, SeriesID: "s1", Path: "/media/tv/s01e02-e03.mkv", Data: "double"},
		{ID: "e4", Parent: "s1-2", Name: "Back", Type: "Episode", MediaType: "Video", Season: 2, Episode: 1, SeriesID: "s1", Path: "/media/tv/s02e01.avi", Data: "back"},
		{ID: "lib-home", Name: "Home videos", Type: "CollectionFolder", CollectionType: "homevideos", IsFolder: true},
		{ID: "f1", Parent: "lib-home", Name: "Trip", Type: "Folder", IsFolder: true},
		{ID: "v1", Parent: "f1", Name: "beach", Type: "Video", MediaType: "Video", Path: "/media/home/trip/beach.mov", Data: "waves"},
		{ID: "p1", Parent: "f1", Name: "sunset", Type: "Photo", MediaType: "Photo", Path: "/media/home/trip/sunset.jpg"},
		{ID: "lib-music", Name: "Music", Type: "CollectionFolder", CollectionType: "music", IsFolder: true},
		{ID: "a1", Parent: "lib-music", Name: "Song", Type: "Audio", MediaType: "Audio"},
	}
	return items
}

// Many is a film library of n films, Film 0000 to Film n-1.
func Many(n int) []Item {
	items := []Item{{ID: "lib-many", Name: "Many", Type: "CollectionFolder", CollectionType: "movies", IsFolder: true}}
	for i := range n {
		items = append(items, Item{ID: fmt.Sprintf("many-%04d", i), Parent: "lib-many", Name: fmt.Sprintf("Film %04d", i), Type: "Movie", MediaType: "Video", Container: "mkv", Data: "x"})
	}
	return items
}
