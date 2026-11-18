package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/phonesync"
	"kando/apps/kando/shelf"
	"kando/packages/core"
	"kando/packages/media"
	"kando/packages/study"
	psync "kando/packages/sync"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// phoneStateFind is the record kind id among records.
func phoneStateFind(t *testing.T, records []psync.Record, kind, id string) psync.Record {
	t.Helper()
	i := slices.IndexFunc(records, func(r psync.Record) bool { return r.Kind == kind && r.ID == id })
	if i < 0 {
		t.Fatalf("no %s %s among %+v", kind, id, records)
	}
	return records[i]
}

func phoneStateString(t *testing.T, f psync.Field) string {
	t.Helper()
	var s string
	if err := json.Unmarshal(f.Value, &s); err != nil {
		t.Fatalf("%s is not a string: %v", f.Value, err)
	}
	return s
}

// phoneStateLibrary is phonePaired with the open library holding a resume position, a saved line and a correction of its talk, whose video's path it returns besides, and the clock the server and the saved lines go by, which a test moves past the desktop's changes before the phone makes its own.
func phoneStateLibrary(t *testing.T) (a *app, p phoneTT, dir, talk string, c *psync.Client, clock *phoneClock) {
	t.Helper()
	a, tt, updates, dir, clock := phoneSyncApp(t)
	a.pipeline = &fakePipeline{block: true}
	a.saved.now = clock.Now
	phoneSyncOn(t, a, tt, updates)
	c = phoneClient(t, a, "")
	if _, err := phonePair(t, a, tt, updates, c, a.phone.offer.Code); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		a.queue.stopActive(context.Canceled)
		waitUntil(t, tt, a, updates, func() bool { return !a.running() })
	})
	p = phoneTT{a: a, tt: tt, updates: updates}
	talk = filepath.Join(dir, "Rust Talk", "talk.mp4")
	a.player.positions = map[string]time.Duration{talk: 90 * time.Second}
	a.savedSync()
	a.saved.book.Toggle(study.Line{Video: a.saved.book.Key(talk), Start: time.Second, End: 2500 * time.Millisecond, Original: "Hello there", Translation: "你好", Film: "Rust Talk"})
	a.savedWrite()
	general := "将军"
	if err := media.SaveCorrections(media.CorrectionsPath(talk), []core.Correction{{Span: core.Span{Start: 3 * time.Second, End: 4 * time.Second}, Translation: &general}}); err != nil {
		t.Fatal(err)
	}
	refresh(tt, a)
	return a, p, dir, talk, c, clock
}

// phoneTT is what a phone's calls run against (phoneCall).
type phoneTT struct {
	a       *app
	tt      *nsgotest.App
	updates chan func()
}

func phoneAsk[T any](t *testing.T, p phoneTT, call func(ctx context.Context) (T, error)) (T, error) {
	t.Helper()
	return phoneCall(t, p.a, p.tt, p.updates, call)
}

