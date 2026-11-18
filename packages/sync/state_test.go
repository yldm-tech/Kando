package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	gosync "sync"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)

func at(min int, device string) Stamp {
	return Stamp{At: t0.Add(time.Duration(min) * time.Minute), Device: device}
}

func field(v any, s Stamp) Field { return Field{Value: Value(v), Stamp: s} }

func str(t *testing.T, f Field) string {
	t.Helper()
	var s string
	if err := json.Unmarshal(f.Value, &s); err != nil {
		t.Fatalf("value %s is not a string: %v", f.Value, err)
	}
	return s
}

func TestMergeRecordTakesTheLaterChangeOfEachField(t *testing.T) {
	have := Record{Kind: KindLine, ID: "l1", Film: "f1", Fields: map[string]Field{
		FieldNote:        field("desk note", at(5, "desk")),
		FieldTranslation: field("old", at(1, "desk")),
		FieldReview:      field(Review{Box: 1}, at(1, "desk")),
	}}
	in := Record{Kind: KindLine, ID: "l1", Fields: map[string]Field{
		FieldNote:        {Value: Value("phone note"), Stamp: at(3, "phone"), Base: &Stamp{At: at(5, "desk").At, Device: "desk"}},
		FieldTranslation: {Value: Value("new"), Stamp: at(4, "phone"), Base: &Stamp{At: at(1, "desk").At, Device: "desk"}},
		FieldReview:      field(Review{Box: 2}, at(4, "phone")),
	}}
	out, changed, clashes := MergeRecord(have, in)
	if !changed {
		t.Fatal("merge reported no change")
	}
	if got := str(t, out.Fields[FieldNote]); got != "desk note" {
		t.Errorf("note = %q, want the later desk note", got)
	}
	if got := str(t, out.Fields[FieldTranslation]); got != "new" {
		t.Errorf("translation = %q, want the phone's later one", got)
	}
	var r Review
	_ = json.Unmarshal(out.Fields[FieldReview].Value, &r)
	if r.Box != 2 {
		t.Errorf("review box = %d, want 2", r.Box)
	}
	if out.Film != "f1" {
		t.Errorf("film = %q, want it kept", out.Film)
	}
	if len(clashes) != 0 {
		t.Errorf("clashes = %+v, want none: the phone changed what it had seen", clashes)
	}
	if _, ok := out.Fields[FieldNote]; ok && out.Fields[FieldNote].Base != nil {
		t.Error("a merged field keeps the device's base")
	}
	again, changed, _ := MergeRecord(out, in)
	if changed || !sameRecord(again, out) {
		t.Errorf("merging again changed the record: %+v", again)
	}
}

func sameRecord(a, b Record) bool {
	da, _ := json.Marshal(a)
	db, _ := json.Marshal(b)
	return string(da) == string(db)
}

func TestMergeRecordTiesGoToTheDeviceThatSortsLast(t *testing.T) {
	have := Record{Kind: KindPosition, ID: "f1", Fields: map[string]Field{FieldPositionMS: field(1000, at(2, "a"))}}
	in := Record{Kind: KindPosition, ID: "f1", Fields: map[string]Field{FieldPositionMS: field(5000, at(2, "b"))}}
	out, _, _ := MergeRecord(have, in)
	if got := string(out.Fields[FieldPositionMS].Value); got != "5000" {
		t.Errorf("position = %s, want b's", got)
	}
	back, _, _ := MergeRecord(in, have)
	if got := string(back.Fields[FieldPositionMS].Value); got != "5000" {
		t.Errorf("position merged the other way = %s, want b's", got)
	}
}

