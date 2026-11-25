package catalog

import (
	"context"
	"slices"
	"testing"
	"time"

	"kando/apps/mobile/store"
	"kando/packages/sync"
	"kando/packages/sync/synctest"
)

var day = time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)

func film(id, title, source string, updated time.Time) sync.Film {
	return sync.Film{ID: id, Title: title, Source: source, DurationMS: 600_000, Updated: updated}
}

func keys(entries []Entry) []string {
	var out []string
	for _, e := range entries {
		out = append(out, e.Key())
	}
	return out
}

// A computer's listing has its libraries' films and, from the learning state, where each was left and on which device; a film's position on this phone is the phone's.
func TestLoadReadsTheFilmsAndWhereTheyWereLeft(t *testing.T) {
	d := synctest.New(t, "Studio")
	d.AddLibrary("lib", "Japanese Class")
	d.AddFilm(synctest.Film{Library: "lib", Video: []byte("v"), Film: film("f1", "Morning Talk", "youtube", day)})
	d.AddFilm(synctest.Film{Library: "lib", Video: []byte("v"), Film: film("f2", "Evening Talk", "web", day)})
	d.Offer("123456")
	c := sync.NewClient(d.Addr, d.Fingerprint, "")
	defer c.Close()
	ctx := context.Background()
	resp, err := c.Pair(ctx, sync.PairRequest{Code: "123456", Device: "Test Phone", Platform: sync.PlatformIOS})
	if err != nil {
		t.Fatal(err)
	}
	pos := func(ms int64) map[string]sync.Field {
		return map[string]sync.Field{sync.FieldPositionMS: {Value: sync.Value(ms)}}
	}
	d.SetState("lib", day, sync.Record{Kind: sync.KindPosition, ID: "f1", Film: "f1", Fields: pos(100_000)})
	if _, err := c.PutState(ctx, "lib", []sync.Record{{Kind: sync.KindPosition, ID: "f2", Film: "f2",
		Fields: map[string]sync.Field{sync.FieldPositionMS: {Value: sync.Value(5_000), Stamp: sync.Stamp{At: day.Add(time.Hour)}}}}}); err != nil {
		t.Fatal(err)
	}
	got, err := Load(ctx, c, resp.DeviceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Libraries) != 1 || got.Libraries[0].Name != "Japanese Class" || len(got.Libraries[0].Films) != 2 {
		t.Fatalf("listed %+v", got)
	}
	p := got.Libraries[0].Positions
	if f1 := p["f1"]; f1.At != 100*time.Second || f1.Mine || f1.Device != "Studio" || !f1.When.Equal(day) {
		t.Errorf("f1 was left at %+v, want 1:40 on Studio", f1)
	}
	if f2 := p["f2"]; f2.At != 5*time.Second || !f2.Mine {
		t.Errorf("f2 was left at %+v, want 0:05 on this phone", f2)
	}
}

// A position set to null, as a film watched to its end, or of another kind of record, is no position.
func TestPositionsSkipNullsAndOtherRecords(t *testing.T) {
	changes := sync.StateChanges{Devices: map[string]string{"mac": "MacBook Pro"}, Records: []sync.Record{
		{Kind: sync.KindPosition, ID: "a", Film: "a", Fields: map[string]sync.Field{sync.FieldPositionMS: {Value: []byte("null"), Stamp: sync.Stamp{At: day, Device: "mac"}}}},
		{Kind: sync.KindLine, ID: "l", Film: "b", Fields: map[string]sync.Field{sync.FieldStartMS: {Value: sync.Value(1000)}}},
		{Kind: sync.KindPosition, ID: "c", Film: "c", Fields: map[string]sync.Field{sync.FieldPositionMS: {Value: sync.Value(2500), Stamp: sync.Stamp{At: day, Device: "mac"}}}},
	}}
	got := Positions(changes, "phone")
	if len(got) != 1 || got["c"].At != 2500*time.Millisecond || got["c"].Device != "MacBook Pro" || got["c"].Mine {
		t.Fatalf("positions %+v", got)
	}
}

