package synctest

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"testing"

	"kando/packages/sync"
)

func TestFilmMarksOverV2(t *testing.T) {
	ctx := context.Background()
	d := New(t, "Mac")
	d.AddLibrary("lib", "Library")
	d.AddFilm(Film{Film: sync.Film{ID: "f1", Title: "Rain"}, Library: "lib", Cues: []sync.Cue{{StartMS: 0, EndMS: 1000, Original: "今日は雨"}}})
	d.Offer("123456")
	c := sync.NewClient(d.Addr, d.Fingerprint, "")
	t.Cleanup(c.Close)
	paired, err := c.Pair(ctx, sync.PairRequest{Code: "123456", Device: "Pixel", Platform: sync.PlatformAndroid})
	if err != nil {
		t.Fatal(err)
	}
	c = sync.NewClient(d.Addr, d.Fingerprint, paired.Token)
	t.Cleanup(c.Close)

	if h, err := c.Hello(ctx); err != nil || !slices.Contains(h.Ops, sync.OpFilmMarks) {
		t.Fatalf("hello %+v, %v: no marks", h, err)
	}
	want := sync.FilmMarks{Lines: []sync.LineMarks{{Line: 0, Hash: sync.LineHash("今日は雨"), Marks: []sync.Mark{{Start: 0, End: 6, Ruby: "きょう"}, {Start: 9, End: 12, Tone: sync.ToneAccent, Underline: true}}}}}
	d.SetMarks("f1", want)
	got, err := c.FilmMarks(ctx, "f1")
	if err != nil || got.Pending || len(got.Lines) != 1 || !slices.Equal(got.Lines[0].Marks, want.Lines[0].Marks) {
		t.Fatalf("marks %+v, %v", got, err)
	}
	if of := got.Of([]string{"今日は雨"}); len(of[0]) != 2 {
		t.Fatalf("the line's marks read %+v", of)
	}
	var e *sync.Error
	if _, err := c.FilmMarks(ctx, "nope"); !errors.As(err, &e) || e.Code != sync.CodeNotFound {
		t.Fatalf("a film not served answered %v", err)
	}
	d.SetMarksPending(true)
	if got, err := c.FilmMarks(ctx, "f1"); err != nil || !got.Pending || len(got.Lines) != 0 {
		t.Fatalf("marks being computed answered %+v, %v", got, err)
	}
	d.SetMarksPending(false)

	// A read-only phone reads them too.
	d.SetReadOnly(true)
	if got, err := c.FilmMarks(ctx, "f1"); err != nil || len(got.Lines) != 1 {
		t.Fatalf("a read-only phone read %+v, %v", got, err)
	}

	// A desktop from before them leaves them out of hello and answers the call unsupported; a v1 desktop has no calls at all.
	d.SetNoMarks(true)
	if h, err := c.Hello(ctx); err != nil || slices.Contains(h.Ops, sync.OpFilmMarks) {
		t.Fatalf("an older desktop's hello %+v, %v", h, err)
	}
	if _, err := c.FilmMarks(ctx, "f1"); !errors.As(err, &e) || e.Code != sync.CodeUnsupported || e.Status != http.StatusNotImplemented {
		t.Fatalf("an older desktop answered %v", err)
	}
	d.SetOlder(true)
	if _, err := c.FilmMarks(ctx, "f1"); !errors.As(err, &e) || e.Status != http.StatusNotFound {
		t.Fatalf("a v1 desktop answered %v", err)
	}
}
