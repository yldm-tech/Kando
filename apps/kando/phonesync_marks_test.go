package main

import (
	"context"
	"slices"
	"testing"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/saved"
	psync "kando/packages/sync"
)

// TestPluginMarksForAPhone checks the word marks a paired phone asks for (OpFilmMarks): a film that is not open is marked on demand with 逐词标注's switches, each line's marks with the hash of its original; asked again, the marks kept answer without computing them again; through the storage relay, which does not wait, marks still being computed answer pending, and the next ask answers them; with every switch off a film has none.
func TestPluginMarksForAPhone(t *testing.T) {
	a, tt, updates, dir, c := phonePaired(t)
	for i := range a.library.Jobs {
		if a.library.Jobs[i].ID == "yt" {
			a.library.Jobs[i].SpokenLanguage = "en"
		}
	}
	pluginsDictInstalled(t, a, tt, updates, pluginWordlist("example.cefr", "en"), map[string][]byte{"words.tsv": []byte("hello\tb1\ngeneral\tc1\n")})
	marks := func(ctx context.Context) (psync.FilmMarks, error) { return c.FilmMarks(ctx, "yt") }

	h, err := phoneCall(t, a, tt, updates, c.Hello)
	if err != nil || !slices.Contains(h.Ops, psync.OpFilmMarks) {
		t.Fatalf("hello %+v, %v: no marks", h, err)
	}
	if got, err := phoneCall(t, a, tt, updates, marks); err != nil || got.Pending || got.Lines == nil || len(got.Lines) != 0 {
		t.Fatalf("with every switch off the film's marks are %+v, %v", got, err)
	}

	apply(tt, a, func() { a.SetMarkWordlist("example.cefr") })
	if a.player.film.video != "" {
		t.Fatal("a film is open")
	}
	got, err := phoneCall(t, a, tt, updates, marks)
	want := []psync.LineMarks{
		{Line: 0, Hash: psync.LineHash("Hello there"), Marks: []psync.Mark{{Start: 0, End: 5, Tone: psync.ToneAccent}}},
		{Line: 1, Hash: psync.LineHash("General Kenobi"), Marks: []psync.Mark{{Start: 0, End: 7, Tone: psync.ToneWarn}}},
	}
	if err != nil || got.Pending || !slices.EqualFunc(got.Lines, want, func(x, y psync.LineMarks) bool {
		return x.Line == y.Line && x.Hash == y.Hash && slices.Equal(x.Marks, y.Marks)
	}) {
		t.Fatalf("the marks of a film not open: %+v, %v", got, err)
	}
	if len(a.plugins.phone.kept) != 1 || len(a.plugins.phone.running) != 0 {
		t.Fatalf("kept %d films' marks, computing %d", len(a.plugins.phone.kept), len(a.plugins.phone.running))
	}

	// Asked again, the marks kept answer: a mark only they have comes back.
	onEngine(a, func() { a.plugins.phone.kept[0].lines[0] = []subtitle.Mark{{Start: 0, End: 5, Ruby: "kept"}} })
	got, err = phoneCall(t, a, tt, updates, marks)
	if err != nil || len(got.Lines) != 2 || got.Lines[0].Marks[0].Ruby != "kept" || len(a.plugins.phone.kept) != 1 {
		t.Fatalf("the marks were computed again: %+v, %v", got, err)
	}

	// A word put in the vocabulary, with its switch on, is another key: through the relay the first ask answers pending, and the marks once computed.
	book, err := saved.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	apply(tt, a, func() {
		a.saved.book = book
		a.saved.book.AddWord(saved.Word{Text: "kenobi", Spoken: "en"})
		a.SetMarkKnown(true)
	})
	relayed := func(ctx context.Context) (psync.FilmMarks, error) {
		return phoneMarks{a}.FilmMarks(ctx, psync.Caller{DeviceID: "phone", Relay: true}, psync.FilmRef{ID: "yt"})
	}
	if got, err := phoneCall(t, a, tt, updates, relayed); err != nil || !got.Pending || len(got.Lines) != 0 {
		t.Fatalf("marks being computed answered %+v, %v through the relay", got, err)
	}
	waitUntil(t, tt, a, updates, func() bool { return len(a.plugins.phone.running) == 0 })
	got, err = phoneCall(t, a, tt, updates, relayed)
	if err != nil || got.Pending || len(got.Lines) != 2 || !slices.Equal(got.Lines[1].Marks, []psync.Mark{{Start: 0, End: 7, Tone: psync.ToneWarn}, {Start: 8, End: 14, Underline: true}}) {
		t.Fatalf("the marks with the vocabulary: %+v, %v", got, err)
	}

	apply(tt, a, func() {
		a.SetMarkWordlist("")
		a.SetMarkKnown(false)
	})
	if got, err := phoneCall(t, a, tt, updates, marks); err != nil || got.Pending || len(got.Lines) != 0 {
		t.Fatalf("turned off, the film's marks are %+v, %v", got, err)
	}
	if _, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.FilmMarks, error) { return c.FilmMarks(ctx, "nope") }); err == nil {
		t.Fatal("a film not served has marks")
	}
}

