// Package glossary keeps the terms of a series and the global ones (core.Term), which every film of the series, or every film, is translated with: Store keeps them in glossary.json beside settings.json. A film's own terms are kept beside its video (media.TermsPath). The app gives a job the glossary of its film (core.MergeTerms) as it starts (state_glossary.go).
package glossary

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"kando/packages/core"
)

// Series is a series of films the user said belong together, as the videos of a subscription that is one (subscribe.Rules.Series): ID is what a film's job names it by (core.Job.Series), Name what the app calls it, and Terms its glossary.
type Series struct {
	ID    string      `json:"id"`
	Name  string      `json:"name"`
	Terms []core.Term `json:"terms,omitempty"`
}

// Store is the global terms and the series, in the order they were made, and the file they are kept in. NotSeries are the shows the user said are not a series when the app guessed they were from their films' names (core.EpisodeOf), each a key of NotSeriesKey, so the app does not ask again.
type Store struct {
	path      string
	Global    []core.Term
	Series    []Series
	NotSeries []string
}

// stored is glossary.json.
type stored struct {
	Global    []core.Term `json:"global"`
	Series    []Series    `json:"series"`
	NotSeries []string    `json:"notSeries,omitempty"`
}

// NotSeriesKey is the key Store.NotSeries keeps a show by: the library its films are in and the show's key (core.EpisodeOf).
func NotSeriesKey(library, show string) string { return filepath.Clean(library) + "\x00" + show }

// IsNotSeries reports whether the user said the show of key, of NotSeriesKey, is not a series.
func (s Store) IsNotSeries(key string) bool { return slices.Contains(s.NotSeries, key) }

// SetNotSeries keeps that the show of key, of NotSeriesKey, is not a series, and reports whether that changed anything.
func (s *Store) SetNotSeries(key string) bool {
	if s.IsNotSeries(key) {
		return false
	}
	s.NotSeries = append(s.NotSeries, key)
	return true
}

// Load reads the glossary at path. A missing file holds none; a file that cannot be read or is not JSON holds none either, and the error says why.
func Load(path string) (Store, error) {
	s := Store{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	var file stored
	if err := json.Unmarshal(data, &file); err != nil {
		return s, err
	}
	s.Global = clean(file.Global, core.ScopeGlobal)
	s.NotSeries = slices.DeleteFunc(file.NotSeries, func(key string) bool { return strings.TrimSpace(key) == "" })
	for _, series := range file.Series {
		if strings.TrimSpace(series.ID) == "" {
			continue
		}
		series.Terms = clean(series.Terms, core.ScopeSeries)
		s.Series = append(s.Series, series)
	}
	return s, nil
}

// clean is terms without those that have no source or target, each in scope.
func clean(terms []core.Term, scope core.TermScope) []core.Term {
	var out []core.Term
	for _, t := range terms {
		if t.Valid() {
			t.Scope = scope
			out = append(out, t)
		}
	}
	return out
}

// Path is the file the store is kept in, "" for one that is not kept.
func (s Store) Path() string { return s.path }

// Save writes the glossary to the store's file, readable by the user alone, replacing it whole so a crash leaves the old one; a store without a file writes nothing.
func (s Store) Save() error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	file := stored{Global: clean(s.Global, core.ScopeGlobal), Series: []Series{}, NotSeries: s.NotSeries}
	if file.Global == nil {
		file.Global = []core.Term{}
	}
	for _, series := range s.Series {
		series.Terms = clean(series.Terms, core.ScopeSeries)
		file.Series = append(file.Series, series)
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Find is the series with id.
func (s *Store) Find(id string) (*Series, bool) {
	if id == "" {
		return nil, false
	}
	for i := range s.Series {
		if s.Series[i].ID == id {
			return &s.Series[i], true
		}
	}
	return nil, false
}

// Name names the series with id, adding it when there is none, and reports whether anything changed.
func (s *Store) Name(id, name string) bool {
	name = strings.TrimSpace(name)
	if series, ok := s.Find(id); ok {
		if series.Name == name || name == "" {
			return false
		}
		series.Name = name
		return true
	}
	s.Series = append(s.Series, Series{ID: id, Name: name})
	return true
}

// Terms is the glossary a film of the series with id is translated with besides its own terms: the series', none for a film of no series, and the global ones.
func (s Store) Terms(id string) (series, global []core.Term) {
	if found, ok := s.Find(id); ok {
		series = found.Terms
	}
	return series, s.Global
}

// Set puts t in the glossary of its scope, the series with id for a series term, in place of an entry for the same source there, and takes the same source out of the other one this store keeps, so a term moved from the series to every film, or back, is in one of them alone. A series term for an id the store has no series of adds one, unnamed, as for a film whose job names a series glossary.json lost. A term whose scope is the film's, or the series' with no id, is taken out of both. It reports whether anything changed.
func (s *Store) Set(id string, t core.Term) bool {
	before := s.Clone()
	if _, ok := s.Find(id); !ok && id != "" && t.Scope == core.ScopeSeries && t.Valid() {
		s.Series = append(s.Series, Series{ID: id})
	}
	series, hasSeries := s.Find(id)
	switch {
	case t.Scope == core.ScopeGlobal:
		s.Global = core.SetTerm(s.Global, t)
		if hasSeries {
			series.Terms = core.RemoveTerm(series.Terms, t.Source)
		}
	case t.Scope == core.ScopeSeries && hasSeries:
		series.Terms = core.SetTerm(series.Terms, t)
		s.Global = core.RemoveTerm(s.Global, t.Source)
	default:
		s.Remove(id, t.Source)
	}
	return !before.equal(*s)
}

// Remove takes source out of the global terms and out of the series with id, and reports whether it was in either.
func (s *Store) Remove(id, source string) bool {
	before := s.Clone()
	s.Global = core.RemoveTerm(s.Global, source)
	if series, ok := s.Find(id); ok {
		series.Terms = core.RemoveTerm(series.Terms, source)
	}
	return !before.equal(*s)
}

// Clone is a copy of s that shares nothing with it, so a change to one of them leaves the other as it was.
func (s Store) Clone() Store {
	out := Store{path: s.path, Global: slices.Clone(s.Global), NotSeries: slices.Clone(s.NotSeries)}
	for _, series := range s.Series {
		series.Terms = slices.Clone(series.Terms)
		out.Series = append(out.Series, series)
	}
	return out
}

func (s Store) equal(o Store) bool {
	return slices.Equal(s.Global, o.Global) && slices.Equal(s.NotSeries, o.NotSeries) && slices.EqualFunc(s.Series, o.Series, func(a, b Series) bool {
		return a.ID == b.ID && a.Name == b.Name && slices.Equal(a.Terms, b.Terms)
	})
}
