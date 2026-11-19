package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
)

// The marks are an addition to v2: a phone that knows them reads a desktop's answer, and drops the marks of a line whose original is not the one they were computed for.
func TestFilmMarksOfDropsStaleLines(t *testing.T) {
	originals := []string{"今日は雨", "Hello", "明日も雨"}
	m := FilmMarks{Lines: []LineMarks{
		{Line: 0, Hash: LineHash("今日は雨"), Marks: []Mark{{Start: 0, End: 6, Ruby: "きょう"}, {Start: 9, End: 12, Tone: ToneAccent}}},
		{Line: 2, Hash: LineHash("明日は雨"), Marks: []Mark{{Start: 0, End: 6, Ruby: "あした"}}},
		{Line: 7, Hash: LineHash("x"), Marks: []Mark{{Start: 0, End: 1, Underline: true}}},
		{Line: -1, Hash: LineHash("今日は雨"), Marks: []Mark{{Start: 0, End: 1}}},
	}}
	got := m.Of(originals)
	if len(got) != 3 || len(got[0]) != 2 || got[0][0].Ruby != "きょう" || got[0][1].Tone != ToneAccent {
		t.Fatalf("the marks of the first line read %+v", got)
	}
	if got[1] != nil || got[2] != nil {
		t.Fatalf("a line without marks, or one whose original changed, has %+v and %+v", got[1], got[2])
	}
	if LineHash("今日は雨") == LineHash("今日は雨 ") {
		t.Fatal("two originals hash alike")
	}
}

func TestFilmMarksIsAnAddition(t *testing.T) {
	data, err := json.Marshal(FilmMarks{Lines: []LineMarks{{Line: 1, Hash: "h", Marks: []Mark{{Start: 0, End: 3, Ruby: "かわ"}, {Start: 3, End: 6, Tone: ToneWarn, Underline: true}}}}})
	if err != nil || !strings.Contains(string(data), `{"line":1,"hash":"h","marks":[{"start":0,"end":3,"ruby":"かわ"},{"start":3,"end":6,"tone":"warn","underline":true}]}`) || strings.Contains(string(data), "pending") {
		t.Fatalf("the marks read %s, %v", data, err)
	}
	// A desktop from after them may add to a mark or a line, which a phone skips.
	var m FilmMarks
	if err := json.Unmarshal([]byte(`{"lines":[{"line":0,"hash":"h","marks":[{"start":0,"end":3,"ruby":"かわ","kind":"later"}],"later":1}],"later":true}`), &m); err != nil || len(m.Lines) != 1 || m.Lines[0].Marks[0].Ruby != "かわ" {
		t.Fatalf("a later desktop's marks read %+v, %v", m, err)
	}
	if Mutates(OpFilmMarks) {
		t.Fatal("asking for a film's marks counts as a change")
	}
}

// stubService answers hello alone, as a desktop with no other service.
type stubService struct{ Service }

func (stubService) Hello(context.Context, Caller) (Hello, error) { return Hello{}, nil }

type stubMarks struct{ got []FilmRef }

func (s *stubMarks) FilmMarks(_ context.Context, _ Caller, ref FilmRef) (FilmMarks, error) {
	s.got = append(s.got, ref)
	if ref.ID == "gone" {
		return FilmMarks{}, Errorf(CodeNotFound, "no film")
	}
	return FilmMarks{}, nil
}

// A desktop with marks answers them, to a read-only phone too, with an empty list for none; one from before them answers the call unsupported.
func TestDeskAnswersFilmMarks(t *testing.T) {
	ctx := context.Background()
	marks := &stubMarks{}
	d := &Desk{Service: stubService{}, Marks: marks, Journal: NewJournal()}
	reply := d.Answer(ctx, Caller{DeviceID: "p", ReadOnly: true}, Call{ID: "1", V: CallVersion, Op: OpFilmMarks, Args: json.RawMessage(`{"id":"f1"}`)})
	if reply.Error != nil || string(reply.Result) != `{"lines":[]}` || !slices.Equal(marks.got, []FilmRef{{ID: "f1"}}) {
		t.Fatalf("a read-only phone's call answered %s, %+v", reply.Result, reply.Error)
	}
	if reply := d.Answer(ctx, Caller{DeviceID: "p"}, Call{ID: "2", V: CallVersion, Op: OpFilmMarks, Args: json.RawMessage(`{}`)}); reply.Error == nil || reply.Error.Code != CodeInvalid {
		t.Fatalf("a call naming no film answered %+v", reply)
	}
	if reply := d.Answer(ctx, Caller{DeviceID: "p"}, Call{ID: "3", V: CallVersion, Op: OpFilmMarks, Args: json.RawMessage(`{"id":"gone"}`)}); reply.Error == nil || reply.Error.Code != CodeNotFound {
		t.Fatalf("a film not served answered %+v", reply)
	}

	older := &Desk{Service: stubService{}, Journal: NewJournal()}
	reply = older.Answer(ctx, Caller{DeviceID: "p"}, Call{ID: "4", V: CallVersion, Op: OpFilmMarks, Args: json.RawMessage(`{"id":"f1"}`)})
	if reply.Error == nil || reply.Error.Code != CodeUnsupported || StatusOf(reply.Error.Code) != http.StatusNotImplemented {
		t.Fatalf("a desktop from before the marks answered %+v", reply)
	}
}