// TestPluginMarksForAPhoneOfAGrowingFilm checks a film whose subtitles are made while it plays, which has more lines each time a phone asks: the lines there are marked, and at the next ask only the new ones are, the others keeping the marks they had; the desktop keeps one set of marks for the film.
func TestPluginMarksForAPhoneOfAGrowingFilm(t *testing.T) {
	a, tt, updates, _, _ := phonePaired(t)
	pluginsDictInstalled(t, a, tt, updates, pluginWordlist("example.cefr", "en"), map[string][]byte{"words.tsv": []byte("hello\tb1\ngeneral\tc1\n")})
	apply(tt, a, func() { a.SetMarkWordlist("example.cefr") })
	ask := func(originals ...string) pluginsMarks {
		t.Helper()
		var w *pluginsMarkWait
		var m pluginsMarks
		onEngine(a, func() { m, w, _ = a.pluginsMarksFor("/films/growing.mp4", "en", false, originals) })
		if w != nil {
			waitUntil(t, tt, a, updates, func() bool {
				select {
				case <-w.done:
					return true
				default:
					return false
				}
			})
			m = w.marks
		}
		return m
	}
	first := ask("Hello there")
	if len(first.lines) != 1 || !slices.Equal(first.lines[0], []subtitle.Mark{{Start: 0, End: 5, Tone: subtitle.ToneAccent}}) {
		t.Fatalf("the first part's marks: %+v", first.lines)
	}
	// A mark only the marks kept have shows that the line there is not marked again.
	onEngine(a, func() {
		a.plugins.phone.kept[0].byOriginal["Hello there"] = []subtitle.Mark{{Start: 0, End: 5, Ruby: "kept"}}
	})
	grown := ask("Hello there", "General Kenobi")
	if len(grown.lines) != 2 || grown.lines[0][0].Ruby != "kept" || !slices.Equal(grown.lines[1], []subtitle.Mark{{Start: 0, End: 7, Tone: subtitle.ToneWarn}}) {
		t.Fatalf("the grown film's marks: %+v", grown.lines)
	}
	if len(a.plugins.phone.kept) != 1 {
		t.Fatalf("kept %d sets of marks for one film", len(a.plugins.phone.kept))
	}
	if got := phoneMarksOf(grown); len(got.Lines) != 2 || got.Lines[1].Hash != psync.LineHash("General Kenobi") {
		t.Fatalf("the phone is sent %+v", got)
	}
}

