package sync

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
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

func (f fakeStudy) Ask(_ context.Context, _ Caller, req ExplainAsk) (ExplainAnswer, error) {
	if f.noAI {
		return ExplainAnswer{}, Errorf(CodeNoAI, "no AI service")
	}
	return ExplainAnswer{Text: req.Question + " of " + req.Lines[req.Index].Original}, nil
}

func (f fakeStudy) Recap(_ context.Context, _ Caller, req FilmRecap) (Recap, error) {
	if f.noAI {
		return Recap{}, Errorf(CodeNoAI, "no AI service")
	}
	return Recap{Items: []RecapItem{{StartMS: req.Lines[0].StartMS, Text: "began with " + req.Lines[0].Original}}}, nil
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
	reply = studyCall(t, d, OpExplainAsk, ExplainAsk{LineExplain: LineExplain{Lines: []Cue{{Original: "This one."}}}, Question: "why"})
	var a ExplainAnswer
	if reply.Error != nil || json.Unmarshal(reply.Result, &a) != nil || a.Text != "why of This one." {
		t.Fatalf("ask: %+v", reply)
	}
	reply = studyCall(t, d, OpRecap, FilmRecap{Lines: []Cue{{StartMS: 1000, Original: "Once."}}})
	var r Recap
	if reply.Error != nil || json.Unmarshal(reply.Result, &r) != nil || len(r.Items) != 1 || r.Items[0].StartMS != 1000 {
		t.Fatalf("recap: %+v", reply)
	}
}

// An explanation's sections travel with it, and a follow-up question carries the line as LineExplain does, flat in the same object.
func TestExplanationSectionsAndFollowUpsTravelAsJSON(t *testing.T) {
	data, err := json.Marshal(ExplainAsk{LineExplain: LineExplain{Film: "f1", Lines: []Cue{{Original: "a"}}}, Question: "why"})
	if err != nil || !strings.Contains(string(data), `"film":"f1"`) || !strings.Contains(string(data), `"question":"why"`) {
		t.Fatalf("a follow-up reads %s, %v", data, err)
	}
	var x Explanation
	if err := json.Unmarshal([]byte(`{"text":"t","model":"m"}`), &x); err != nil || x.Sections != nil || x.Local {
		t.Fatalf("an explanation of an older desktop reads %+v, %v", x, err)
	}
}

