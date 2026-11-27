// Package catalog is the Library tab's films from every paired computer as one library: what a computer lists (Load) with where each film was left on any device, read from the learning state of protocol v2 (yldm-tech/Kando#217); the computers' films and the phone's own merged into one list (Merge); and that list narrowed by library, by source and by a search over titles and the lines kept on the phone (Filter, Search), with the films to go on with (Continue), and the films of a work folded into one row (Fold, works.go). Nothing here touches the interface.
package catalog

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	gosync "sync"
	"time"

	"kando/apps/mobile/store"
	"kando/packages/sync"
)

// Position is where a film was left: how far in, when, and on which device, by the name its computer gives it; Mine for this phone.
type Position struct {
	At     time.Duration
	When   time.Time
	Device string
	Mine   bool
}

// Library is a library of a computer: the films it lists, and where they were left, by film ID.
type Library struct {
	sync.Library
	Films     []sync.Film
	Positions map[string]Position
}

// Listing is what a computer lists: its libraries, in its order.
type Listing struct {
	Libraries []Library
}

// Load reads what the computer c serves: its libraries (including locked password-protected ones on desktops that support sync.WithLocked), the films of each unlocked library, those being made among them (sync.Film.Making), and, where it serves protocol v2's learning state, where they were left, device being the ID it paired this phone under. A library the computer no longer serves, as one locked meanwhile, is left out; any other failure fails the listing. A computer that serves no learning state lists no positions.
func Load(ctx context.Context, c *sync.Client, device string) (Listing, error) {
	libs, err := c.LibrariesWithLocked(ctx)
	if err != nil {
		return Listing{}, err
	}
	out := make([]Library, len(libs))
	errs := make([]error, len(libs))
	var wg gosync.WaitGroup
	for i, lib := range libs {
		if lib.Locked {
			out[i] = Library{Library: lib}
			continue
		}
		wg.Go(func() {
			films, err := c.Films(ctx, lib.ID)
			if err != nil {
				errs[i] = err
				return
			}
			// The films being made follow, which a computer from before them lists none of; one that cannot list them now lists the films done alone.
			if making, err := c.Making(ctx, lib.ID); err == nil {
				films = append(films, making...)
			}
			out[i] = Library{Library: lib, Films: films}
			// The learning state is v2's: a desktop that speaks v1 alone, or keeps no state, answers with an error, which leaves the library without positions.
			if changes, err := c.State(ctx, lib.ID, "", 0); err == nil {
				out[i].Positions = Positions(changes, device)
			}
		})
	}
	wg.Wait()
	var kept []Library
	for i, lib := range out {
		var e *sync.Error
		switch {
		case errs[i] == nil:
			kept = append(kept, lib)
		case errors.As(errs[i], &e) && e.Status == http.StatusNotFound:
		default:
			return Listing{}, errs[i]
		}
	}
	return Listing{Libraries: kept}, nil
}

// FromRelay is what a computer lists through the user's storage away from its network (sync.RelayShelf): its libraries, in its order, with the films each holds there; a film of a library the shelf does not name is left out. It lists no positions, as the learning state does not travel through the storage yet.
func FromRelay(shelf sync.RelayShelf) Listing {
	var out []Library
	at := map[string]int{}
	for _, lib := range shelf.Libraries {
		at[lib.ID] = len(out)
		out = append(out, Library{Library: lib, Films: []sync.Film{}})
	}
	for _, f := range shelf.Films {
		if i, ok := at[f.Library]; ok {
			out[i].Films = append(out[i].Films, f.Film)
		}
	}
	return Listing{Libraries: out}
}

// Positions are the positions of a library's learning state, by film ID, named by the devices the state names; device is this phone's ID. A film whose position is null, to start from the beginning, has none.
func Positions(changes sync.StateChanges, device string) map[string]Position {
	out := map[string]Position{}
	for _, r := range changes.Records {
		if r.Kind != sync.KindPosition || r.Removed != nil {
			continue
		}
		f, ok := r.Fields[sync.FieldPositionMS]
		if !ok || sync.IsNull(f.Value) {
			continue
		}
		var ms float64
		if json.Unmarshal(f.Value, &ms) != nil || ms <= 0 {
			continue
		}
		film := cmp.Or(r.Film, r.ID)
		out[film] = Position{At: time.Duration(ms) * time.Millisecond, When: f.At, Device: changes.Devices[f.Device], Mine: f.Device != "" && f.Device == device}
	}
	return out
}

