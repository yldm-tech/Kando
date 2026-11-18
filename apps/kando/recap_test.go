package main

import (
	"strings"
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/recap"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// recapReply is what the AI service sums up rivers' first 0:31 with: three items at lines watched.
const recapReply = `{"items":[{"at":"0:00","text":"河流始于高山细流。"},{"at":"0:10","text":"水流裹挟泥沙而下。"},{"at":"0:30","text":"河道越切越深。"}]}`

// recapOpen opens rivers of paletteLibrary, played to 0:31, as its poster does: held where it stopped last time, at 0:31, and waits for the card to settle, as it does once the film's lines are read.
func recapOpen(t *testing.T, tools media.Tools) (*app, *nsgotest.App, chan func()) {
	t.Helper()
	a, tt, updates := paletteLibrary(t, tools)
	item, _ := a.paletteFilm("rivers")
	apply(tt, a, func() {
		a.player.positions = map[string]time.Duration{item.VideoPath: 31 * time.Second}
		a.Open(item)
	})
	waitUntil(t, tt, a, updates, func() bool { return a.player.subs.ready })
	refresh(tt, a)
	return a, tt, updates
}

// TestRecapSumsUpWhatWasWatched checks that a film opened held where it stopped last time shows the 前情提要 card: where it stopped, and the AI service's three items of what happened in the part watched, each at the line it is about, from only the lines watched, which the footnote says, naming the model and that it runs on this computer; 从 0:31 继续 closes the card and plays on from there.
func TestRecapSumsUpWhatWasWatched(t *testing.T) {
	srv, seen := fakeLLM(t, recapReply)
	a, tt, updates := recapOpen(t, media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}})
	msgs := testlocale.Messages()
	if r := a.Recap(); !r.Open || !r.Busy || r.At != 31*time.Second || !hasText(tt, msgs.RecapBusy()) || !hasText(tt, msgs.RecapLastAt("0:31")) {
		t.Fatalf("the card while it sums up: %+v, texts %q", r, tt.Texts())
	}
	waitUntil(t, tt, a, updates, func() bool { return !a.Recap().Busy })
	for _, want := range []string{"河流始于高山细流。", "水流裹挟泥沙而下。", "河道越切越深。", "0:30", msgs.RecapContinue("0:31"), msgs.RecapStartOver(), msgs.RecapNoteLocal("m1")} {
		if !hasText(tt, want) {
			t.Fatalf("missing %q in %q", want, tt.Texts())
		}
	}
	if items := a.Recap().Items; len(items) != 3 || items[2].At != 30*time.Second+400*time.Millisecond {
		t.Fatalf("items %+v", items)
	}
	sent := strings.Join(*seen, "\n")
	if !strings.Contains(sent, "[0:30] 数千年间，河道被切割得越来越深。") || strings.Contains(sent, "meanders") {
		t.Fatalf("sent %q: want the lines watched and none after", sent)
	}
	tt.Press("id:film-recap-continue")
	if a.Recap().Open || tt.Exists("id:film-recap") || !a.player.play.played || a.player.play.head != 31*time.Second {
		t.Fatalf("继续: card %+v, playing %v at %v", a.Recap(), a.player.play.played, a.player.play.head)
	}
	// Opened again before more was watched, it shows the same recap without asking again.
	asked := len(*seen)
	item, _ := a.paletteFilm("rivers")
	a.closeFilm()
	apply(tt, a, func() {
		a.player.positions = map[string]time.Duration{item.VideoPath: 31 * time.Second}
		a.Open(item)
	})
	waitUntil(t, tt, a, updates, func() bool { return a.player.subs.ready })
	refresh(tt, a)
	if r := a.Recap(); !r.Open || r.Busy || len(r.Items) != 3 || len(*seen) != asked {
		t.Fatalf("opened again: %+v after %d asks, %d before", r, len(*seen), asked)
	}
}

