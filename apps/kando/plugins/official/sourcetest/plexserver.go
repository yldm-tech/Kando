package sourcetest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// PlexSection is a library of a PlexServer, by its key, title and type (movie, show, artist, photo).
type PlexSection struct {
	Key, Title, Type string
}

// PlexItem is an item of a PlexServer: a film, a show, a season or an episode, by its rating key, under a section (Section, for a film or a show) or another item (Parent), with the fields Plex answers of it; Parts are its first media's files, by their path on the server and their bytes; Thumb, Art and GrandparentArt are the artwork bytes Plex serves at /library/metadata/<ratingKey>/(thumb|art)/<ts> (or its show's art for an episode).
type PlexItem struct {
	RatingKey, Section, Parent, Type, Title string
	Year, Index, ParentIndex                int
	Container                               string
	Parts                                   []PlexPart
	Thumb, Art                              []byte
	GrandparentArt                          string
}

// PlexPart is a file of a PlexItem: its path on the server, its container when Plex says one, and its bytes.
type PlexPart struct {
	File, Container, Data string
}

// PlexServer is a fake Plex Media Server, the few endpoints the official plugin uses, in memory: /library/sections, /library/sections/<key>/all?type=1|2, /library/metadata/<key>/children, in pages by X-Plex-Container-Start and X-Plex-Container-Size, and /library/parts/<id>/<changestamp>/file.<ext>, which serves the part's file with Range. It answers XML unless asked for JSON, as Plex does, and 401 to every request Authorized does not take. It keeps what it heard, by method, path and query.
type PlexServer struct {
	*httptest.Server
	// Authorized reports whether a request carries the token, as the server reads it.
	Authorized func(r *http.Request) bool

	mu       sync.Mutex
	sections []PlexSection
	items    []PlexItem
	heard    []string
}

// plexAdded is when every item of a PlexServer was added, in seconds since 1970.
const plexAdded = 1762165230

// NewPlexServer starts a PlexServer of sections and items, which takes the requests authorized reports.
func NewPlexServer(t testing.TB, authorized func(r *http.Request) bool, sections []PlexSection, items []PlexItem) *PlexServer {
	s := &PlexServer{Authorized: authorized, sections: sections, items: items}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

// Heard are the requests the server heard since the last Heard, by method, path and query.
func (s *PlexServer) Heard() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.heard
	s.heard = nil
	return out
}

// partID is the id of an item's ith part: the item's rating key and i, as digits.
func partID(it PlexItem, i int) string { return it.RatingKey + strconv.Itoa(i) }

func (s *PlexServer) metadata(it PlexItem) map[string]any {
	d := map[string]any{"ratingKey": it.RatingKey, "key": "/library/metadata/" + it.RatingKey, "type": it.Type, "title": it.Title, "addedAt": plexAdded, "updatedAt": plexAdded + 100}
	if it.Year != 0 {
		d["year"] = it.Year
	}
	if it.Type == "episode" || it.Type == "season" {
		d["index"] = it.Index
	}
	if it.Type == "episode" {
		d["parentIndex"] = it.ParentIndex
	}
	if len(it.Thumb) > 0 {
		d["thumb"] = fmt.Sprintf("/library/metadata/%s/thumb/%d", it.RatingKey, plexAdded)
	}
	if len(it.Art) > 0 {
		d["art"] = fmt.Sprintf("/library/metadata/%s/art/%d", it.RatingKey, plexAdded)
	}
	if it.GrandparentArt != "" {
		d["grandparentArt"] = fmt.Sprintf("/library/metadata/%s/art/%d", it.GrandparentArt, plexAdded)
	}
	if len(it.Parts) > 0 {
		var parts []map[string]any
		for i, p := range it.Parts {
			ext := strings.TrimPrefix(path.Ext(p.File), ".")
			if ext == "" {
				ext = p.Container
			}
			part := map[string]any{"id": partID(it, i), "key": fmt.Sprintf("/library/parts/%s/%d/file.%s", partID(it, i), plexAdded, ext), "file": p.File, "size": len(p.Data)}
			if p.Container != "" {
				part["container"] = p.Container
			}
			parts = append(parts, part)
		}
		d["Media"] = []map[string]any{{"id": it.RatingKey, "container": it.Container, "Part": parts}}
	}
	return d
}

