package widgets

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	gosync "sync"
	"testing"
	"time"

	"kando/apps/mobile/learning"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/widget"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

// now is the clock of the tests' sessions: a morning, so the midnights ahead are the next days'.
func now() time.Time { return time.Date(2026, 11, 17, 9, 0, 0, 0, time.Local) }

// texts are the texts and symbols of v and the views it holds, in order.
func texts(v widget.View) []string {
	var out []string
	var walk func(v widget.View)
	walk = func(v widget.View) {
		switch v := v.(type) {
		case widget.Text:
			out = append(out, v.Content)
		case widget.Symbol:
			out = append(out, "symbol:"+v.Name)
		case widget.Image:
			out = append(out, "image")
		case widget.Progress:
			out = append(out, "progress")
		case widget.Column:
			for _, c := range v.Children {
				walk(c)
			}
		case widget.Row:
			for _, c := range v.Children {
				walk(c)
			}
		case widget.Stack:
			for _, c := range v.Children {
				walk(c)
			}
		}
	}
	walk(v)
	return out
}

// The study widget's timeline holds today's streak and what is due, then an entry from each midnight ahead, dated by it; the Lock Screen's ring shows what is due alone.
func TestTheStudyWidgetFollowsTheDays(t *testing.T) {
	m := locales.In("en")
	var w Shown
	for i := range w.Days {
		w.Days[i] = Day{At: now().AddDate(0, 0, i), Streak: 5 - i, Due: 3 + i}
	}
	study := Entries(m, w)[Study]
	if len(study) != ahead+1 {
		t.Fatalf("the study widget has %d entries, want %d", len(study), ahead+1)
	}
	if !study[0].Date.IsZero() {
		t.Errorf("today's entry is dated %v, want the time of the update", study[0].Date)
	}
	for i, e := range study[1:] {
		if !e.Date.Equal(w.Days[i+1].At) {
			t.Errorf("entry %d is dated %v, want %v", i+1, e.Date, w.Days[i+1].At)
		}
	}
	got := strings.Join(texts(study[0].View), "|")
	want := "symbol:flame.fill|Streak|5|days|3 to review|Tap to review"
	if got != want {
		t.Errorf("today's study widget shows %q, want %q", got, want)
	}
	if got := strings.Join(texts(study[0].Families[widget.Circular]), "|"); got != "3|to review" {
		t.Errorf("the Lock Screen's ring shows %q, want the 3 due", got)
	}
	if got := texts(study[2].View); !slices.Contains(got, "3") || !slices.Contains(got, "5 to review") {
		t.Errorf("the entry of the second midnight shows %q, want its streak of 3 and 5 due", got)
	}
}

// The Continue widget shows the film left partway over its poster, the line it was left on and how far it played, and the medium one the title, the line and today's streak and what is due beside the poster; without a film it says there is none.
func TestTheContinueWidgetShowsTheFilmLeftPartway(t *testing.T) {
	m := locales.In("en")
	w := Shown{Line: "你好吗？"}
	w.Days[0] = Day{At: now(), Streak: 2, Due: 1}
	w.Film = Film{Key: "d/f1", Desktop: "d", ID: "f1", Title: "Morning Talk", At: 3500 * time.Millisecond, Length: 10 * time.Second, PosterFile: "/films/poster.jpg"}
	cont := Entries(m, w)[Continue]
	if len(cont) != 1 {
		t.Fatalf("the Continue widget has %d entries, want 1", len(cont))
	}
	if img, ok := cont[0].Background.(widget.Image); !ok || img.File != w.Film.PosterFile || !img.Cover {
		t.Errorf("the Continue widget's background is %#v, want the poster covering it", cont[0].Background)
	}
	if got, want := strings.Join(texts(cont[0].View), "|"), "Continue|你好吗？|progress"; got != want {
		t.Errorf("the small Continue widget shows %q, want %q", got, want)
	}
	if got, want := strings.Join(texts(cont[0].Families[widget.Medium]), "|"), "image|Continue · 35%|Morning Talk|你好吗？|1 to review · 2-day streak"; got != want {
		t.Errorf("the medium Continue widget shows %q, want %q", got, want)
	}

	w.Line = ""
	if got := texts(Entries(m, w)[Continue][0].View); !slices.Contains(got, "Morning Talk") {
		t.Errorf("a film whose lines the phone does not keep shows %q, want its title", got)
	}
	w.Film = Film{}
	none := Entries(m, w)[Continue][0]
	if none.Background != nil || none.Families != nil {
		t.Errorf("the Continue widget without a film has a background %#v or families %v", none.Background, none.Families)
	}
	if got := texts(none.View); !slices.Contains(got, "Nothing to continue") {
		t.Errorf("the Continue widget without a film shows %q", got)
	}
}