func TestMergeRecordRecordsAConflictOfEditsMadeApart(t *testing.T) {
	seen := at(1, "desk")
	have := Record{Kind: KindLine, ID: "l1", Fields: map[string]Field{FieldNote: field("on the desk", at(5, "ipad"))}}
	in := Record{Kind: KindLine, ID: "l1", Fields: map[string]Field{FieldNote: {Value: Value("on the phone"), Stamp: at(7, "phone"), Base: &seen}}}
	out, _, clashes := MergeRecord(have, in)
	if got := str(t, out.Fields[FieldNote]); got != "on the phone" {
		t.Errorf("note = %q, want the later phone note", got)
	}
	if len(clashes) != 1 {
		t.Fatalf("clashes = %+v, want one", clashes)
	}
	if c := clashes[0]; str(t, c.Kept) != "on the phone" || str(t, c.Lost) != "on the desk" || c.Field != FieldNote {
		t.Errorf("clash = %+v", c)
	}

	// A field a learner does not write merges silently.
	pos := Record{Kind: KindPosition, ID: "f1", Fields: map[string]Field{FieldPositionMS: field(1, at(5, "ipad"))}}
	_, _, clashes = MergeRecord(pos, Record{Kind: KindPosition, ID: "f1", Fields: map[string]Field{FieldPositionMS: {Value: Value(2), Stamp: at(7, "phone"), Base: &seen}}})
	if len(clashes) != 0 {
		t.Errorf("a position clashed: %+v", clashes)
	}

	// A value with nothing in it loses nothing.
	_, _, clashes = MergeRecord(Record{Kind: KindLine, ID: "l1", Fields: map[string]Field{FieldNote: field("", at(5, "ipad"))}}, in)
	if len(clashes) != 0 {
		t.Errorf("an empty note clashed: %+v", clashes)
	}

	// The device's own earlier change is not another device's.
	_, _, clashes = MergeRecord(Record{Kind: KindLine, ID: "l1", Fields: map[string]Field{FieldNote: field("phone, before", at(5, "phone"))}}, in)
	if len(clashes) != 0 {
		t.Errorf("the phone's own change clashed: %+v", clashes)
	}

	// The earlier of two changes made apart loses but is not lost either.
	late := Record{Kind: KindLine, ID: "l1", Fields: map[string]Field{FieldNote: {Value: Value("late phone"), Stamp: at(3, "phone"), Base: &seen}}}
	out, changed, clashes := MergeRecord(have, late)
	if changed || str(t, out.Fields[FieldNote]) != "on the desk" {
		t.Errorf("an earlier change won: %+v", out)
	}
	if len(clashes) != 1 || str(t, clashes[0].Lost) != "late phone" {
		t.Errorf("clashes = %+v, want the phone's note set aside", clashes)
	}
}

func TestATombstoneWinsOverEveryChange(t *testing.T) {
	gone := Record{Kind: KindLine, ID: "l1", Removed: &Stamp{At: at(1, "desk").At, Device: "desk"}}
	edit := Record{Kind: KindLine, ID: "l1", Fields: map[string]Field{FieldNote: field("later", at(9, "phone"))}}
	out, changed, _ := MergeRecord(gone, edit)
	if changed || out.Removed == nil || out.Fields != nil {
		t.Errorf("a later edit brought a removed line back: %+v", out)
	}
	out, changed, _ = MergeRecord(Record{Kind: KindLine, ID: "l1", Fields: edit.Fields}, gone)
	if !changed || out.Removed == nil || len(out.Fields) != 0 {
		t.Errorf("a removal did not take the line out: %+v", out)
	}
	// A record removed on a device before the desktop had it is kept as a tombstone, so it does not come back.
	out, changed, _ = MergeRecord(Record{}, gone)
	if !changed || out.Removed == nil {
		t.Errorf("a removal of a new record = %+v", out)
	}
}