// A phone reads a library's learning state, changes it, and the desktop keeps the changes where it keeps its own: the resume position with the shelf, the line in the study record, the corrections in the film's file, and a line the phone saved in the study record too.
func TestPhoneSyncSyncsTheLearningState(t *testing.T) {
	a, p, dir, talk, c, clock := phoneStateLibrary(t)
	lib := phoneLibraryID(dir)
	h, err := phoneAsk(t, p, c.Hello)
	if err != nil || !slices.Contains(h.Ops, psync.OpStatePut) || !slices.Equal(h.State, phonesync.StateKinds) {
		t.Fatalf("hello %+v, %v", h, err)
	}

	got, err := phoneAsk(t, p, func(ctx context.Context) (psync.StateChanges, error) { return c.State(ctx, lib, "", 0) })
	if err != nil {
		t.Fatal(err)
	}
	if !got.Reset || got.Library != lib || got.Devices[a.saved.device] == "" || got.Devices[a.phone.devices[0].ID] != "Evan's iPhone" {
		t.Fatalf("state %+v", got)
	}
	pos := phoneStateFind(t, got.Records, psync.KindPosition, "yt")
	if string(pos.Fields[psync.FieldPositionMS].Value) != "90000" || pos.Fields[psync.FieldPositionMS].Device != a.saved.device {
		t.Errorf("position %+v", pos)
	}
	lineID := a.saved.book.Lines[0].ID
	line := phoneStateFind(t, got.Records, psync.KindLine, lineID)
	if line.Film != "yt" || phoneStateString(t, line.Fields[psync.FieldOriginal]) != "Hello there" || string(line.Fields[psync.FieldStartMS].Value) != "1000" {
		t.Errorf("line %+v", line)
	}
	corr := phoneStateFind(t, got.Records, psync.KindCorrection, psync.CorrectionID("yt", 3000, 4000))
	if phoneStateString(t, corr.Fields[psync.FieldTranslation]) != "将军" || !psync.IsNull(corr.Fields[psync.FieldOriginal].Value) {
		t.Errorf("correction %+v", corr)
	}
	if _, err := os.Stat(filepath.Join(dir, phonesync.LedgerFile)); err != nil {
		t.Errorf("the ledger was not kept in the library: %v", err)
	}

	// The phone changes the note, moves the position, corrects another line and saves a line of the interview.
	interview := phoneFilmID(films.Film{VideoPath: filepath.Join(dir, "Interview", "interview.mp4")})
	noteBase := line.Fields[psync.FieldNote].Stamp
	clock.Add(time.Minute)
	now := clock.Now()
	field := func(v any) psync.Field { return psync.Field{Value: psync.Value(v), Stamp: psync.Stamp{At: now}} }
	put := []psync.Record{
		{Kind: psync.KindLine, ID: lineID, Film: "yt", Fields: map[string]psync.Field{psync.FieldNote: {Value: psync.Value("from the phone"), Stamp: psync.Stamp{At: now}, Base: &noteBase}}},
		{Kind: psync.KindPosition, ID: "yt", Film: "yt", Fields: map[string]psync.Field{psync.FieldPositionMS: field(120000)}},
		{Kind: psync.KindCorrection, ID: psync.CorrectionID("yt", 1000, 2500), Film: "yt", Fields: map[string]psync.Field{psync.FieldTranslation: field("你好啊")}},
		{Kind: psync.KindLine, ID: "phoneline", Film: interview, Fields: map[string]psync.Field{
			psync.FieldStartMS: field(500), psync.FieldEndMS: field(1000), psync.FieldOriginal: field("Good morning"), psync.FieldTranslation: field("早上好"),
			psync.FieldTitle: field("Interview"), psync.FieldSavedAt: field(now), psync.FieldNote: field(""), psync.FieldReview: field(psync.Review{Due: now}),
		}},
	}
	res, err := phoneAsk(t, p, func(ctx context.Context) (psync.StatePutResult, error) { return c.PutState(ctx, lib, put) })
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Records) != 4 || len(res.Conflicts) != 0 {
		t.Fatalf("put %+v", res)
	}
	if i := a.saved.book.Index(lineID); i < 0 || a.saved.book.Lines[i].Note != "from the phone" || a.saved.book.Lines[i].Stamps.Note.Device != a.phone.devices[0].ID {
		t.Errorf("the desktop's line %+v", a.saved.book.Lines)
	}
	if i := a.saved.book.Index("phoneline"); i < 0 || a.saved.book.Lines[i].Video != "Interview/interview.mp4" || a.saved.book.Lines[i].Start != 500*time.Millisecond || a.saved.book.Lines[i].Original != "Good morning" {
		t.Errorf("the phone's line on the desktop %+v", a.saved.book.Lines)
	}
	if disk, err := study.Load(dir); err != nil || disk.Index("phoneline") < 0 {
		t.Errorf("the study record on disk lacks the phone's line: %v", err)
	}
	if a.player.positions[talk] != 120*time.Second {
		t.Errorf("position %v, want the phone's 2:00", a.player.positions[talk])
	}
	edits, err := media.LoadCorrections(media.CorrectionsPath(talk))
	if err != nil || len(edits) != 2 {
		t.Fatalf("corrections %+v, %v", edits, err)
	}

	// The desktop changes the note too; the phone hears the state changed, and its own stale change is set aside as a conflict.
	list, err := phoneAsk(t, p, c.Jobs)
	if err != nil {
		t.Fatal(err)
	}
	clock.Add(time.Minute)
	a.saved.book.SetNote(lineID, "from the desk")
	a.savedWrite()
	refresh(p.tt, a)
	ev, err := phoneAsk(t, p, func(ctx context.Context) (psync.Events, error) {
		return c.Events(ctx, list.Epoch, list.Rev, time.Second)
	})
	if err != nil || !slices.ContainsFunc(ev.Events, func(e psync.Event) bool { return e.Kind == psync.EventState && e.ID == lib }) {
		t.Fatalf("events %+v, %v; want the library's state", ev, err)
	}
	since, err := phoneAsk(t, p, func(ctx context.Context) (psync.StateChanges, error) { return c.State(ctx, lib, got.Epoch, res.Rev) })
	if err != nil || since.Reset {
		t.Fatalf("changes %+v, %v", since, err)
	}
	if l := phoneStateFind(t, since.Records, psync.KindLine, lineID); phoneStateString(t, l.Fields[psync.FieldNote]) != "from the desk" {
		t.Errorf("changed line %+v", l)
	}
	stale := res.Records[0].Fields[psync.FieldNote].Stamp
	clock.Add(time.Minute)
	later := clock.Now()
	_, err = phoneAsk(t, p, func(ctx context.Context) (psync.StatePutResult, error) {
		return c.PutState(ctx, lib, []psync.Record{{Kind: psync.KindLine, ID: lineID, Fields: map[string]psync.Field{psync.FieldNote: {Value: psync.Value("phone again"), Stamp: psync.Stamp{At: later}, Base: &stale}}}})
	})
	if err != nil {
		t.Fatal(err)
	}
	open, err := phoneAsk(t, p, func(ctx context.Context) (psync.StateChanges, error) { return c.State(ctx, lib, "", 0) })
	if err != nil || len(open.Conflicts) != 1 {
		t.Fatalf("conflicts %+v, %v", open.Conflicts, err)
	}
	conflict := open.Conflicts[0]
	if phoneStateString(t, conflict.Lost) != "from the desk" || conflict.Record != lineID || conflict.Field != psync.FieldNote {
		t.Errorf("conflict %+v", conflict)
	}

	// 两条都留: the phone keeps both, which the desktop's line takes.
	if _, err := phoneAsk(t, p, func(ctx context.Context) (psync.StatePutResult, error) {
		return c.ResolveConflict(ctx, lib, conflict.ID, psync.Value("phone again\nfrom the desk"))
	}); err != nil {
		t.Fatal(err)
	}
	if i := a.saved.book.Index(lineID); a.saved.book.Lines[i].Note != "phone again\nfrom the desk" {
		t.Errorf("note once resolved %q", a.saved.book.Lines[i].Note)
	}

	// The phone removes its line; the desktop's study record removes it for good.
	gone := a.phoneStateNow()
	if _, err := phoneAsk(t, p, func(ctx context.Context) (psync.StatePutResult, error) {
		return c.PutState(ctx, lib, []psync.Record{{Kind: psync.KindLine, ID: "phoneline", Removed: &psync.Stamp{At: gone}}})
	}); err != nil {
		t.Fatal(err)
	}
	if a.saved.book.Index("phoneline") >= 0 || !slices.ContainsFunc(a.saved.book.Removed, func(r study.Removed) bool { return r.ID == "phoneline" }) {
		t.Errorf("the removed line %+v, removed %+v", a.saved.book.Lines, a.saved.book.Removed)
	}

	// A record of a film the library does not have is refused, and nothing of it kept.
	_, err = phoneAsk(t, p, func(ctx context.Context) (psync.StatePutResult, error) {
		return c.PutState(ctx, lib, []psync.Record{{Kind: psync.KindPosition, ID: "nope", Film: "nope", Fields: map[string]psync.Field{psync.FieldPositionMS: field(1)}}})
	})
	phoneWantCode(t, err, http.StatusBadRequest, psync.CodeInvalid)
}

