package synctest

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"testing"

	"kando/packages/sync"
)

func TestTheGlossaryOverV2(t *testing.T) {
	ctx := context.Background()
	d := New(t, "Mac")
	d.Offer("123456")
	c := sync.NewClient(d.Addr, d.Fingerprint, "")
	t.Cleanup(c.Close)
	paired, err := c.Pair(ctx, sync.PairRequest{Code: "123456", Device: "Pixel", Platform: sync.PlatformAndroid})
	if err != nil {
		t.Fatal(err)
	}
	c = sync.NewClient(d.Addr, d.Fingerprint, paired.Token)
	t.Cleanup(c.Close)

	h, err := c.Hello(ctx)
	if err != nil || !slices.Contains(h.Ops, sync.OpTerms) || !slices.Contains(h.Ops, sync.OpTermPut) {
		t.Fatalf("hello %+v, %v: no glossary", h, err)
	}
	d.SetGlossary("lib", "f1", "Rivers", true, sync.Term{Source: "meander", Target: "河曲", Scope: sync.TermFilm, Locked: true}, sync.Term{Source: "delta", Target: "三角洲", Scope: sync.TermGlobal})
	g, err := c.Terms(ctx, "lib", "f1")
	if err != nil || len(g.Terms) != 2 || g.Series != "Rivers" || !g.Writable || !g.Terms[0].Locked {
		t.Fatalf("terms %+v, %v", g, err)
	}

	// A term for words the glossary has takes their place in the scope they are in; another goes into the film's own terms.
	g, err = c.PutTerm(ctx, sync.TermPut{Library: "lib", Film: "f1", Term: sync.Term{Source: "Delta", Target: "河口三角洲", Locked: true}})
	if err != nil || len(g.Terms) != 2 || g.Terms[1].Target != "河口三角洲" || g.Terms[1].Scope != sync.TermGlobal || !g.Terms[1].Locked {
		t.Fatalf("changing a term answered %+v, %v", g, err)
	}
	g, err = c.PutTerm(ctx, sync.TermPut{Library: "lib", Film: "f1", Term: sync.Term{Source: "oxbow lake", Target: "牛轭湖", Locked: true}})
	if err != nil || len(g.Terms) != 3 || g.Terms[0].Source != "oxbow lake" || g.Terms[0].Scope != sync.TermFilm {
		t.Fatalf("adding a term answered %+v, %v", g, err)
	}
	var e *sync.Error
	if _, err := c.PutTerm(ctx, sync.TermPut{Library: "lib", Film: "f1", Term: sync.Term{Source: "levee"}}); !errors.As(err, &e) || e.Code != sync.CodeInvalid || e.Status != http.StatusBadRequest {
		t.Fatalf("a term without a translation answered %v", err)
	}

	// A read-only phone reads the glossary but changes nothing.
	d.SetReadOnly(true)
	if g, err := c.Terms(ctx, "lib", "f1"); err != nil || len(g.Terms) != 3 {
		t.Fatalf("a read-only phone read %+v, %v", g, err)
	}
	if _, err := c.PutTerm(ctx, sync.TermPut{Library: "lib", Film: "f1", Term: sync.Term{Source: "levee", Target: "堤"}}); !errors.As(err, &e) || e.Code != sync.CodeReadOnly || e.Status != http.StatusForbidden {
		t.Fatalf("a read-only phone's term answered %v", err)
	}
	if got := d.Glossary("lib", "f1"); len(got.Terms) != 3 {
		t.Fatalf("a read-only phone changed the glossary: %+v", got)
	}
	d.SetReadOnly(false)

	// A desktop that does not share its glossary answers its calls unsupported, and does not list them.
	d.SetNoGlossary(true)
	if h, err := c.Hello(ctx); err != nil || slices.Contains(h.Ops, sync.OpTerms) {
		t.Fatalf("hello %+v, %v lists the glossary", h, err)
	}
	if _, err := c.Terms(ctx, "lib", "f1"); !errors.As(err, &e) || e.Code != sync.CodeUnsupported {
		t.Fatalf("an unshared glossary answered %v", err)
	}
}
