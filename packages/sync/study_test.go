package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// fakeStudy answers every lookup with the word in capitals and every explanation with the line explained, or CodeNoAI while noAI is set.
type fakeStudy struct{ noAI bool }

func (f fakeStudy) LookUp(_ context.Context, _ Caller, req WordLookup) (WordEntry, error) {
	if f.noAI {
		return WordEntry{}, Errorf(CodeNoAI, "no AI service")
	}
	return WordEntry{Word: req.Word, Meaning: "meaning of " + req.Word + " in " + req.Line}, nil
}

func (f fakeStudy) Explain(_ context.Context, _ Caller, req LineExplain) (Explanation, error) {
	if f.noAI {
		return Explanation{}, Errorf(CodeNoAI, "no AI service")
	}
	return Explanation{Text: "about " + req.Lines[req.Index].Original}, nil
}

func studyCall(t *testing.T, d *Desk, op string, args any) Reply {
	t.Helper()
	data, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return d.Answer(context.Background(), Caller{DeviceID: "phone"}, Call{ID: NewCallID(), V: CallVersion, Op: op, Args: data})
}

func TestLookupsAndExplanationsGoToTheStudyService(t *testing.T) {
	d := &Desk{Service: &fakeService{}, Study: fakeStudy{}, Journal: NewJournal()}
	reply := studyCall(t, d, OpLookUp, WordLookup{Word: "river", Line: "Every river begins."})
	var e WordEntry
	if reply.Error != nil || json.Unmarshal(reply.Result, &e) != nil || e.Meaning != "meaning of river in Every river begins." {
		t.Fatalf("lookup: %+v", reply)
	}
	reply = studyCall(t, d, OpExplain, LineExplain{Lines: []Cue{{Original: "Before."}, {Original: "This one."}}, Index: 1})
	var x Explanation
	if reply.Error != nil || json.Unmarshal(reply.Result, &x) != nil || x.Text != "about This one." {
		t.Fatalf("explain: %+v", reply)
	}
}

func TestLookupsAndExplanationsAreChecked(t *testing.T) {
	d := &Desk{Service: &fakeService{}, Study: fakeStudy{}, Journal: NewJournal()}
	bad := []struct {
		op   string
		args any
	}{
		{OpLookUp, WordLookup{Word: "  ", Line: "a line"}},
		{OpExplain, LineExplain{}},
		{OpExplain, LineExplain{Lines: []Cue{{Original: "a"}}, Index: 1}},
		{OpExplain, LineExplain{Lines: []Cue{{Original: "a"}, {}}, Index: 1}},
		{OpExplain, LineExplain{Lines: make([]Cue, MaxExplainLines+1), Index: 0}},
	}
	for _, b := range bad {
		if reply := studyCall(t, d, b.op, b.args); reply.Error == nil || reply.Error.Code != CodeInvalid {
			t.Fatalf("%s %+v answered %+v, want invalid", b.op, b.args, reply)
		}
	}
}

func TestADesktopWithoutLookupsOrAIServiceSaysSo(t *testing.T) {
	d := &Desk{Service: &fakeService{}, Journal: NewJournal()}
	if reply := studyCall(t, d, OpLookUp, WordLookup{Word: "river"}); reply.Error == nil || reply.Error.Code != CodeUnsupported {
		t.Fatalf("without a study service: %+v", reply)
	}
	d.Study = fakeStudy{noAI: true}
	reply := studyCall(t, d, OpExplain, LineExplain{Lines: []Cue{{Original: "a"}}})
	if reply.Error == nil || reply.Error.Code != CodeNoAI {
		t.Fatalf("without an AI service: %+v", reply)
	}
	if StatusOf(CodeNoAI) != http.StatusConflict {
		t.Fatalf("no_ai answers %d", StatusOf(CodeNoAI))
	}
}