// Computer is a paired computer as the merge takes it: its ID on the phone, its name, and what it last listed.
type Computer struct {
	ID, Name string
	Listing  Listing
}

// Entry is a film of the merged library: the computer it is from, by its ID on the phone and its name, its library, which a film only the phone still keeps knows by name alone, the film, when it last changed on its computer (sync.Film.Updated), which the list is ordered by, and where it was last left on any device.
type Entry struct {
	Desktop  string
	Computer string
	Library  sync.Library
	Film     sync.Film
	Added    time.Time
	Position Position
}

// Key names the entry's film on the phone (store.Key).
func (e Entry) Key() string { return store.Key(e.Desktop, e.Film.ID) }

// Merge is the films the computers list and those the phone keeps, each once, the latest first by when each last changed on its computer, so a film keeps its place whether or not its computer answers: a film both have is the computer's, as it lists it now, with where it was left last of the computer's and the phone's positions, the phone's being newer even where it went back to the beginning. A film the phone keeps from a computer that does not list it, as one out of reach or unpaired, is the phone's. A film whose computer gave no time is ordered by when the phone downloaded it.
//
// A film of a NAS or a server matched to a computer's film of the same file (store.Film.Lines, docs/media-sources.md 10.5) is that computer's film: it is listed once, under the computer's film's key, with the computer's title, poster and library, and where it was left last of both; where the computer does not list it, it is listed as the phone last took it from the computer (store.Lines). Which of the phone's copies plays it is the session's to say (session.PlayKey, session.Copy).
func Merge(computers []Computer, kept []store.Film) []Entry {
	var out []Entry
	at := map[string]int{}
	for _, c := range computers {
		for _, lib := range c.Listing.Libraries {
			for _, f := range lib.Films {
				e := Entry{Desktop: c.ID, Computer: c.Name, Library: lib.Library, Film: f, Added: f.Updated, Position: lib.Positions[f.ID]}
				if _, dup := at[e.Key()]; dup {
					continue
				}
				at[e.Key()] = len(out)
				out = append(out, e)
			}
		}
	}
	for _, k := range kept {
		mine := Position{At: time.Duration(k.PositionMS) * time.Millisecond, When: k.PositionAt, Mine: true}
		key := k.Key()
		if k.FromSource() && k.Lines != nil {
			key = k.Lines.Key()
		}
		if i, ok := at[key]; ok {
			if (k.PositionMS > 0 || !k.PositionAt.IsZero()) && (out[i].Position.At <= 0 || !out[i].Position.When.After(k.PositionAt)) {
				out[i].Position = mine
			}
			if out[i].Added.IsZero() {
				out[i].Added = k.Added
			}
			continue
		}
		e := Entry{Desktop: k.Desktop, Library: sync.Library{Name: k.Library}, Film: k.Film, Added: k.Film.Updated}
		if l := k.Lines; key != k.Key() {
			e = Entry{Desktop: l.Desktop, Library: sync.Library{ID: l.Library, Name: l.LibraryName}, Film: l.Film, Added: l.Film.Updated}
		}
		if e.Added.IsZero() {
			e.Added = k.Added
		}
		if k.PositionMS > 0 {
			e.Position = mine
		}
		at[e.Key()] = len(out)
		out = append(out, e)
	}
	slices.SortStableFunc(out, func(a, b Entry) int { return b.Added.Compare(a.Added) })
	return out
}

// The sources a film comes from, as the desktop files them (sync.Film.Source), in the order the Library's chips list them.
const (
	SourceYouTube  = "youtube"
	SourceBilibili = "bilibili"
	SourceWeb      = "web"
	SourceLocal    = "local"
)

// Sources are the sources the chips offer.
var Sources = []string{SourceYouTube, SourceBilibili, SourceWeb, SourceLocal}

// SourceOf is the source f is filed under among Sources: a source the phone does not know is the web's.
func SourceOf(f sync.Film) string {
	if slices.Contains(Sources, f.Source) {
		return f.Source
	}
	return SourceWeb
}

// Scope is a library of the merged one, by its computer and its ID, or its name for a library only the phone knows of: the films of one library.
type Scope struct {
	Desktop, ID, Name string
	// Computer is the computer's name, for the switcher to say.
	Computer    string
	Locked      bool
	HasPassword bool
}

