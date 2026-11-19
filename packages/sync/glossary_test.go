package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// fakeGlossary counts the terms put.
type fakeGlossary struct{ puts int }

func (f *fakeGlossary) Terms(_ context.Context, _ Caller, req FilmRef) (Glossary, error) {
	return Glossary{Library: req.Library, Film: req.Film, Terms: []Term{{Source: "meander", Target: "河曲", Scope: TermFilm}}}, nil
}

func (f *fakeGlossary) PutTerm(_ context.Context, _ Caller, req TermPut) (Glossary, error) {
	f.puts++
	return Glossary{Library: req.Library, Film: req.Film, Terms: []Term{req.Term}}, nil
}

// A desktop that does not share its glossary answers its calls unsupported; a read-only device reads it, and is refused a term before the desktop's work.
func TestGlossaryUnsupportedAndReadOnly(t *testing.T) {
	ctx := context.Background()
	desk := newDesk(&fakeService{receive: ReceiveQueue})
	_, err := v2Desktop(t, desk).Terms(ctx, "lib", "f1")
	wantError(t, err, http.StatusNotImplemented, CodeUnsupported)

	f := &fakeGlossary{}
	desk.Glossary = f
	if !Mutates(OpTermPut) || Mutates(OpTerms) {
		t.Fatal("putting a term must count as a change, and listing the glossary must not")
	}
	reader := Caller{DeviceID: "d1", Name: "Pixel", ReadOnly: true}
	data, _ := json.Marshal(TermPut{Library: "lib", Film: "f1", Term: Term{Source: "delta", Target: "三角洲"}})
	if r := desk.Answer(ctx, reader, Call{ID: "w", V: CallVersion, Op: OpTermPut, Args: data}); r.Error == nil || r.Error.Code != CodeReadOnly || f.puts != 0 {
		t.Fatalf("a read-only device's term answered %s %+v, %d puts", r.Result, r.Error, f.puts)
	}
	data, _ = json.Marshal(FilmRef{Library: "lib", Film: "f1"})
	r := desk.Answer(ctx, reader, Call{ID: "r", V: CallVersion, Op: OpTerms, Args: data})
	var g Glossary
	if r.Error != nil || json.Unmarshal(r.Result, &g) != nil || len(g.Terms) != 1 {
		t.Fatalf("a read-only device's glossary: %s %+v", r.Result, r.Error)
	}
	g, err = v2Desktop(t, desk).PutTerm(ctx, TermPut{Library: "lib", Film: "f1", Term: Term{Source: "delta", Target: "三角洲"}})
	if err != nil || f.puts != 1 || len(g.Terms) != 1 || g.Terms[0].Source != "delta" {
		t.Fatalf("a term put answered %+v, %v; %d puts", g, err, f.puts)
	}
}