// TestRecapWithoutAIShowsLinesWatched checks that without an AI service the card shows the first, middle and last lines watched and says they are not summed up; a click on one plays from the moment the picture shows it, and 从头看 plays from the start.
func TestRecapWithoutAIShowsLinesWatched(t *testing.T) {
	a, tt, _ := recapOpen(t, media.Tools{Translator: media.ChatTranslator{BaseURL: "https://api.openai.com/v1"}})
	msgs := testlocale.Messages()
	r := a.Recap()
	if !r.Open || r.Busy || !r.Excerpts || len(r.Items) != 3 || !hasText(tt, msgs.RecapExcerptNote()) {
		t.Fatalf("the card without AI: %+v, texts %q", r, tt.Texts())
	}
	for _, want := range []string{"每条河都始于高山上的一股细流。", "字幕里的每个词都能查。", "数千年间，河道被切割得越来越深。"} {
		if !hasText(tt, want) {
			t.Fatalf("missing %q in %q", want, tt.Texts())
		}
	}
	apply(tt, a, func() { a.player.film.delay = time.Second })
	tt.Press("id:film-recap-item >> nth=1")
	if a.Recap().Open || a.player.play.head != 21*time.Second+400*time.Millisecond {
		t.Fatalf("the item played from %v", a.player.play.head)
	}
	a2, tt2, _ := recapOpen(t, media.Tools{})
	tt2.Press("id:film-recap-start")
	if a2.Recap().Open || !a2.player.play.played || a2.player.play.head != 0 {
		t.Fatalf("从头看 played from %v", a2.player.play.head)
	}
}

// TestRecapShowsOnlyForAFilmHeldWhereItStopped checks that no card shows for a film opened playing, as by 继续, nor for one of which nothing was watched, and that the card closes as the film plays, at ×, and once it is not the film open; a cloud model's footnote does not say it ran on this computer.
func TestRecapShowsOnlyForAFilmHeldWhereItStopped(t *testing.T) {
	srv, _ := fakeLLM(t, recapReply)
	tools := media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}}
	a, tt, updates := paletteLibrary(t, tools)
	rivers, _ := a.paletteFilm("rivers")
	market, _ := a.paletteFilm("market")
	apply(tt, a, func() {
		a.player.positions = map[string]time.Duration{rivers.VideoPath: 31 * time.Second, market.VideoPath: 20 * time.Second}
		a.OpenAt(rivers, 31*time.Second)
	})
	waitUntil(t, tt, a, updates, func() bool { return a.player.subs.ready })
	refresh(tt, a)
	if a.Recap().Open || a.recap.card.open {
		t.Fatal("a film opened playing shows the card")
	}
	apply(tt, a, func() { a.Open(market) })
	waitUntil(t, tt, a, updates, func() bool { return a.player.subs.ready })
	refresh(tt, a)
	if a.Recap().Open || a.recap.card.open {
		t.Fatal("a film of which nothing was watched shows the card")
	}
	apply(tt, a, func() { a.Open(rivers) })
	waitUntil(t, tt, a, updates, func() bool { return a.player.subs.ready && !a.Recap().Busy })
	if !a.Recap().Open {
		t.Fatal("no card for a film held where it stopped")
	}
	apply(tt, a, func() { a.recap.card.local = false })
	if note := a.Recap().Note(a.cx); note != testlocale.Messages().RecapNote("m1") {
		t.Fatalf("a cloud model's footnote %q", note)
	}
	tt.Press("id:film-recap-close")
	if a.Recap().Open || a.player.play.played {
		t.Fatal("× did not close the card, or played the film")
	}
	apply(tt, a, func() { a.Open(rivers) })
	waitUntil(t, tt, a, updates, func() bool { return a.player.subs.ready && a.Recap().Open })
	apply(tt, a, func() { a.player.play.toggle() })
	if a.Recap().Open {
		t.Fatal("the card stays as the film plays")
	}
	apply(tt, a, func() { a.Open(rivers) })
	waitUntil(t, tt, a, updates, func() bool { return a.player.subs.ready && a.Recap().Open })
	apply(tt, a, a.closeFilm)
	if a.recap.card.open {
		t.Fatal("the card stays once the film closed")
	}
}

// TestRecapFallsBackWhenTheServiceFails checks that when the AI service fails, the card shows the lines watched and says it could not sum them up.
func TestRecapFallsBackWhenTheServiceFails(t *testing.T) {
	srv, _ := fakeLLM(t, "not a recap")
	a, tt, updates := recapOpen(t, media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}})
	waitUntil(t, tt, a, updates, func() bool { return !a.Recap().Busy })
	if r := a.Recap(); !r.Excerpts || len(r.Items) != len(recap.Excerpts(a.recapSeen())) || !hasText(tt, testlocale.Messages().RecapFailedNote()) {
		t.Fatalf("failed recap: %+v, texts %q", r, tt.Texts())
	}
}
