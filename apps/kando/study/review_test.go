package study

import (
	"slices"
	"testing"
	"time"

	"kando/apps/kando/live"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

type fakeReview struct {
	live.Rev
	cards []Card
	calls []string
}

func (f *fakeReview) ReviewCard() Card {
	if len(f.cards) == 0 {
		return Card{Done: true, Reviewed: 2}
	}
	return f.cards[0]
}
func (f *fakeReview) ReviewReveal() { f.calls = append(f.calls, "reveal"); f.cards[0].Revealed = true }
func (f *fakeReview) ReviewGrade(good bool) {
	if good {
		f.calls = append(f.calls, "good")
	} else {
		f.calls = append(f.calls, "again")
	}
	f.cards = f.cards[1:]
}
func (f *fakeReview) ReviewReplay()    { f.calls = append(f.calls, "replay"); f.cards[0].Plays++ }
func (f *fakeReview) ReviewClipEnded() { f.calls = append(f.calls, "ended") }
func (f *fakeReview) CloseReview()     { f.calls = append(f.calls, "close") }

// TestReviewHidesTheTranslationUntilShown goes through two lines: the first shows its original with the translation hidden, plays its sound, shows the translation on Space and is answered with 2; the second, without a sound, is shown with Show Translation and answered with Again; then the review says how many were reviewed, and Done closes it. R plays the sound again from its start.
func TestReviewHidesTheTranslationUntilShown(t *testing.T) {
	a := &fakeReview{cards: []Card{
		{Left: 2, Film: "Talk", Original: "Every river", Translation: "每条河", Start: time.Second, Clip: "/tmp/clip.m4a", Playing: true, Plays: 1},
		{Left: 1, Film: "Talk", Original: "Bye.", NoAudio: true},
	}}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		a.Attach(cx)
		return Review{App: a}
	}, nsgotest.Size(1100, 720), nsgotest.Environment(testlocale.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop})))
	msgs := testlocale.Messages()
	refresh := func() { a.Changed(); app.Settle() }
	if !app.Exists(whole("Every river")) || app.Exists(whole("每条河")) || !app.Exists(whole(msgs.SavedReviewLeft(2))) || !app.Exists(whole(msgs.SavedWhere("Talk", "0:01.0"))) {
		t.Fatalf("the first line shows %q", app.Texts())
	}
	clip := app.Find("id:review-clip").Video
	if clip.Source != "/tmp/clip.m4a" || clip.Paused {
		t.Fatalf("the sound %+v", clip)
	}
	app.Key("2")
	if len(a.calls) != 0 {
		t.Fatalf("2 answered before the translation showed: %q", a.calls)
	}
	app.Key("r")
	refresh()
	if seeks := app.Find("id:review-clip").Seeks; !slices.Equal(seeks, []time.Duration{0}) {
		t.Fatalf("R seeked %v", seeks)
	}
	app.Key("space")
	refresh()
	if !app.Exists(whole("每条河")) || !app.Exists("id:review-good") {
		t.Fatalf("Space shows %q", app.Texts())
	}
	app.Key("2")
	refresh()
	if !app.Exists(whole("Bye.")) || !app.Exists(whole(msgs.SavedReviewNoAudio())) || app.Exists("id:review-replay") {
		t.Fatalf("the second line shows %q", app.Texts())
	}
	app.Press("id:review-show")
	refresh()
	app.Press("id:review-again")
	refresh()
	if !app.Exists(whole(msgs.SavedReviewDone(2))) {
		t.Fatalf("once done: %q", app.Texts())
	}
	app.Press("id:review-close")
	if want := []string{"replay", "reveal", "good", "reveal", "again", "close"}; !slices.Equal(a.calls, want) {
		t.Fatalf("calls %q, want %q", a.calls, want)
	}
}

// TestReviewFollowsTheAppearance checks the review in the window's theme as the system turns dark: the line's original and translation turn light.
func TestReviewFollowsTheAppearance(t *testing.T) {
	a := &fakeReview{cards: []Card{{Left: 1, Film: "Talk", Original: "Every river", Translation: "每条河", Revealed: true, NoAudio: true}}}
	env := testlocale.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop})
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		a.Attach(cx)
		theme.Apply(cx)
		return Review{App: a}
	}, nsgotest.Size(1100, 720), nsgotest.Environment(env))
	ink := func(id string) core.Color { return app.Find("id:" + id).TextStyle.Color }
	for _, id := range []string{"review-original", "review-translation"} {
		if r, g, b, _ := ink(id).Components(); r > 0.5 || g > 0.5 || b > 0.5 {
			t.Fatalf("%s is %#x on the light review, want a dark ink", id, uint32(ink(id)))
		}
	}
	env.Dark = true
	app.SetEnvironment(env)
	for _, id := range []string{"review-original", "review-translation"} {
		if r, g, b, _ := ink(id).Components(); r < 0.5 || g < 0.5 || b < 0.5 {
			t.Fatalf("%s is %#x on the dark review, want a light ink", id, uint32(ink(id)))
		}
	}
}
