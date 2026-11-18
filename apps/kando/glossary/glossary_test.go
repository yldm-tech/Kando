package glossary

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"kando/packages/core"
)

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "glossary.json")
	s, err := Load(path)
	if err != nil || len(s.Global) != 0 || len(s.Series) != 0 {
		t.Fatalf("no file: %+v %v", s, err)
	}
	s.Name("sub-1", "地理课")
	s.Set("sub-1", core.Term{Source: "meander", Target: "河曲", Scope: core.ScopeSeries, Locked: true})
	s.Set("", core.Term{Source: "delta", Target: "三角洲", Scope: core.ScopeGlobal})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	// Windows keeps no Unix permission bits: Go reports a file there as 0666, or 0444 when read-only.
	if info, err := os.Stat(path); err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o600) {
		t.Fatalf("file %v %v", info, err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	series, global := loaded.Terms("sub-1")
	if want := []core.Term{{Source: "meander", Target: "河曲", Scope: core.ScopeSeries, Locked: true}}; !reflect.DeepEqual(series, want) {
		t.Fatalf("series terms %+v, want %+v", series, want)
	}
	if want := []core.Term{{Source: "delta", Target: "三角洲", Scope: core.ScopeGlobal}}; !reflect.DeepEqual(global, want) {
		t.Fatalf("global terms %+v, want %+v", global, want)
	}
	if found, ok := loaded.Find("sub-1"); !ok || found.Name != "地理课" {
		t.Fatalf("series %+v %v", found, ok)
	}
	if series, global := loaded.Terms(""); series != nil || len(global) != 1 {
		t.Fatalf("a film of no series: %+v %+v", series, global)
	}
}

func TestLoadSaysWhyAFileCannotBeRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "glossary.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("a file that is not JSON read without an error")
	}
}

// A term moved from one scope to another is in that one alone, and a term moved back to the film is out of both.
func TestSetMovesATermBetweenScopes(t *testing.T) {
	var s Store
	s.Name("s", "Series")
	term := core.Term{Source: "meander", Target: "河曲", Scope: core.ScopeSeries}
	if !s.Set("s", term) {
		t.Fatal("adding a series term changed nothing")
	}
	if s.Set("s", term) {
		t.Fatal("setting the same term again changed something")
	}
	term.Scope = core.ScopeGlobal
	s.Set("s", term)
	if series, global := s.Terms("s"); len(series) != 0 || len(global) != 1 {
		t.Fatalf("moved to every film: series %+v, global %+v", series, global)
	}
	term.Scope = core.ScopeSeries
	s.Set("s", term)
	if series, global := s.Terms("s"); len(series) != 1 || len(global) != 0 {
		t.Fatalf("moved back to the series: series %+v, global %+v", series, global)
	}
	term.Scope = core.ScopeFilm
	s.Set("s", term)
	if series, global := s.Terms("s"); len(series) != 0 || len(global) != 0 {
		t.Fatalf("moved to the film: series %+v, global %+v", series, global)
	}
	term.Scope = core.ScopeSeries
	if s.Set("", term) {
		t.Fatal("a series term without a series was kept")
	}
	if s.Name("s", "Series") || !s.Name("s", "Other") || s.Name("s", " ") {
		t.Fatal("Name reports changes wrongly")
	}
}

// A series term for a series the store does not have, as when glossary.json lost it while a film's job still names it, adds the series rather than dropping the term.
func TestSetAddsTheSeriesATermNames(t *testing.T) {
	var s Store
	term := core.Term{Source: "meander", Target: "河曲", Scope: core.ScopeSeries}
	if !s.Set("lost", term) {
		t.Fatal("a series term for a series the store lacks changed nothing")
	}
	if series, ok := s.Find("lost"); !ok || series.Name != "" || len(series.Terms) != 1 {
		t.Fatalf("series %+v %v", series, ok)
	}
	if s.Set("other", core.Term{Source: "delta", Target: "三角洲", Scope: core.ScopeGlobal}); len(s.Series) != 1 {
		t.Fatalf("a global term added a series: %+v", s.Series)
	}
}

// Clone shares nothing with the store it copies: a change to a series' terms in the copy leaves the store's as they were.
func TestCloneSharesNoTerms(t *testing.T) {
	var s Store
	s.Name("s", "Series")
	s.Set("s", core.Term{Source: "meander", Target: "河曲", Scope: core.ScopeSeries})
	c := s.Clone()
	c.Set("s", core.Term{Source: "meander", Target: "河曲", Scope: core.ScopeGlobal})
	c.Set("s", core.Term{Source: "delta", Target: "三角洲", Scope: core.ScopeSeries})
	if series, global := s.Terms("s"); len(series) != 1 || series[0].Source != "meander" || len(global) != 0 {
		t.Fatalf("the store changed with its copy: series %+v, global %+v", series, global)
	}
}