func TestLedgerObserveStampsTheDesktopsChanges(t *testing.T) {
	l := NewLedger()
	all := func(Record) bool { return true }
	saved := at(1, "desk")
	current := []Record{
		{Kind: KindPosition, ID: "f1", Fields: map[string]Field{FieldPositionMS: {Value: Value(1000)}}},
		{Kind: KindLine, ID: "l1", Film: "f1", Fields: map[string]Field{FieldNote: field("n", saved)}},
	}
	if !l.Observe(current, all, at(2, "desk")) {
		t.Fatal("the first observation changed nothing")
	}
	if p, _ := l.Record(KindPosition, "f1"); !p.Fields[FieldPositionMS].Stamp.Equal(at(2, "desk")) {
		t.Errorf("an unstamped position = %+v, want it stamped now", p)
	}
	if n, _ := l.Record(KindLine, "l1"); !n.Fields[FieldNote].Stamp.Equal(saved) {
		t.Errorf("a stamped note = %+v, want its own stamp", n)
	}
	rev := l.Rev()
	if l.Observe(current, all, at(3, "desk")) || l.Rev() != rev {
		t.Error("observing the same state again changed the ledger")
	}

	current[0].Fields[FieldPositionMS] = Field{Value: Value(4000)}
	if !l.Observe(current, all, at(4, "desk")) {
		t.Fatal("a moved position changed nothing")
	}
	if p, _ := l.Record(KindPosition, "f1"); !p.Fields[FieldPositionMS].Stamp.Equal(at(4, "desk")) || p.Rev != l.Rev() {
		t.Errorf("a moved position = %+v at rev %d", p, l.Rev())
	}

	// What the desktop no longer has, and read, is gone: a line by its tombstone, a position set to null; what it could not read stays.
	l.Observe([]Record{{Kind: KindPosition, ID: "f2", Film: "f2", Fields: map[string]Field{FieldPositionMS: {Value: Value(1)}}}}, func(r Record) bool { return r.ID == "f2" }, at(5, "desk"))
	l.Observe(nil, func(r Record) bool { return r.ID != "f2" }, at(6, "desk"))
	if line, _ := l.Record(KindLine, "l1"); line.Removed == nil || !line.Removed.Equal(at(6, "desk")) {
		t.Errorf("a line the desktop lost = %+v, want a tombstone", line)
	}
	if p, _ := l.Record(KindPosition, "f1"); !IsNull(p.Fields[FieldPositionMS].Value) {
		t.Errorf("a position the desktop lost = %+v, want null", p)
	}
	if p, _ := l.Record(KindPosition, "f2"); IsNull(p.Fields[FieldPositionMS].Value) {
		t.Errorf("a position the desktop did not read was taken away: %+v", p)
	}
}

func TestLedgerSinceAnswersTheChangesOrAllOfThem(t *testing.T) {
	l := NewLedger()
	all := func(Record) bool { return true }
	l.Observe([]Record{{Kind: KindPosition, ID: "f1", Fields: map[string]Field{FieldPositionMS: {Value: Value(1)}}}}, all, at(1, "desk"))
	first := l.Since("lib", "", 0)
	if !first.Reset || len(first.Records) != 1 || first.Epoch != l.Epoch() || first.Library != "lib" {
		t.Fatalf("first answer = %+v", first)
	}
	if next := l.Since("lib", first.Epoch, first.Rev); next.Reset || len(next.Records) != 0 {
		t.Errorf("no change answered %+v", next)
	}
	l.Observe([]Record{
		{Kind: KindPosition, ID: "f1", Fields: map[string]Field{FieldPositionMS: {Value: Value(1)}}},
		{Kind: KindPosition, ID: "f2", Fields: map[string]Field{FieldPositionMS: {Value: Value(2)}}},
	}, all, at(2, "desk"))
	next := l.Since("lib", first.Epoch, first.Rev)
	if next.Reset || len(next.Records) != 1 || next.Records[0].ID != "f2" {
		t.Errorf("changes = %+v, want f2 alone", next)
	}
	if past := l.Since("lib", first.Epoch, l.Rev()+5); !past.Reset || len(past.Records) != 2 {
		t.Errorf("a revision the ledger never reached = %+v, want everything again", past)
	}
}

