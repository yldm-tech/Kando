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

// callerStudy notes the callers it answers.
type callerStudy struct {
	fakeStudy
	callers *[]Caller
}

func (f callerStudy) LookUp(ctx context.Context, caller Caller, req WordLookup) (WordEntry, error) {
	*f.callers = append(*f.callers, caller)
	return f.fakeStudy.LookUp(ctx, caller, req)
}

func (f callerStudy) Explain(ctx context.Context, caller Caller, req LineExplain) (Explanation, error) {
	*f.callers = append(*f.callers, caller)
	return f.fakeStudy.Explain(ctx, caller, req)
}

// Lookups and explanations are reads: a read-only device has them answered, on the local network and over the relay alike, and the study service is told it is read-only, so it keeps nothing the device looked up; what changes something is still refused.
func TestAReadOnlyDeviceLooksUpAndHasLinesExplained(t *testing.T) {
	if Mutates(OpLookUp) || Mutates(OpExplain) {
		t.Fatal("lookups and explanations are taken to change something")
	}
	var callers []Caller
	d := &Desk{Service: &fakeService{}, Study: callerStudy{callers: &callers}, Journal: NewJournal()}
	lookUp, _ := json.Marshal(WordLookup{Word: "river", Line: "Every river begins."})
	explain, _ := json.Marshal(LineExplain{Lines: []Cue{{Original: "This one."}}})
	for _, relay := range []bool{false, true} {
		caller := Caller{DeviceID: "phone", ReadOnly: true, Relay: relay}
		for _, call := range []Call{{ID: NewCallID(), V: CallVersion, Op: OpLookUp, Args: lookUp}, {ID: NewCallID(), V: CallVersion, Op: OpExplain, Args: explain}} {
			if reply := d.Answer(context.Background(), caller, call); reply.Error != nil {
				t.Fatalf("%s by a read-only device (relay %v): %+v", call.Op, relay, reply.Error)
			}
		}
		create, _ := json.Marshal(JobCreate{Link: "https://example.com/v"})
		if reply := d.Answer(context.Background(), caller, Call{ID: NewCallID(), V: CallVersion, Op: OpJobCreate, Args: create}); reply.Error == nil || reply.Error.Code != CodeReadOnly {
			t.Fatalf("a link from a read-only device (relay %v): %+v", relay, reply)
		}
	}
	if len(callers) != 4 {
		t.Fatalf("the study service answered %d calls, want 4", len(callers))
	}
	for _, c := range callers {
		if !c.ReadOnly {
			t.Fatalf("the study service was not told the device is read-only: %+v", c)
		}
	}
}