// Holds reports whether e is a film of the library s names.
func (s Scope) Holds(e Entry) bool {
	if e.Desktop != s.Desktop {
		return false
	}
	if e.Library.ID != "" && s.ID != "" {
		return e.Library.ID == s.ID
	}
	return e.Library.Name == s.Name
}

// Libraries are the libraries entries come from, once each, by computer in the order of computers and then those only the phone knows of, each in the order its films first come.
func Libraries(computers []Computer, entries []Entry) []Scope {
	var out []Scope
	seen := func(s Scope) bool {
		return slices.ContainsFunc(out, func(o Scope) bool { return o.Desktop == s.Desktop && o.ID == s.ID && o.Name == s.Name })
	}
	for _, c := range computers {
		for _, lib := range c.Listing.Libraries {
			if s := (Scope{Desktop: c.ID, ID: lib.ID, Name: lib.Name, Computer: c.Name, Locked: lib.Locked, HasPassword: lib.HasPassword}); !seen(s) {
				out = append(out, s)
			}
		}
	}
	for _, e := range entries {
		if slices.ContainsFunc(out, func(o Scope) bool { return o.Holds(e) }) {
			continue
		}
		out = append(out, Scope{Desktop: e.Desktop, ID: e.Library.ID, Name: e.Library.Name, Computer: e.Computer})
	}
	return out
}

// Filter is entries narrowed to library scope, nil for every library, and to source, "" for every source.
func Filter(entries []Entry, scope *Scope, source string) []Entry {
	var out []Entry
	for _, e := range entries {
		if (scope == nil || scope.Holds(e)) && (source == "" || SourceOf(e.Film) == source) {
			out = append(out, e)
		}
	}
	return out
}

// Hit is a line a search found: where it starts, and its text that matched.
type Hit struct {
	At   time.Duration
	Text string
}

// Result is a film a search found, with the line that matched where neither its title nor its work's did; ByWork is one found by its work's name (sync.Film.WorkTitle), which the list folds into the work's row (Fold).
type Result struct {
	Entry
	Hit    *Hit
	ByWork bool
	// searched is a film a search found, rather than one listed without a search.
	searched bool
}

// Search is the entries whose title or work's name has query in it, or one of whose lines kept on the phone, by film key, says it in the original or the translation, ignoring case; the first such line is the result's hit. An empty query finds every entry, without hits.
func Search(entries []Entry, lines map[string][]sync.Cue, query string) []Result {
	q := strings.ToLower(strings.TrimSpace(query))
	var out []Result
	for _, e := range entries {
		if q == "" {
			out = append(out, Result{Entry: e})
			continue
		}
		byWork := e.Film.WorkTitle != "" && strings.Contains(strings.ToLower(e.Film.WorkTitle), q)
		if byWork || strings.Contains(strings.ToLower(e.Film.Title), q) {
			out = append(out, Result{Entry: e, ByWork: byWork, searched: true})
			continue
		}
		for _, c := range lines[e.Key()] {
			text := ""
			switch {
			case strings.Contains(strings.ToLower(c.Original), q):
				text = c.Original
			case strings.Contains(strings.ToLower(c.Translation), q):
				text = c.Translation
			default:
				continue
			}
			out = append(out, Result{Entry: e, Hit: &Hit{At: time.Duration(c.StartMS) * time.Millisecond, Text: text}, searched: true})
			break
		}
	}
	return out
}

// resumeMargin is how close to its end a film left there counts as watched: the player starts it from the beginning again (player.Page).
const resumeMargin = 2 * time.Second

// Resumable reports whether p is where to go on with a film that plays for length, 0 when it is not known: past the start and short of the end.
func Resumable(p Position, length time.Duration) bool {
	return p.At > 0 && (length <= 0 || p.At < length-resumeMargin)
}

// Continue is at most n of the entries to go on with, the one left most recently first.
func Continue(entries []Entry, n int) []Entry {
	var out []Entry
	for _, e := range entries {
		if Resumable(e.Position, time.Duration(e.Film.DurationMS)*time.Millisecond) {
			out = append(out, e)
		}
	}
	slices.SortStableFunc(out, func(a, b Entry) int { return b.Position.When.Compare(a.Position.When) })
	if len(out) > n {
		out = out[:n]
	}
	return out
}
