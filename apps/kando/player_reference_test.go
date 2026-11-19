package main

import (
	"os"
	"testing"
	"time"

	"kando/packages/media"
)

// A film whose job kept the captions its site made itself opens with them as its reference layer: the line playing shows on the picture above the film's own, until the subtitle menu hides it, for the films opened after too.
func TestFilmShowsItsReferenceLayer(t *testing.T) {
	a, tt, updates, item := playerFilm(t, 12, nil)
	if err := os.WriteFile(media.ReferencePath(item.VideoPath), []byte("1\n00:00:00,500 --> 00:00:04,000\nhello every one\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	playerOpenFilm(t, a, tt, updates, item)
	waitUntil(t, tt, a, updates, func() bool { return len(a.Reference()) == 1 })
	playerLoadFilm(tt, a, 12*time.Second)
	playerPlayTo(tt, a, 1500*time.Millisecond)
	wantText(t, tt, "hello every one")
	apply(tt, a, func() { a.SetShowReference(false) })
	if hasText(tt, "hello every one") {
		t.Fatal("the hidden reference layer shows")
	}
	apply(tt, a, func() { a.resetFilm() })
	if a.ShowReference() || a.Reference() != nil {
		t.Fatalf("another film: shown %v, reference %+v", a.ShowReference(), a.Reference())
	}
}