func TestLedgerPutRecordsConflictsAndResolveSettlesThem(t *testing.T) {
	l := NewLedger()
	all := func(Record) bool { return true }
	l.Observe([]Record{{Kind: KindLine, ID: "l1", Film: "f1", Fields: map[string]Field{FieldNote: field("base", at(1, "desk"))}}}, all, at(1, "desk"))
	ids := 0
	newID := func() string { ids++; return fmt.Sprintf("c%d", ids) }
	base := at(1, "desk")
	changed, conflicts := l.Put([]Record{{Kind: KindLine, ID: "l1", Fields: map[string]Field{FieldNote: {Value: Value("phone"), Stamp: at(3, "phone"), Base: &base}}}}, newID)
	if len(changed) != 1 || len(conflicts) != 0 {
		t.Fatalf("a change from what the desktop had = %+v, %+v", changed, conflicts)
	}
	// The iPad changed the note it had before the phone's change reached it.
	changed, conflicts = l.Put([]Record{{Kind: KindLine, ID: "l1", Fields: map[string]Field{FieldNote: {Value: Value("ipad"), Stamp: at(4, "ipad"), Base: &base}}}}, newID)
	if len(changed) != 1 || len(conflicts) != 1 {
		t.Fatalf("an edit made apart = %+v, %+v", changed, conflicts)
	}
	c := conflicts[0]
	if str(t, c.Kept) != "ipad" || str(t, c.Lost) != "phone" || c.Film != "f1" || c.Record != "l1" {
		t.Errorf("conflict = %+v", c)
	}
	if open := l.Since("lib", l.Epoch(), 0).Conflicts; len(open) != 1 {
		t.Errorf("open conflicts = %+v", open)
	}
	before := l.Rev()

	// 两条都留: the learner keeps both.
	rec, closed, err := l.Resolve("c1", Value("ipad\nphone"), at(4, "desk"))
	if err != nil {
		t.Fatal(err)
	}
	if str(t, rec.Fields[FieldNote]) != "ipad\nphone" || closed.Resolved == nil {
		t.Errorf("resolved = %+v, %+v", rec, closed)
	}
	if !rec.Fields[FieldNote].Stamp.After(c.Kept.Stamp) {
		t.Errorf("the resolution %+v is not later than the value it replaced %+v", rec.Fields[FieldNote].Stamp, c.Kept.Stamp)
	}
	if len(l.Conflicts()) != 0 {
		t.Errorf("conflicts still open: %+v", l.Conflicts())
	}
	news := l.Since("lib", l.Epoch(), before)
	if len(news.Records) != 1 || len(news.Conflicts) != 1 || news.Conflicts[0].Resolved == nil {
		t.Errorf("the resolution was not told: %+v", news)
	}
	if _, _, err := l.Resolve("c1", Value("x"), at(5, "desk")); !errors.Is(err, ErrNoConflict) {
		t.Errorf("resolving again = %v, want ErrNoConflict", err)
	}
	if reset := l.Since("lib", "", 0); len(reset.Conflicts) != 0 {
		t.Errorf("a reset told resolved conflicts: %+v", reset.Conflicts)
	}
}

func TestLedgerIsKeptAndReadBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	l, err := ReadLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	l.Observe([]Record{{Kind: KindLine, ID: "l1", Film: "f1", Fields: map[string]Field{FieldNote: field("n", at(1, "desk"))}}}, func(Record) bool { return true }, at(1, "desk"))
	base := Stamp{}
	l.Put([]Record{{Kind: KindLine, ID: "l1", Fields: map[string]Field{FieldNote: {Value: Value("p"), Stamp: at(2, "phone"), Base: &base}}}}, func() string { return "c1" })
	if err := WriteLedger(path, l); err != nil {
		t.Fatal(err)
	}
	back, err := ReadLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	if back.Epoch() != l.Epoch() || back.Rev() != l.Rev() || len(back.Conflicts()) != 1 {
		t.Errorf("read back epoch %q rev %d conflicts %d, want %q %d 1", back.Epoch(), back.Rev(), len(back.Conflicts()), l.Epoch(), l.Rev())
	}
	if r, ok := back.Record(KindLine, "l1"); !ok || str(t, r.Fields[FieldNote]) != "p" {
		t.Errorf("read back %+v", r)
	}
}