func (s *PlexServer) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.heard = append(s.heard, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
	if !s.Authorized(r) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "<html><head><title>Unauthorized</title></head><body><h1>401 Unauthorized</h1></body></html>")
		return
	}
	q := r.URL.Query()
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) == 5 && parts[0] == "library" && parts[1] == "parts" {
		for _, it := range s.items {
			for i, p := range it.Parts {
				if partID(it, i) == parts[2] {
					http.ServeContent(w, r, p.File, time.Time{}, strings.NewReader(p.Data))
					return
				}
			}
		}
		http.NotFound(w, r)
		return
	}
	if len(parts) == 5 && parts[0] == "library" && parts[1] == "metadata" && (parts[3] == "thumb" || parts[3] == "art") {
		for _, it := range s.items {
			if it.RatingKey != parts[2] {
				continue
			}
			var pic []byte
			if parts[3] == "thumb" {
				pic = it.Thumb
			} else {
				pic = it.Art
			}
			if len(pic) == 0 {
				break
			}
			w.Header().Set("Content-Type", http.DetectContentType(pic))
			_, _ = w.Write(pic)
			return
		}
		http.NotFound(w, r)
		return
	}
	container := map[string]any{}
	var list []PlexItem
	switch {
	case r.URL.Path == "/library/sections":
		var dirs []map[string]any
		for _, sec := range s.sections {
			dirs = append(dirs, map[string]any{"key": sec.Key, "title": sec.Title, "type": sec.Type})
		}
		container["size"] = len(dirs)
		container["Directory"] = dirs
	case len(parts) == 4 && parts[0] == "library" && parts[1] == "sections" && parts[3] == "all":
		if !slices.ContainsFunc(s.sections, func(sec PlexSection) bool { return sec.Key == parts[2] }) {
			http.NotFound(w, r)
			return
		}
		kind := map[string]string{"1": "movie", "2": "show"}[q.Get("type")]
		for _, it := range s.items {
			if it.Section == parts[2] && (kind == "" || it.Type == kind) {
				list = append(list, it)
			}
		}
	case len(parts) == 4 && parts[0] == "library" && parts[1] == "metadata" && parts[3] == "children":
		if !slices.ContainsFunc(s.items, func(it PlexItem) bool { return it.RatingKey == parts[2] }) {
			http.NotFound(w, r)
			return
		}
		for _, it := range s.items {
			if it.Parent == parts[2] {
				list = append(list, it)
			}
		}
	default:
		http.NotFound(w, r)
		return
	}
	if r.URL.Path != "/library/sections" {
		total := len(list)
		start, _ := strconv.Atoi(q.Get("X-Plex-Container-Start"))
		start = min(max(start, 0), total)
		end := total
		if size, err := strconv.Atoi(q.Get("X-Plex-Container-Size")); err == nil && size >= 0 {
			end = min(start+size, total)
		}
		out := []map[string]any{}
		for _, it := range list[start:end] {
			out = append(out, s.metadata(it))
		}
		container["size"] = len(out)
		container["totalSize"] = total
		container["offset"] = start
		container["Metadata"] = out
	}
	if !strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "text/xml;charset=utf-8")
		fmt.Fprintf(w, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<MediaContainer size=\"%v\"></MediaContainer>\n", container["size"])
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"MediaContainer": container})
}