// TestPluginMarksForAPhoneWaitForThePlugins checks a phone asking right after the app starts, before the plugins folder is read or while a dictionary turned on is still opening: the desktop answers pending, rather than marks without the word list or the readings that the phone would keep in place of its own, and computes nothing it would keep; once they are read, the marks.
func TestPluginMarksForAPhoneWaitForThePlugins(t *testing.T) {
	a, tt, updates, _, c := phonePaired(t)
	for i := range a.library.Jobs {
		if a.library.Jobs[i].ID == "yt" {
			a.library.Jobs[i].SpokenLanguage = "en"
		}
	}
	pluginsDictInstalled(t, a, tt, updates, pluginWordlist("example.cefr", "en"), map[string][]byte{"words.tsv": []byte("hello\tb1\ngeneral\tc1\n")})
	pluginsDictInstalled(t, a, tt, updates, pluginDictionary("example.cedict", "CEDICT", "zh", "en"), map[string][]byte{"words.tsv": []byte("你好\tnǐ hǎo\thello\n")})
	apply(tt, a, func() { a.SetMarkWordlist("example.cefr") })
	marks := func(ctx context.Context) (psync.FilmMarks, error) { return c.FilmMarks(ctx, "yt") }

	onEngine(a, func() { a.plugins.scanned = false })
	if got, err := phoneCall(t, a, tt, updates, marks); err != nil || !got.Pending || len(got.Lines) != 0 {
		t.Fatalf("before the plugins are read the marks are %+v, %v", got, err)
	}
	onEngine(a, func() { a.plugins.scanned = true })

	// The readings turned on while the dictionary is still opening.
	apply(tt, a, func() { a.SetMarkReadings(true) })
	var opened *pluginsDict
	onEngine(a, func() {
		opened = a.plugins.dicts["example.cedict"]
		a.plugins.dicts["example.cedict"] = &pluginsDict{hash: opened.hash}
	})
	if opened == nil || opened.d == nil {
		t.Fatal("the dictionary is not open")
	}
	if got, err := phoneCall(t, a, tt, updates, marks); err != nil || !got.Pending {
		t.Fatalf("with a dictionary opening the marks are %+v, %v", got, err)
	}
	if len(a.plugins.phone.kept) != 0 || len(a.plugins.phone.running) != 0 {
		t.Fatalf("marks were computed before the plugins were read: kept %d, running %d", len(a.plugins.phone.kept), len(a.plugins.phone.running))
	}
	onEngine(a, func() { a.plugins.dicts["example.cedict"] = opened })
	if got, err := phoneCall(t, a, tt, updates, marks); err != nil || got.Pending || len(got.Lines) != 2 {
		t.Fatalf("once the plugins are read the marks are %+v, %v", got, err)
	}
}

// TestPluginMarksForAPhoneKeepTheNewest checks two asks of one film for different lines, the second before the first is computed: the first run is stopped, those waiting for it are let go without marks, and the marks kept are the second's, whenever the first ends; a change of how the lines are marked stops a run too.
func TestPluginMarksForAPhoneKeepTheNewest(t *testing.T) {
	a, tt, updates, _, _ := phonePaired(t)
	pluginsDictInstalled(t, a, tt, updates, pluginWordlist("example.cefr", "en"), map[string][]byte{"words.tsv": []byte("hello\tb1\ngeneral\tc1\n")})
	apply(tt, a, func() { a.SetMarkWordlist("example.cefr") })
	start := func(originals ...string) *pluginsMarkWait {
		var w *pluginsMarkWait
		onEngine(a, func() { _, w, _ = a.pluginsMarksFor("/films/growing.mp4", "en", false, originals) })
		if w == nil {
			t.Fatal("no run started")
		}
		return w
	}
	older := start("Hello there")
	newer := start("Hello there", "General Kenobi")
	if !older.stopped || !older.stop.Load() || newer.stopped || len(a.plugins.phone.running) != 1 {
		t.Fatalf("the older run goes on: older stopped %v, newer stopped %v, running %d", older.stopped, newer.stopped, len(a.plugins.phone.running))
	}
	select {
	case <-older.done:
	default:
		t.Fatal("those waiting for the older run wait on")
	}
	waitUntil(t, tt, a, updates, func() bool { return len(a.plugins.phone.running) == 0 })
	// Whatever the older run's report, which may come after, the newer marks stay.
	for range 3 {
		select {
		case fn := <-updates:
			onEngine(a, fn)
		default:
		}
	}
	if kept := a.plugins.phone.kept; len(kept) != 1 || len(kept[0].originals) != 2 || newer.stopped {
		t.Fatalf("kept %+v", kept)
	}

	run := start("Hello there", "General Kenobi", "Bye")
	apply(tt, a, func() { a.SetMarkKnown(true) })
	if !run.stopped || len(a.plugins.phone.running) != 0 {
		t.Fatal("a change of how the lines are marked left the run going")
	}
}