// Kept, the widgets are written once the session has read the store, with the film kept on the phone left partway, the line it was left on, its poster, and the streak of the days films played; a film played on writes them again, and nothing that changes what they show writes them no more.
func TestKeepWritesTheWidgetsAsTheyChange(t *testing.T) {
	dir := t.TempDir()
	st := store.Open(dir)
	f := store.Film{Desktop: "desk", Library: "Japanese Class", Video: "video.mp4", Poster: "poster.jpg", Added: now(),
		PositionMS: 3500, PositionAt: now().Add(-time.Hour),
		Film: sync.Film{ID: "f1", Title: "Morning Talk", DurationMS: 10_000, Spoken: "ja", Translation: "简体中文"}}
	if err := st.SaveFilm(f); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCues(f.Desktop, f.Film.ID, []sync.Cue{
		{StartMS: 1000, EndMS: 2000, Original: "Good morning.", Translation: "早上好。"},
		{StartMS: 3000, EndMS: 4500, Original: "How are you?", Translation: "你好吗？"},
	}); err != nil {
		t.Fatal(err)
	}
	poster := filepath.Join(st.FilmDir(f.Desktop, f.Film.ID), f.Poster)
	if err := os.WriteFile(poster, []byte("jpeg"), 0o600); err != nil {
		t.Fatal(err)
	}
	watched := learning.Watched{}
	watched.Add(now(), 20*time.Minute)
	watched.Add(now().AddDate(0, 0, -1), 20*time.Minute)
	if err := learning.Open(dir).SaveWatched(watched); err != nil {
		t.Fatal(err)
	}

	var mu gosync.Mutex
	published := map[string][][]widget.Entry{}
	var s *session.Session
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		s = session.New(cx, session.Config{Dir: dir, Device: "Test Phone", Platform: sync.PlatformIOS, Now: now})
		Keep(cx, s, func(kind string, entries ...widget.Entry) error {
			mu.Lock()
			defer mu.Unlock()
			published[kind] = append(published[kind], entries)
			return nil
		})
		return core.View{}
	}, nsgotest.Environment(core.Environment{Locale: "en"}))
	last := func(kind string) []widget.Entry {
		mu.Lock()
		defer mu.Unlock()
		list := published[kind]
		if len(list) == 0 {
			return nil
		}
		return list[len(list)-1]
	}
	count := func(kind string) int {
		mu.Lock()
		defer mu.Unlock()
		return len(published[kind])
	}
	app.WaitUntil(10*time.Second, func() bool {
		study := last(Study)
		return len(study) > 0 && slices.Contains(texts(study[0].View), "2")
	})
	cont := last(Continue)
	if got, want := strings.Join(texts(cont[0].View), "|"), "Continue|你好吗？|progress"; got != want {
		t.Fatalf("the Continue widget shows %q, want %q", got, want)
	}
	if img, ok := cont[0].Background.(widget.Image); !ok || img.File != poster {
		t.Fatalf("the Continue widget's background is %#v, want the poster on the phone", cont[0].Background)
	}

	before := count(Continue)
	app.Post(func() { s.SavePosition(f.Key(), 1500*time.Millisecond) })
	app.WaitUntil(10*time.Second, func() bool {
		c := last(Continue)
		return count(Continue) > before && len(c) > 0 && slices.Contains(texts(c[0].View), "早上好。")
	})
}

// What the widgets show is the same whenever it was read: today's entry is dated by the time of the read, which changes on every read and writes nothing, while a change to a count, the film or the language does.
func TestTheSameSnapshotIsNotWrittenAgain(t *testing.T) {
	read := func(at time.Time) change {
		var snap Snapshot
		for i := range snap.Days {
			snap.Days[i] = Day{At: at, Streak: 2, Due: 1}
			if i > 0 {
				snap.Days[i].At = time.Date(2026, 11, 17+i, 0, 0, 0, 0, time.Local)
			}
		}
		snap.Film = Film{Key: "d/f1", Title: "Morning Talk", At: time.Second}
		return change{m: locales.In("en"), snap: snap}
	}
	a := read(now())
	if !same(a, read(now().Add(time.Minute))) {
		t.Error("a read a minute later shows something else")
	}
	b := read(now())
	b.snap.Days[1].Due++
	c := read(now())
	c.snap.Film.At = 2 * time.Second
	d := read(now())
	d.m = locales.In("ja")
	for name, x := range map[string]change{"a count": b, "the film": c, "the language": d} {
		if same(a, x) {
			t.Errorf("a change of %s shows the same", name)
		}
	}
}

// Days that passed while the app was in the background are read again once it shows: the study widget's timeline is dated from the day the app shows on, not left on the midnights of the day it was last written.
func TestKeepReadsTheDaysAgainWhenTheAppShows(t *testing.T) {
	dir := t.TempDir()
	var mu gosync.Mutex
	clock := now()
	published := [][]widget.Entry{}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		s := session.New(cx, session.Config{Dir: dir, Device: "Test Phone", Platform: sync.PlatformIOS, Now: func() time.Time {
			mu.Lock()
			defer mu.Unlock()
			return clock
		}})
		Keep(cx, s, func(kind string, entries ...widget.Entry) error {
			if kind == Study {
				mu.Lock()
				defer mu.Unlock()
				published = append(published, entries)
			}
			return nil
		})
		return core.View{}
	}, nsgotest.Environment(core.Environment{Locale: "en"}))
	firstMidnight := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		if len(published) == 0 {
			return time.Time{}
		}
		return published[len(published)-1][1].Date
	}
	tomorrow := time.Date(2026, 11, 18, 0, 0, 0, 0, time.Local)
	app.WaitUntil(10*time.Second, func() bool { return firstMidnight().Equal(tomorrow) })

	app.SetEnvironment(core.Environment{Locale: "en", Lifecycle: core.LifecycleBackground})
	mu.Lock()
	clock = clock.AddDate(0, 0, 5)
	mu.Unlock()
	app.SetEnvironment(core.Environment{Locale: "en", Lifecycle: core.LifecycleActive})
	want := tomorrow.AddDate(0, 0, 5)
	app.WaitUntil(10*time.Second, func() bool { return firstMidnight().Equal(want) })
}
