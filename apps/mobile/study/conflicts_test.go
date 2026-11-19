package study

import (
	"testing"
	"time"

	"kando/packages/sync"
)

// conflicted is a library with a saved line whose note two devices changed apart, a word whose meaning they did, and a correction whose translation they did, as the computer answered them.
func conflicted() Library {
	stamp := func(at time.Duration, device string) sync.Stamp { return sync.Stamp{At: t0.Add(at), Device: device} }
	field := func(v any, s sync.Stamp) sync.Field { return sync.Field{Value: sync.Value(v), Stamp: s} }
	desk, phone := stamp(0, "desk"), stamp(time.Minute, "me")
	ch := sync.StateChanges{Epoch: "e", Rev: 9, Reset: true,
		Records: []sync.Record{
			{Kind: sync.KindLine, ID: "l1", Film: "f1", Rev: 3, Fields: map[string]sync.Field{
				sync.FieldStartMS: field(52_000, desk), sync.FieldOriginal: field("That is how valleys form.", desk),
				sync.FieldTitle: field("Rivers", desk), sync.FieldNote: field("mine", phone),
			}},
			{Kind: sync.KindWord, ID: "w1", Film: "f2", Rev: 4, Fields: map[string]sync.Field{
				sync.FieldWord: field("basis", desk), sync.FieldTitle: field("Algebra", desk), sync.FieldMeaning: field("axes", phone),
			}},
			{Kind: sync.KindCorrection, ID: sync.CorrectionID("f3", 7000, 9000), Film: "f3", Rev: 5, Fields: map[string]sync.Field{
				sync.FieldTranslation: field("再见", phone),
			}},
		},
		Conflicts: []sync.Conflict{
			{ID: "c2", Kind: sync.KindWord, Record: "w1", Film: "f2", Field: sync.FieldMeaning, Kept: field("axes", phone), Lost: field("directions", desk), Rev: 7},
			{ID: "c1", Kind: sync.KindLine, Record: "l1", Film: "f1", Field: sync.FieldNote, Kept: field("mine", phone), Lost: field("theirs", desk), Rev: 6},
			{ID: "c3", Kind: sync.KindCorrection, Record: sync.CorrectionID("f3", 7000, 9000), Film: "f3", Field: sync.FieldTranslation, Kept: field("再见", phone), Lost: field("回头见", stamp(0, "tablet")), Rev: 8},
		},
		Devices: map[string]string{"desk": "MacBook Pro", "me": "Test Phone"},
	}
	return Apply(Library{}, ch)
}

func TestConflictsSayWhatWasChangedWhereAndByWhom(t *testing.T) {
	got := Conflicts(conflicted(), "me")
	if len(got) != 3 || got[0].ID != "c1" || got[1].ID != "c2" || got[2].ID != "c3" {
		t.Fatalf("conflicts %+v", got)
	}
	line, word, corr := got[0], got[1], got[2]
	if line.Title != "Rivers" || line.At != 52*time.Second || line.Line != "That is how valleys form." || !line.Both || line.Word != "" {
		t.Errorf("the line's conflict %+v", line)
	}
	if !line.Kept.Mine || line.Kept.Text != "mine" || line.Lost.Mine || line.Lost.Device != "MacBook Pro" || line.Lost.Text != "theirs" || !line.Lost.At.Equal(t0) {
		t.Errorf("the line's versions %+v / %+v", line.Kept, line.Lost)
	}
	if word.Word != "basis" || word.At != -1 || word.Line != "" || !word.Both || word.Title != "Algebra" {
		t.Errorf("the word's conflict %+v", word)
	}
	// Two translations of one line are not kept together; a device the computer does not name has no name.
	if corr.Film != "f3" || corr.At != 7*time.Second || corr.Both || corr.Title != "" || corr.Lost.Device != "" || corr.Lost.Mine {
		t.Errorf("the correction's conflict %+v", corr)
	}
}

func TestAResolvedConflictGoesWithTheValueKept(t *testing.T) {
	l := conflicted()
	kept := sync.Record{Kind: sync.KindLine, ID: "l1", Film: "f1", Rev: 10, Fields: map[string]sync.Field{sync.FieldNote: {Value: sync.Value("mine\ntheirs"), Stamp: sync.Stamp{At: t0.Add(time.Hour), Device: "me"}}}}
	resolved := sync.Stamp{At: t0.Add(time.Hour), Device: "me"}
	l = Resolved(l, sync.StatePutResult{Epoch: "e", Rev: 10, Records: []sync.Record{kept}, Conflicts: []sync.Conflict{{ID: "c1", Resolved: &resolved}}})
	if got := Conflicts(l, "me"); len(got) != 2 || got[0].ID != "c2" {
		t.Fatalf("conflicts left %+v", got)
	}
	if r, ok := View(l, sync.KindLine, "l1"); !ok || sync.Text(r.Fields[sync.FieldNote].Value) != "mine\ntheirs" {
		t.Fatalf("the line %+v", r)
	}
	// One settled on another device meanwhile goes as well.
	if got := Conflicts(Forget(l, "c2"), "me"); len(got) != 1 || got[0].ID != "c3" {
		t.Fatalf("after forgetting: %+v", got)
	}
	// The resolution told again with the next changes closes nothing more, and keeps the devices named.
	l = Apply(l, sync.StateChanges{Epoch: "e", Rev: 11, Conflicts: []sync.Conflict{{ID: "c1", Resolved: &resolved}}, Devices: map[string]string{"tablet": "iPad"}})
	if len(Conflicts(l, "me")) != 2 || l.Devices["desk"] != "MacBook Pro" || l.Devices["tablet"] != "iPad" {
		t.Fatalf("after the changes: %+v", l)
	}
}

func TestAPositionIsAChangeOfTheFilmsRecord(t *testing.T) {
	r := Position("f1", 6500*time.Millisecond, t0)
	if r.Kind != sync.KindPosition || r.ID != "f1" || r.Film != "f1" || string(r.Fields[sync.FieldPositionMS].Value) != "6500" || !r.Fields[sync.FieldPositionMS].At.Equal(t0) {
		t.Fatalf("position %+v", r)
	}
	if v := Position("f1", 0, t0).Fields[sync.FieldPositionMS].Value; !sync.IsNull(v) {
		t.Fatalf("the beginning is %s, want null", v)
	}
	// Positions kept one after another go as one change, the last.
	l := Change(Change(Library{}, r), Position("f1", 8*time.Second, t0.Add(time.Second)))
	if len(l.Pending) != 1 || string(l.Pending[0].Fields[sync.FieldPositionMS].Value) != "8000" {
		t.Fatalf("pending %+v", l.Pending)
	}
}