// The merge lists each film once, the latest first: the computer's as it lists it, with the later of its and the phone's positions, and a film only the phone keeps as the phone keeps it.
func TestMergeListsEachFilmOnceWithTheLatestPosition(t *testing.T) {
	computers := []Computer{{ID: "studio", Name: "Studio", Listing: Listing{Libraries: []Library{
		{Library: sync.Library{ID: "lib", Name: "Japanese Class"}, Films: []sync.Film{film("f1", "Morning Talk (new title)", "youtube", day), film("f2", "Evening Talk", "bilibili", day.Add(-time.Hour))},
			Positions: map[string]Position{"f1": {At: time.Minute, When: day.Add(time.Hour), Device: "MacBook Pro"}, "f2": {At: time.Minute, When: day, Device: "MacBook Pro"}}},
	}}}}
	kept := []store.Film{
		{Desktop: "studio", Library: "Japanese Class", Film: film("f1", "Morning Talk", "youtube", day), PositionMS: 30_000, PositionAt: day, Added: day},
		{Desktop: "studio", Library: "Japanese Class", Film: film("f2", "Evening Talk", "bilibili", day), PositionMS: 90_000, PositionAt: day.Add(2 * time.Hour), Added: day},
		{Desktop: "gone", Library: "Old", Film: film("f9", "Old Film", "local", day.Add(time.Hour)), Added: day},
	}
	got := Merge(computers, kept)
	if want := []string{"gone/f9", "studio/f1", "studio/f2"}; !slices.Equal(keys(got), want) {
		t.Fatalf("merged %q, want %q", keys(got), want)
	}
	if got[1].Film.Title != "Morning Talk (new title)" || got[1].Position.Device != "MacBook Pro" || got[1].Position.At != time.Minute {
		t.Errorf("a film left later on the computer: %+v", got[1])
	}
	if !got[2].Position.Mine || got[2].Position.At != 90*time.Second {
		t.Errorf("a film left later on the phone: %+v", got[2].Position)
	}
	if got[0].Library.Name != "Old" || got[0].Library.ID != "" || got[0].Position.At != 0 {
		t.Errorf("a film only the phone keeps: %+v", got[0])
	}
}

// A film the phone played from the beginning since its computer left it partway is the phone's, at the beginning, and goes from the films to go on with; one the phone never played keeps the computer's position.
func TestMergeTakesThePhonesReturnToTheBeginning(t *testing.T) {
	computers := []Computer{{ID: "studio", Name: "Studio", Listing: Listing{Libraries: []Library{
		{Library: sync.Library{ID: "lib", Name: "Japanese Class"}, Films: []sync.Film{film("f1", "Morning Talk", "youtube", day), film("f2", "Evening Talk", "youtube", day)},
			Positions: map[string]Position{"f1": {At: 4 * time.Minute, When: day, Device: "Studio"}, "f2": {At: 4 * time.Minute, When: day, Device: "Studio"}}},
	}}}}
	kept := []store.Film{
		{Desktop: "studio", Library: "Japanese Class", Film: film("f1", "Morning Talk", "youtube", day), PositionAt: day.Add(time.Minute), Added: day},
		{Desktop: "studio", Library: "Japanese Class", Film: film("f2", "Evening Talk", "youtube", day), Added: day},
	}
	got := Merge(computers, kept)
	if p := got[0].Position; !p.Mine || p.At != 0 {
		t.Errorf("a film played from the beginning on the phone since: %+v", p)
	}
	if p := got[1].Position; p.Mine || p.At != 4*time.Minute {
		t.Errorf("a film the phone never played: %+v", p)
	}
	if c := keys(Continue(got, 5)); !slices.Equal(c, []string{"studio/f2"}) {
		t.Errorf("go on with %q", c)
	}
}

// A film the phone keeps is ordered by when it last changed on its computer whether or not its computer lists it, so it keeps its place while its computer is out of reach; one whose computer gave no time, by when the phone downloaded it.
func TestMergeOrdersAKeptFilmTheSameWhetherItsComputerAnswers(t *testing.T) {
	listed := []Computer{{ID: "studio", Name: "Studio", Listing: Listing{Libraries: []Library{
		{Library: sync.Library{ID: "lib", Name: "Japanese Class"}, Films: []sync.Film{film("f1", "Morning Talk", "youtube", day), film("f2", "Evening Talk", "youtube", day.Add(-2*time.Hour))}},
	}}}}
	unanswered := []Computer{{ID: "studio", Name: "Studio"}}
	kept := []store.Film{
		{Desktop: "studio", Library: "Japanese Class", Film: film("f2", "Evening Talk", "youtube", day.Add(-2*time.Hour)), Added: day.Add(time.Hour)},
		{Desktop: "gone", Library: "Old", Film: sync.Film{ID: "f9", Title: "Old Film"}, Added: day.Add(-time.Hour)},
	}
	want := []string{"studio/f1", "gone/f9", "studio/f2"}
	if got := keys(Merge(listed, kept)); !slices.Equal(got, want) {
		t.Errorf("with the computer answering, merged %q, want %q", got, want)
	}
	if got, want := keys(Merge(unanswered, kept)), []string{"gone/f9", "studio/f2"}; !slices.Equal(got, want) {
		t.Errorf("with the computer out of reach, merged %q, want %q", got, want)
	}
}