// RecapLines keeps the latest lines watched that fit in a call, and every line of a short film.
func TestRecapLinesKeepTheLatestThatFit(t *testing.T) {
	short := []Cue{{StartMS: 0, Original: "a"}, {StartMS: 1000, Original: "b"}}
	if got := RecapLines(short); len(got) != 2 {
		t.Fatalf("a short film keeps %d lines", len(got))
	}
	long := make([]Cue, 2000)
	for i := range long {
		long[i] = Cue{StartMS: int64(i) * 1000, Original: strings.Repeat("word ", 10), Translation: strings.Repeat("词", 10)}
	}
	got := RecapLines(long)
	if len(got) == 0 || len(got) == len(long) || got[len(got)-1].StartMS != long[len(long)-1].StartMS {
		t.Fatalf("a long film keeps %d lines ending at %d", len(got), got[len(got)-1].StartMS)
	}
	data, _ := json.Marshal(Call{ID: NewCallID(), V: CallVersion, Op: OpRecap, Args: mustJSON(t, FilmRecap{Lines: got})})
	if len(data) > MaxCall {
		t.Fatalf("the recap's call takes %d bytes, more than %d", len(data), MaxCall)
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
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
		{OpExplainAsk, ExplainAsk{LineExplain: LineExplain{Lines: []Cue{{Original: "a"}}}, Question: " "}},
		{OpExplainAsk, ExplainAsk{LineExplain: LineExplain{}, Question: "why"}},
		{OpExplainAsk, ExplainAsk{LineExplain: LineExplain{Lines: []Cue{{Original: "a"}}}, Question: "why", Asked: make([]ExplainQA, MaxExplainAsked+1)}},
		{OpRecap, FilmRecap{}},
		{OpRecap, FilmRecap{Lines: []Cue{{StartMS: 1000}}}},
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
	if Mutates(OpLookUp) || Mutates(OpExplain) || Mutates(OpExplainAsk) || Mutates(OpRecap) {
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

// A word's source is an addition to v2: a desktop that does not say leaves it out of the JSON, so an older phone reads the same entry as before; an older desktop's entry, without it, reads as the zero source; and a phone that does not know Source, as a phone before it, reads a newer desktop's entry with the fields it knows.
func TestWordSourceIsAnAddition(t *testing.T) {
	data, err := json.Marshal(WordEntry{Word: "river", Meaning: "河"})
	if err != nil || strings.Contains(string(data), "source") {
		t.Fatalf("an entry without a source reads %s, %v", data, err)
	}
	var e WordEntry
	if err := json.Unmarshal([]byte(`{"word":"river","reading":"","meaning":"河","language":"简体中文"}`), &e); err != nil || e.Source != (WordSource{}) || e.Meaning != "河" {
		t.Fatalf("an older desktop's entry reads %+v, %v", e, err)
	}
	data, err = json.Marshal(WordEntry{Word: "川", Meaning: "river", Source: WordSource{Kind: SourceDictionary, Name: "JMdict (English)", Credit: "JMdict © EDRDG", Desktop: "Studio"}})
	if err != nil || !strings.Contains(string(data), `"source":{"kind":"dictionary","name":"JMdict (English)","credit":"JMdict © EDRDG","desktop":"Studio"}`) {
		t.Fatalf("a dictionary's entry reads %s, %v", data, err)
	}
	var older struct {
		Word    string `json:"word"`
		Meaning string `json:"meaning"`
	}
	if err := json.Unmarshal(data, &older); err != nil || older.Word != "川" || older.Meaning != "river" {
		t.Fatalf("an older phone reads %+v, %v", older, err)
	}
	if err := json.Unmarshal(data, &e); err != nil || e.Source.Kind != SourceDictionary || e.Source.Credit != "JMdict © EDRDG" || e.Source.Desktop != "Studio" {
		t.Fatalf("a phone reads %+v, %v", e, err)
	}
	// A desktop before Desktop sends a source without it, which reads as a source of no desktop's name.
	var unnamed WordEntry
	if err := json.Unmarshal([]byte(`{"word":"river","meaning":"河","source":{"kind":"ai"}}`), &unnamed); err != nil || unnamed.Source != (WordSource{Kind: SourceAI}) {
		t.Fatalf("a source without the desktop's name reads %+v, %v", unnamed, err)
	}
}

// TestWordSourceOfAPlugin checks that a plugin's lookup source goes to a phone with its name and its plugin's, as an optional kind an older phone says nothing of.
func TestWordSourceOfAPlugin(t *testing.T) {
	data, err := json.Marshal(WordEntry{Word: "river", Meaning: "河", Source: WordSource{Kind: SourcePlugin, Name: "River Words", Plugin: "Rivers", Desktop: "Studio"}})
	if err != nil {
		t.Fatal(err)
	}
	var e WordEntry
	if err := json.Unmarshal(data, &e); err != nil || e.Source != (WordSource{Kind: SourcePlugin, Name: "River Words", Plugin: "Rivers", Desktop: "Studio"}) {
		t.Fatalf("read back %+v, %v", e, err)
	}
}

// failingStudy fails every call with err, as a desktop handing on what its AI service answered.
type failingStudy struct{ err error }

func (f failingStudy) LookUp(context.Context, Caller, WordLookup) (WordEntry, error) {
	return WordEntry{}, f.err
}

func (f failingStudy) Explain(context.Context, Caller, LineExplain) (Explanation, error) {
	return Explanation{}, f.err
}

func (f failingStudy) Ask(context.Context, Caller, ExplainAsk) (ExplainAnswer, error) {
	return ExplainAnswer{}, f.err
}

func (f failingStudy) Recap(context.Context, Caller, FilmRecap) (Recap, error) {
	return Recap{}, f.err
}

// What an AI service answered never reaches a phone: an error a StudyService gives without a code answers CodeFailed with the protocol's own phrase, whatever the error says; one with a code answers as given.
func TestAStudyErrorWithoutACodeKeepsWhatTheServiceSaid(t *testing.T) {
	body := `translation API returned 402 Payment Required: {"error":"Top up at https://pay.example.com/buy, 10 credits for $4.99"}`
	d := &Desk{Service: &fakeService{}, Study: failingStudy{errors.New(body)}, Journal: NewJournal()}
	calls := []struct {
		op   string
		args any
	}{
		{OpLookUp, WordLookup{Word: "river"}},
		{OpExplain, LineExplain{Lines: []Cue{{Original: "a"}}}},
		{OpExplainAsk, ExplainAsk{LineExplain: LineExplain{Lines: []Cue{{Original: "a"}}}, Question: "why?"}},
		{OpRecap, FilmRecap{Lines: []Cue{{Original: "a"}}}},
	}
	for _, c := range calls {
		reply := studyCall(t, d, c.op, c.args)
		if reply.Error == nil || reply.Error.Code != CodeFailed || reply.Error.Message != studyFailed {
			t.Fatalf("%s answered %+v, want failed with %q", c.op, reply.Error, studyFailed)
		}
		// The call's id is random hex, which may hold 402; what the service said could leak anywhere else in the reply.
		reply.ID = ""
		data, _ := json.Marshal(reply)
		for _, leak := range []string{"https://", "$4.99", "Top up", "402"} {
			if strings.Contains(string(data), leak) {
				t.Fatalf("%s carries %q to the phone: %s", c.op, leak, data)
			}
		}
	}
	d.Study = failingStudy{Errorf(CodeNoAI, "the AI service set cannot answer now")}
	if reply := studyCall(t, d, OpExplain, LineExplain{Lines: []Cue{{Original: "a"}}}); reply.Error == nil || reply.Error.Code != CodeNoAI || reply.Error.Message != "the AI service set cannot answer now" {
		t.Fatalf("a coded error answered %+v", reply.Error)
	}
}