func TestCorrectionIDsReadBack(t *testing.T) {
	id := CorrectionID("f1", 1200, 3400)
	film, start, end, ok := ParseCorrectionID(id)
	if !ok || film != "f1" || start != 1200 || end != 3400 {
		t.Errorf("ParseCorrectionID(%q) = %q %d %d %v", id, film, start, end, ok)
	}
	for _, bad := range []string{"", "f1", "f1/12", "/1-2", "f1/3-2", "f1/a-b"} {
		if _, _, _, ok := ParseCorrectionID(bad); ok {
			t.Errorf("ParseCorrectionID(%q) read", bad)
		}
	}
}

// ledgerState is a desktop's StateService that keeps one library's state in a ledger, as the desktop does once it has read its own.
type ledgerState struct {
	mu     gosync.Mutex
	ledger *Ledger
	puts   int
	ids    int
}

func (s *ledgerState) GetState(_ context.Context, _ Caller, req StateGet) (StateChanges, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if req.Library != "lib" {
		return StateChanges{}, Errorf(CodeNotFound, "no library %q", req.Library)
	}
	return s.ledger.Since(req.Library, req.Epoch, req.Since), nil
}

func (s *ledgerState) PutState(_ context.Context, _ Caller, req StatePut) (StatePutResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.puts++
	changed, conflicts := s.ledger.Put(req.Records, func() string { s.ids++; return fmt.Sprintf("c%d", s.ids) })
	return StatePutResult{Epoch: s.ledger.Epoch(), Rev: s.ledger.Rev(), Records: changed, Conflicts: conflicts}, nil
}

func (s *ledgerState) ResolveConflict(_ context.Context, caller Caller, req ConflictResolve) (StatePutResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, c, err := s.ledger.Resolve(req.ID, req.Value, Stamp{At: time.Now(), Device: caller.DeviceID})
	if err != nil {
		return StatePutResult{}, Errorf(CodeNotFound, "%v", err)
	}
	return StatePutResult{Epoch: s.ledger.Epoch(), Rev: s.ledger.Rev(), Records: []Record{rec}, Conflicts: []Conflict{c}}, nil
}