// The library switcher lists the computers' libraries and those only the phone knows of, and a library holds its own films.
func TestLibrariesAndFilters(t *testing.T) {
	computers := []Computer{{ID: "studio", Name: "Studio", Listing: Listing{Libraries: []Library{
		{Library: sync.Library{ID: "a", Name: "Japanese Class"}, Films: []sync.Film{film("f1", "One", "youtube", day)}},
		{Library: sync.Library{ID: "b", Name: "Movies"}, Films: []sync.Film{film("f2", "Two", "", day)}},
	}}}}
	kept := []store.Film{
		{Desktop: "studio", Library: "Japanese Class", Film: film("f1", "One", "youtube", day)},
		{Desktop: "gone", Library: "Old", Film: film("f3", "Three", "local", day)},
	}
	entries := Merge(computers, kept)
	libs := Libraries(computers, entries)
	if len(libs) != 3 || libs[0].Name != "Japanese Class" || libs[1].Name != "Movies" || libs[2].Name != "Old" || libs[2].Desktop != "gone" {
		t.Fatalf("libraries %+v", libs)
	}
	if got := keys(Filter(entries, &libs[0], "")); !slices.Equal(got, []string{"studio/f1"}) {
		t.Errorf("Japanese Class holds %q", got)
	}
	if got := keys(Filter(entries, nil, SourceWeb)); !slices.Equal(got, []string{"studio/f2"}) {
		t.Errorf("a film of no known source is the web's: the web holds %q", got)
	}
	if got := keys(Filter(entries, &libs[2], SourceLocal)); !slices.Equal(got, []string{"gone/f3"}) {
		t.Errorf("Old's local films are %q", got)
	}
}

// A search finds titles and the lines kept on the phone, in either language and any case, with the first line that matched.
func TestSearchFindsTitlesAndKeptLines(t *testing.T) {
	entries := []Entry{
		{Desktop: "d", Film: film("f1", "Morning Talk", "youtube", day)},
		{Desktop: "d", Film: film("f2", "Rivers", "youtube", day)},
		{Desktop: "d", Film: film("f3", "Tides", "youtube", day)},
	}
	lines := map[string][]sync.Cue{
		"d/f2": {{StartMS: 1000, Original: "Water flows.", Translation: "水在流。"}, {StartMS: 4000, Original: "The river TALKS.", Translation: "河在说话。"}},
		"d/f3": {{StartMS: 2000, Original: "Waves.", Translation: "海浪。"}},
	}
	got := Search(entries, lines, " talk")
	if len(got) != 2 || got[0].Film.ID != "f1" || got[0].Hit != nil || got[1].Film.ID != "f2" || got[1].Hit == nil || got[1].Hit.At != 4*time.Second || got[1].Hit.Text != "The river TALKS." {
		t.Fatalf("searching talk found %+v", got)
	}
	got = Search(entries, lines, "海浪")
	if len(got) != 1 || got[0].Hit == nil || got[0].Hit.Text != "海浪。" {
		t.Fatalf("searching a translation found %+v", got)
	}
	if got := Search(entries, lines, ""); len(got) != 3 {
		t.Fatalf("an empty search found %d", len(got))
	}
}

