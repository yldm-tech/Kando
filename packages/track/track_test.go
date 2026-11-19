package track

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kando/packages/core"
)

// clock stamps records one millisecond apart from a fixed time, so tests can tell which was written later.
func clock(t *testing.T) {
	t.Helper()
	now := time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)
	Now = func() time.Time { now = now.Add(time.Millisecond); return now }
	t.Cleanup(func() { Now = time.Now })
}

func video(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Episode 1.mkv")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func seg(start, end int, text string) core.Segment {
	return core.Segment{Start: time.Duration(start) * time.Second, End: time.Duration(end) * time.Second, Text: text}
}

func mustAppend(t *testing.T, path, device string, events ...Event) {
	t.Helper()
	if _, err := Append(path, device, events...); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, path string) Doc {
	t.Helper()
	doc, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestAppendNumbersRecordsAfterAHead(t *testing.T) {
	clock(t)
	path := video(t)
	mustAppend(t, path, "mac", Generation(Made{Source: "whisper", Original: LayerHeard, Translation: LayerDraft}, []core.Segment{seg(1, 2, "おはよう")}, nil)...)
	mustAppend(t, path, "mac", Event{Kind: KindGone, Line: "x"})
	doc := mustRead(t, path)
	if doc.Events[0].Kind != KindHead || doc.Events[0].Version != Version {
		t.Fatalf("first record = %+v, want the head", doc.Events[0])
	}
	heads := 0
	for i, e := range doc.Events {
		if e.Seq != int64(i+1) || e.Device != "mac" {
			t.Fatalf("record %d = seq %d device %q", i, e.Seq, e.Device)
		}
		if i > 0 && e.At <= doc.Events[i-1].At {
			t.Fatalf("record %d is not stamped after the one before it", i)
		}
		if e.Kind == KindHead {
			heads++
		}
	}
	if heads != 1 {
		t.Fatalf("%d heads, want 1", heads)
	}
	if got := Logs(path); len(got) != 1 || filepath.Base(got[0]) != ".Episode 1.track.mac.jsonl" {
		t.Fatalf("Logs = %v", got)
	}
}

func TestAViewShowsTheFilmsLines(t *testing.T) {
	clock(t)
	path := video(t)
	mustAppend(t, path, "mac", Generation(Made{Source: "whisper", Original: LayerDraft, Translation: LayerDraft},
		[]core.Segment{seg(5, 6, "second"), seg(1, 2, "first")},
		[]core.Segment{seg(5, 6, "第二"), seg(1, 2, " ")})...)
	lines := mustRead(t, path).View()
	if len(lines) != 2 || lines[0].Original.Text != "first" || lines[1].Original.Text != "second" {
		t.Fatalf("lines = %+v, want them in the order they show", lines)
	}
	if lines[0].Translation != (Text{}) {
		t.Fatalf("a line translated blank shows %+v, want no translation", lines[0].Translation)
	}
	if lines[1].Translation.Text != "第二" || lines[1].Translation.Layer != LayerDraft || lines[1].Translation.Source != "whisper" {
		t.Fatalf("translation = %+v", lines[1].Translation)
	}
	got := Bilingual(lines)
	if got[0].Text != "first" || got[1].Text != "第二\nsecond" || got[1].Start != 5*time.Second {
		t.Fatalf("Bilingual = %+v", got)
	}
}

func TestTheHighestLayerShowsAndThenTheLatest(t *testing.T) {
	clock(t)
	path := video(t)
	events := Generation(Made{Source: "whisper", Original: LayerHeard, Translation: LayerDraft}, []core.Segment{seg(1, 2, "heard")}, nil)
	mustAppend(t, path, "a", events...)
	line := mustRead(t, path).View()[0].ID
	// Another computer's final proofread, then this one's later draft: the final one still shows.
	mustAppend(t, path, "b", Event{Kind: KindRev, Line: line, Side: Original, Layer: LayerFinal, Text: "final"})
	mustAppend(t, path, "a", Event{Kind: KindRev, Line: line, Side: Original, Layer: LayerDraft, Text: "draft"})
	if got := mustRead(t, path).View()[0].Original.Text; got != "final" {
		t.Fatalf("original = %q, want the final layer's", got)
	}
	mustAppend(t, path, "a", Event{Kind: KindRev, Line: line, Side: Original, Layer: LayerFinal, Text: "final again"})
	if got := mustRead(t, path).View()[0].Original.Text; got != "final again" {
		t.Fatalf("original = %q, want the later of two in one layer", got)
	}
}

func TestTwoDevicesAtTheSameTimePickTheSameRevision(t *testing.T) {
	path := video(t)
	write := func(device, records string) {
		if err := os.WriteFile(LogPath(path, device), []byte(records), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a", `{"k":"head","n":1,"t":1,"v":1}
{"k":"gen","n":2,"t":2,"g":"g"}
{"k":"line","n":3,"t":3,"g":"g","l":"l","s":1000,"e":2000}
{"k":"rev","n":4,"t":9,"l":"l","side":"t","layer":4,"x":"from a"}
{"k":"done","n":5,"t":10,"g":"g"}
`)
	write("b", `{"k":"head","n":1,"t":1,"v":1}
{"k":"rev","n":2,"t":9,"l":"l","side":"t","layer":4,"x":"from b"}
`)
	if got := mustRead(t, path).View()[0].Translation.Text; got != "from b" {
		t.Fatalf("translation = %q, want the device whose id sorts last", got)
	}
}

func TestARedoShowsOnceItIsDone(t *testing.T) {
	clock(t)
	path := video(t)
	mustAppend(t, path, "mac", Generation(Made{Source: "import", Original: LayerFinal}, []core.Segment{seg(1, 2, "old")}, nil)...)
	// A redo that has begun and heard a line, not done.
	gen := NewID()
	mustAppend(t, path, "mac",
		Event{Kind: KindGen, Gen: gen, Source: "whisper"},
		Event{Kind: KindLine, Gen: gen, Line: "n1", Start: 1000, End: 2000},
		Event{Kind: KindRev, Line: "n1", Side: Original, Layer: LayerHeard, Text: "new"},
	)
	if got := mustRead(t, path).View(); len(got) != 1 || got[0].Original.Text != "old" {
		t.Fatalf("while the redo runs the film shows %+v, want the old lines", got)
	}
	mustAppend(t, path, "mac", Event{Kind: KindDone, Gen: gen})
	if got := mustRead(t, path).View(); len(got) != 1 || got[0].Original.Text != "new" {
		t.Fatalf("once the redo is done the film shows %+v, want its lines", got)
	}
}

// A redo begun before another and done after it, as one stopped and run again once the other was done, is what the film shows.
func TestTheGenerationDoneLastShows(t *testing.T) {
	clock(t)
	path := video(t)
	mustAppend(t, path, "mac", Generation(Made{Source: "import", Original: LayerFinal}, []core.Segment{seg(1, 2, "old")}, nil)...)
	early, late := NewID(), NewID()
	line := func(gen, id, text string) []Event {
		return []Event{{Kind: KindLine, Gen: gen, Line: id, Start: 1000, End: 2000}, {Kind: KindRev, Line: id, Side: Original, Layer: LayerHeard, Text: text}}
	}
	mustAppend(t, path, "mac", append([]Event{{Kind: KindGen, Gen: early, Source: "make"}}, line(early, "e1", "early")...)...)
	mustAppend(t, path, "mac", append(append([]Event{{Kind: KindGen, Gen: late, Source: "make"}}, line(late, "l1", "late")...), Event{Kind: KindDone, Gen: late})...)
	mustAppend(t, path, "mac", Event{Kind: KindDone, Gen: early})
	if got := mustRead(t, path).View(); len(got) != 1 || got[0].Original.Text != "early" {
		t.Fatalf("the film shows %+v, want the generation done last", got)
	}
}

func TestAFirstMakingShowsAsItsLinesCome(t *testing.T) {
	clock(t)
	path := video(t)
	gen := NewID()
	mustAppend(t, path, "mac",
		Event{Kind: KindGen, Gen: gen, Source: "whisper"},
		Event{Kind: KindLine, Gen: gen, Line: "l1", Start: 1000, End: 2000},
		Event{Kind: KindRev, Line: "l1", Side: Original, Layer: LayerHeard, Text: "first"},
		Event{Kind: KindLine, Gen: gen, Line: "l2", Start: 3000, End: 4000},
		Event{Kind: KindGone, Line: "l2"},
	)
	doc := mustRead(t, path)
	if doc.Shown() != gen || doc.Done(gen) {
		t.Fatalf("Shown = %q, Done = %v", doc.Shown(), doc.Done(gen))
	}
	if got := doc.View(); len(got) != 1 || got[0].ID != "l1" {
		t.Fatalf("lines = %+v, want the one not taken out", got)
	}
}

func TestARecordCutOffByACrashIsDropped(t *testing.T) {
	clock(t)
	path := video(t)
	mustAppend(t, path, "mac", Generation(Made{Source: "whisper", Original: LayerHeard}, []core.Segment{seg(1, 2, "kept")}, nil)...)
	log := LogPath(path, "mac")
	f, err := os.OpenFile(log, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(`{"k":"rev","n":99,"l":"x","x":"cut of`)
	_ = f.Close()
	if got := mustRead(t, path).View(); len(got) != 1 {
		t.Fatalf("a log cut off reads as %+v", got)
	}
	// Written by another program, the log is read again rather than trusted, and the cut record goes.
	mustAppend(t, path, "mac", Event{Kind: KindGone, Line: "y"})
	data, _ := os.ReadFile(log)
	if strings.Contains(string(data), "cut of") || !strings.HasSuffix(string(data), "\n") {
		t.Fatalf("log = %q, want the cut record gone", data)
	}
	doc := mustRead(t, path)
	last := doc.Events[len(doc.Events)-1]
	if last.Kind != KindGone || last.Seq != doc.Events[len(doc.Events)-2].Seq+1 {
		t.Fatalf("last record = %+v, want it numbered after the whole ones", last)
	}
}

func TestADeviceIDMustNameAFile(t *testing.T) {
	path := video(t)
	for _, device := range []string{"", "a.b", "../x", "a b"} {
		if _, err := Append(path, device, Event{Kind: KindGone}); !errors.Is(err, ErrDevice) {
			t.Errorf("Append(%q) = %v, want ErrDevice", device, err)
		}
	}
}

func TestANewerKandosLogIsNotWrittenTo(t *testing.T) {
	path := video(t)
	log := LogPath(path, "mac")
	if err := os.WriteFile(log, []byte(`{"k":"head","n":1,"t":1,"v":99}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !mustRead(t, path).Newer {
		t.Fatal("Newer = false for a log a newer Kando wrote")
	}
	if _, err := Append(path, "mac", Event{Kind: KindGone}); !errors.Is(err, ErrNewer) {
		t.Fatalf("Append = %v, want ErrNewer", err)
	}
}

func TestLogsAreTheVideosOwn(t *testing.T) {
	path := video(t)
	dir := filepath.Dir(path)
	for _, name := range []string{
		".Episode 1.track.phone.jsonl",
		".Episode 1.track.x.y.jsonl",     // not a device id
		".Episode 10.track.mac.jsonl",    // another video
		".Episode 1.track.mac.jsonl.tmp", // not a log
		".Episode 1.translation.json",    // another companion
	} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := Logs(path)
	if len(got) != 1 || filepath.Base(got[0]) != ".Episode 1.track.phone.jsonl" {
		t.Fatalf("Logs = %v", got)
	}
}

// A redo begun on a film whose first making was never done, which fails or is stopped, leaves the film showing the first making's lines, not the part the redo made.
func TestARedoNotDoneDoesNotTakeTheFirstMakingsPlace(t *testing.T) {
	clock(t)
	path := video(t)
	first, redo := NewID(), NewID()
	line := func(gen, id, text string) []Event {
		return []Event{{Kind: KindLine, Gen: gen, Line: id, Start: 1000, End: 2000}, {Kind: KindRev, Line: id, Side: Original, Layer: LayerHeard, Text: text}}
	}
	mustAppend(t, path, "mac", append([]Event{{Kind: KindGen, Gen: first, Source: "make"}}, line(first, "f1", "first")...)...)
	mustAppend(t, path, "mac", append([]Event{{Kind: KindGen, Gen: redo, Source: "make"}}, line(redo, "r1", "redo")...)...)
	if got := mustRead(t, path).View(); len(got) != 1 || got[0].Original.Text != "first" {
		t.Fatalf("the film shows %+v, want the first making's lines", got)
	}
}