// PlexLibrary is a sample library of a Plex server: a film library (a film, two of the same title and year, one split over two files, and one whose file has no extension), a TV library (a show of two seasons and a show that hides its one season), an other-video library (Plex's movie type, with no year), and a music and a photo library, which are not shown.
func PlexLibrary() ([]PlexSection, []PlexItem) {
	sections := []PlexSection{
		{Key: "1", Title: "Films", Type: "movie"},
		{Key: "2", Title: "Shows", Type: "show"},
		{Key: "3", Title: "Music", Type: "artist"},
		{Key: "4", Title: "Photos", Type: "photo"},
		{Key: "5", Title: "Home videos", Type: "movie"},
	}
	items := []PlexItem{
		{RatingKey: "101", Section: "1", Type: "movie", Title: "Talk", Year: 2024, Container: "mkv", Parts: []PlexPart{{File: "/media/films/Talk (2024)/Talk.mkv", Data: "0123456789abcdefghij"}}},
		{RatingKey: "103", Section: "1", Type: "movie", Title: "Twin", Year: 2020, Container: "mp4", Parts: []PlexPart{{File: "/media/films/Twin b.mp4", Data: "twin b"}}},
		{RatingKey: "102", Section: "1", Type: "movie", Title: "Twin", Year: 2020, Container: "mp4", Parts: []PlexPart{{File: "/media/films/Twin a.mp4", Data: "twin a"}}},
		{RatingKey: "104", Section: "1", Type: "movie", Title: "Long Film", Year: 2019, Container: "avi", Parts: []PlexPart{{File: "/media/films/Long/cd1.avi", Data: "first"}, {File: "/media/films/Long/cd2.avi", Data: "second"}}},
		{RatingKey: "105", Section: "1", Type: "movie", Title: "Raw", Year: 2001, Container: "mp4", Parts: []PlexPart{{File: "/media/films/raw", Container: "mp4", Data: "raw"}}},
		{RatingKey: "201", Section: "2", Type: "show", Title: "Night/Day"},
		{RatingKey: "211", Parent: "201", Type: "season", Title: "Season 1", Index: 1},
		{RatingKey: "212", Parent: "201", Type: "season", Title: "Season 2", Index: 2},
		{RatingKey: "221", Parent: "211", Type: "episode", Title: "Pilot", Index: 1, ParentIndex: 1, Container: "mp4", Parts: []PlexPart{{File: "/media/tv/s01e01.mp4", Data: "pilot"}}},
		{RatingKey: "222", Parent: "211", Type: "episode", Title: "Second", Index: 2, ParentIndex: 1, Container: "mkv", Parts: []PlexPart{{File: "/media/tv/s01e02.mkv", Data: "second"}}},
		{RatingKey: "223", Parent: "212", Type: "episode", Title: "Back", Index: 1, ParentIndex: 2, Container: "avi", Parts: []PlexPart{{File: "/media/tv/s02e01.avi", Data: "back"}}},
		{RatingKey: "202", Section: "2", Type: "show", Title: "Solo"},
		{RatingKey: "231", Parent: "202", Type: "episode", Title: "Only", Index: 1, ParentIndex: 1, Container: "mkv", Parts: []PlexPart{{File: "/media/tv/solo/s01e01.mkv", Data: "only"}}},
		{RatingKey: "501", Section: "5", Type: "movie", Title: "beach", Container: "mov", Parts: []PlexPart{{File: "/media/home/trip/beach.mov", Data: "waves"}}},
		{RatingKey: "301", Section: "3", Type: "artist", Title: "Band"},
	}
	return sections, items
}

// PlexMany is a film library of n films, Film 0000 to Film n-1.
func PlexMany(n int) ([]PlexSection, []PlexItem) {
	var items []PlexItem
	for i := range n {
		items = append(items, PlexItem{RatingKey: strconv.Itoa(10000 + i), Section: "9", Type: "movie", Title: fmt.Sprintf("Film %04d", i), Container: "mkv", Parts: []PlexPart{{File: fmt.Sprintf("/media/many/%04d.mkv", i), Data: "x"}}})
	}
	return []PlexSection{{Key: "9", Title: "Many", Type: "movie"}}, items
}