// The films to go on with are those left partway, the latest left first.
func TestContinueListsFilmsLeftPartway(t *testing.T) {
	long := film("x", "x", "", day)
	at := func(id string, pos time.Duration, when time.Time) Entry {
		f := long
		f.ID = id
		return Entry{Desktop: "d", Film: f, Position: Position{At: pos, When: when}}
	}
	entries := []Entry{
		at("start", 0, day),
		at("old", time.Minute, day),
		at("end", 10*time.Minute-time.Second, day.Add(3*time.Hour)),
		at("new", time.Minute, day.Add(time.Hour)),
		at("newer", time.Minute, day.Add(2*time.Hour)),
	}
	if got := keys(Continue(entries, 2)); !slices.Equal(got, []string{"d/newer", "d/new"}) {
		t.Fatalf("go on with %q", got)
	}
}

// What a computer lists through the storage is its libraries in its order, each with its films there; a film of a library it does not name is left out.
func TestFromRelayListsTheLibrariesFilms(t *testing.T) {
	shelf := sync.RelayShelf{
		Libraries: []sync.Library{{ID: "b", Name: "Movies"}, {ID: "a", Name: "Japanese Class"}},
		Films: []sync.RelayFilm{
			{Library: "a", Film: film("f1", "One", "youtube", day)},
			{Library: "b", Film: film("f2", "Two", "web", day)},
			{Library: "gone", Film: film("f3", "Three", "web", day)},
		},
	}
	got := FromRelay(shelf)
	if len(got.Libraries) != 2 || got.Libraries[0].Name != "Movies" || len(got.Libraries[0].Films) != 1 || got.Libraries[0].Films[0].ID != "f2" || got.Libraries[1].Films[0].ID != "f1" || got.Libraries[0].Positions != nil {
		t.Fatalf("listed %+v", got)
	}
}

// A film of a NAS matched to a computer's film of the same file (store.Film.Lines) is that film: listed once, under the computer's key, with the computer's title and library, and where it was left last of the computer's position and those of the phone's two copies; with the computer out of reach it is listed as the phone last took it from the computer. A film of a NAS matched to nothing is listed as its own.
func TestMergeListsAMatchedSourceFilmOnce(t *testing.T) {
	listed := film("f2", "Holiday on the NAS", "local", day)
	lines := &store.Lines{Desktop: "d1", Library: "lib", LibraryName: "Japanese Class", Film: listed}
	nasFilm := store.Film{Desktop: store.SourceOwner("src"), Library: "Home NAS", Source: "src", Path: "Holiday.mp4", Streamed: true, Added: day,
		Film: sync.Film{ID: store.SourceFilmID("Holiday.mp4"), Title: "Holiday", Source: "local"}, Lines: lines,
		PositionMS: 4_000, PositionAt: day.Add(2 * time.Hour)}
	computer := Computer{ID: "d1", Name: "Studio", Listing: Listing{Libraries: []Library{{Library: sync.Library{ID: "lib", Name: "Japanese Class"}, Films: []sync.Film{listed},
		Positions: map[string]Position{"f2": {At: 9 * time.Second, When: day.Add(time.Hour), Device: "Studio"}}}}}}
	downloaded := store.Film{Desktop: "d1", Library: "Japanese Class", Film: listed, Video: "video.mp4", Added: day, PositionMS: 2_000, PositionAt: day.Add(30 * time.Minute)}

	got := Merge([]Computer{computer}, []store.Film{nasFilm, downloaded})
	if len(got) != 1 || got[0].Key() != "d1/f2" || got[0].Film.Title != "Holiday on the NAS" || got[0].Computer != "Studio" || got[0].Library.ID != "lib" {
		t.Fatalf("merged %+v", got)
	}
	if p := got[0].Position; p.At != 4*time.Second || !p.Mine {
		t.Errorf("the film was left at %+v, want 0:04 on the phone, the latest", p)
	}
	if c := Continue(got, 6); len(c) != 1 || c[0].Key() != "d1/f2" {
		t.Errorf("Continue %v", keys(c))
	}

	got = Merge(nil, []store.Film{nasFilm})
	if len(got) != 1 || got[0].Key() != "d1/f2" || got[0].Film.Title != "Holiday on the NAS" || got[0].Library.Name != "Japanese Class" || got[0].Position.At != 4*time.Second {
		t.Fatalf("merged with the computer out of reach %+v", got)
	}

	unmatched := nasFilm
	unmatched.Lines = nil
	got = Merge([]Computer{computer}, []store.Film{unmatched})
	if want := []string{"d1/f2", unmatched.Key()}; !slices.Equal(keys(got), want) {
		t.Errorf("merged a film matched to nothing as %v, want %v", keys(got), want)
	}
}