func TestTheLearningStateOverV2(t *testing.T) {
	ctx := context.Background()
	state := &ledgerState{ledger: NewLedger()}
	state.ledger.Observe([]Record{{Kind: KindLine, ID: "l1", Film: "f1", Fields: map[string]Field{FieldNote: field("desk", at(1, "desk"))}}}, func(Record) bool { return true }, at(1, "desk"))
	now := t0.Add(time.Hour)
	desk := &Desk{Service: &fakeService{}, State: state, Journal: NewJournal(), Desktop: "Mac", Now: func() time.Time { return now }}
	c := v2Desktop(t, desk)

	got, err := c.State(ctx, "lib", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Reset || len(got.Records) != 1 {
		t.Fatalf("first state = %+v", got)
	}
	if _, err := c.State(ctx, "locked", "", 0); !isCode(err, StatusOf(CodeNotFound), CodeNotFound) {
		t.Errorf("a library the desktop does not serve = %v", err)
	}

	// The phone's change is stamped with the phone's ID and no later than the desktop's clock.
	base := got.Records[0].Fields[FieldNote].Stamp
	put := []Record{{Kind: KindLine, ID: "l1", Fields: map[string]Field{FieldNote: {Value: Value("phone"), Stamp: Stamp{At: now.Add(48 * time.Hour), Device: "desk"}, Base: &base}}}}
	res, err := c.PutState(ctx, "lib", put)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Records) != 1 {
		t.Fatalf("put = %+v", res)
	}
	if f := res.Records[0].Fields[FieldNote]; f.Device != "d1" || !f.At.Equal(now) {
		t.Errorf("the put was stamped %+v, want d1 at the desktop's now", f.Stamp)
	}

	// The same call sent twice does its work once.
	id := NewCallID()
	for range 2 {
		if err := c.CallID(ctx, id, OpStatePut, StatePut{Library: "lib", Records: put}, nil); err != nil {
			t.Fatal(err)
		}
	}
	if state.puts != 2 {
		t.Errorf("puts = %d, want 2: the call sent again answered from memory", state.puts)
	}

	// A record removed is a tombstone; a position is not removed.
	if _, err := c.PutState(ctx, "lib", []Record{{Kind: KindPosition, ID: "f1", Removed: &Stamp{At: now}}}); !isCode(err, StatusOf(CodeInvalid), CodeInvalid) {
		t.Errorf("removing a position = %v, want invalid", err)
	}
	if _, err := c.PutState(ctx, "lib", []Record{{ID: "l2", Fields: map[string]Field{FieldNote: field("n", at(1, ""))}}}); !isCode(err, StatusOf(CodeInvalid), CodeInvalid) {
		t.Errorf("a record of no kind = %v, want invalid", err)
	}

	next, err := c.State(ctx, "lib", got.Epoch, got.Rev)
	if err != nil {
		t.Fatal(err)
	}
	if next.Reset || len(next.Records) != 1 || str(t, next.Records[0].Fields[FieldNote]) != "phone" {
		t.Errorf("changes since = %+v", next)
	}

	if _, err := c.ResolveConflict(ctx, "lib", "nope", Value("x")); !isCode(err, StatusOf(CodeNotFound), CodeNotFound) {
		t.Errorf("resolving an unknown conflict = %v", err)
	}
	if _, err := c.ResolveConflict(ctx, "lib", "", Value("x")); !isCode(err, StatusOf(CodeInvalid), CodeInvalid) {
		t.Errorf("a resolution of no conflict = %v", err)
	}

	// A desktop without the learning state answers its calls unsupported, and lists no kinds.
	bare := v2Desktop(t, &Desk{Service: &fakeService{}, Journal: NewJournal(), Desktop: "Mac"})
	if _, err := bare.State(ctx, "lib", "", 0); !isCode(err, StatusOf(CodeUnsupported), CodeUnsupported) {
		t.Errorf("state on a desktop without it = %v", err)
	}
	h, err := bare.Hello(ctx)
	if err != nil || h.State == nil || len(h.State) != 0 {
		t.Errorf("hello = %+v, %v; want no kinds, as an empty list", h, err)
	}
}

func TestAStateEventWakesAPhoneAndKeepsApartFromAJobOfTheSameID(t *testing.T) {
	j := NewJournal()
	j.PublishJobs([]Job{{ID: "same", State: JobWaiting}})
	list := j.List()
	done := make(chan Events)
	go func() { done <- j.Wait(context.Background(), list.Epoch, list.Rev, 5*time.Second) }()
	j.PublishState("same")
	got := <-done
	if len(got.Events) != 1 || got.Events[0].Kind != EventState || got.Events[0].ID != "same" {
		t.Fatalf("events = %+v, want the state of library same", got.Events)
	}
	j.PublishState("same")
	j.PublishJobs([]Job{{ID: "same", State: JobRunning}})
	j.PublishState("same")
	all := j.Wait(context.Background(), list.Epoch, list.Rev, 0)
	if len(all.Events) != 2 || all.Events[0].Kind != EventJob || all.Events[1].Kind != EventState {
		t.Errorf("events = %+v, want the job's newest and the state's newest", all.Events)
	}
}