// A locked library's learning state is neither read nor written, and no ledger is left for it.
func TestPhoneSyncNeverSyncsALockedLibrarysState(t *testing.T) {
	a, p, dir, talk, c, _ := phoneStateLibrary(t)
	lib := phoneLibraryID(dir)
	l, err := shelf.NewLock("pw")
	if err != nil {
		t.Fatal(err)
	}
	a.library.SetLock(dir, l)
	refresh(p.tt, a)
	_, err = phoneAsk(t, p, func(ctx context.Context) (psync.StateChanges, error) { return c.State(ctx, lib, "", 0) })
	phoneWantCode(t, err, http.StatusNotFound, psync.CodeNotFound)
	_, err = phoneAsk(t, p, func(ctx context.Context) (psync.StatePutResult, error) {
		return c.PutState(ctx, lib, []psync.Record{{Kind: psync.KindPosition, ID: "yt", Film: "yt", Fields: map[string]psync.Field{psync.FieldPositionMS: {Value: psync.Value(5000), Stamp: psync.Stamp{At: time.Now()}}}}})
	})
	phoneWantCode(t, err, http.StatusNotFound, psync.CodeNotFound)
	if a.player.positions[talk] != 90*time.Second {
		t.Errorf("a locked library's position moved to %v", a.player.positions[talk])
	}
	if _, err := os.Stat(filepath.Join(dir, phonesync.LedgerFile)); !os.IsNotExist(err) {
		t.Errorf("a ledger was kept for a locked library: %v", err)
	}
}
