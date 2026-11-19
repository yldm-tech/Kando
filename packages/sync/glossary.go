package sync

import "context"

// The glossary of a film (yldm-tech/Kando#213): how the computer translates the names and terms of a film, the film's own terms, its series' and those of every film, as the computer's 术语 tab keeps them. A phone lists a film's glossary (OpTerms), which its player shows without changing it, and adds a term to the film's own while it corrects a line (OpTermPut), which the computer translates the film with from then on. The computer keeps the glossary: a phone neither moves a term to another scope nor removes one. A film of a locked library is not found. Like every v2 call they go over any transport, the storage relay too; OpTermPut changes the computer (Mutates), so a read-only device (Caller.ReadOnly) is refused it, and OpTerms only reads.
const (
	// OpTerms takes FilmRef and answers Glossary.
	OpTerms = "glossary.get"
	// OpTermPut takes TermPut and answers the Glossary as it is once changed.
	OpTermPut = "glossary.put"
)

// The scopes of a Term: the film's own, its series', every film's.
const (
	TermFilm   = "film"
	TermSeries = "series"
	TermGlobal = "global"
)

// Term is an entry of a glossary: Source as the film's lines write it, Target as the translation writes it, Scope which films it is for (TermFilm, TermSeries, TermGlobal; a phone shows a scope it does not know as the film's), and Locked for one the translation must write as it is.
type Term struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Scope  string `json:"scope"`
	Locked bool   `json:"locked,omitempty"`
}

// FilmRef names film Film of library Library.
type FilmRef struct {
	Library string `json:"library"`
	Film    string `json:"film"`
}

// Glossary is a film's glossary as the computer translates it: its own terms first, then its series', then every film's, each source once. Series is the name of the film's series, "" for none. Writable reports whether the film can keep terms of its own, as one whose video the computer keeps; a phone offers to add a term only then.
type Glossary struct {
	Library  string `json:"library"`
	Film     string `json:"film"`
	Terms    []Term `json:"terms"`
	Series   string `json:"series,omitempty"`
	Writable bool   `json:"writable,omitempty"`
}

// TermPut adds Term to the glossary of film Film of library Library: a term for the same source the film's glossary has already takes Target and Locked, in the scope it is in; another is added to the film's own terms. Scope is not read. A term without a source or a target answers CodeInvalid, a film that cannot keep terms (Glossary.Writable) CodeNotReady.
type TermPut struct {
	Library string `json:"library"`
	Film    string `json:"film"`
	Term    Term   `json:"term"`
}

// GlossaryService is what a desktop does for the glossary's calls, whichever transport carries them. Its errors are as Service's.
type GlossaryService interface {
	Terms(ctx context.Context, caller Caller, req FilmRef) (Glossary, error)
	PutTerm(ctx context.Context, caller Caller, req TermPut) (Glossary, error)
}

// answerGlossary runs a call of the glossary.
func (d *Desk) answerGlossary(ctx context.Context, caller Caller, call Call) (any, error) {
	if d.Glossary == nil {
		return nil, Errorf(CodeUnsupported, "this desktop does not share its glossary")
	}
	if call.Op == OpTerms {
		var req FilmRef
		if err := args(call, &req); err != nil {
			return nil, err
		}
		return d.Glossary.Terms(ctx, caller, req)
	}
	var req TermPut
	if err := args(call, &req); err != nil {
		return nil, err
	}
	return d.Glossary.PutTerm(ctx, caller, req)
}

// Terms is the glossary of film of library.
func (c *Client) Terms(ctx context.Context, library, film string) (Glossary, error) {
	var out Glossary
	err := c.Call(ctx, OpTerms, FilmRef{Library: library, Film: film}, &out)
	return out, err
}

// PutTerm adds a term to a film's glossary, and answers the glossary as it is after.
func (c *Client) PutTerm(ctx context.Context, req TermPut) (Glossary, error) {
	var out Glossary
	err := c.Call(ctx, OpTermPut, req, &out)
	return out, err
}
