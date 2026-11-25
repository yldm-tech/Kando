package main

import (
	"testing"

	"kando/apps/mobile/nas"
	"kando/apps/mobile/store"
	"kando/packages/remotefs/davtest"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// toChoosing takes a new phone's introduction to its choosing step.
func (p *phone) toChoosing() {
	p.t.Helper()
	p.app.WaitFor("id:onboarding-intro", wait)
	p.app.Press("id:onboarding-continue")
	p.app.WaitFor("id:onboarding-languages", wait)
	p.app.Press("id:onboarding-continue")
	p.app.WaitFor("id:onboarding-choosing", wait)
}

// addDav fills the introduction's form with a WebDAV source of f's Films folder, named Home NAS, and saves it.
func (p *phone) addDav(f *davtest.Server) {
	p.t.Helper()
	p.app.WaitFor("id:onboarding-adding", wait)
	p.app.Press("id:source-kind-webdav")
	p.app.WaitFor("id:source-field-url", wait)
	p.app.Type("id:source-name", "Home NAS")
	p.app.Type("id:source-field-url", f.URL)
	p.app.Type("id:source-field-root", "Films")
	p.app.Type("id:source-field-user", davtest.User)
	p.app.Type("id:source-field-password", davtest.Password)
	p.app.Press("id:source-save")
	p.app.WaitFor("id:onboarding-added", wait)
}

// steps is what the introduction's step dots say.
func (p *phone) steps() string {
	p.t.Helper()
	return p.app.Find("id:onboarding-steps").Props.Label
}

// The choosing step asks where the films are as three cards, one chosen at a time, which assistive technology hears as radios with their lines; going on waits for a choice. The NAS card says its films have no subtitles. A single way is three steps, both four.
func TestTheIntroductionAsksWhereTheFilmsAre(t *testing.T) {
	p := startPhone(t, "")
	p.toChoosing()
	if got, want := p.steps(), msgs.OnboardingStep(3, 3); got != want {
		t.Fatalf("the dots say %q before a way is chosen, want %q", got, want)
	}
	if !p.app.Find("id:onboarding-continue").Props.Disabled {
		t.Fatal("the introduction goes on before a way is chosen")
	}
	ways := []string{"computer", "nas", "both"}
	for _, w := range ways {
		v := p.app.Find("id:onboarding-way-" + w)
		if v.Props.Role != core.RoleRadio || v.Props.Checked || v.Props.Label == "" || v.Props.Description == "" {
			t.Fatalf("the %s card is %+v, want an unchecked radio with a label and a description", w, v.Props)
		}
	}
	if got, want := p.app.Find("id:onboarding-way-nas").Props.Description, msgs.OnboardingWayNASLine(); got != want {
		t.Fatalf("the NAS card says %q, want %q", got, want)
	}
	p.app.Press("id:onboarding-way-both")
	p.app.WaitUntil(wait, func() bool { return p.steps() == msgs.OnboardingStep(3, 4) })
	p.app.Press("id:onboarding-way-nas")
	p.app.WaitUntil(wait, func() bool { return p.steps() == msgs.OnboardingStep(3, 3) })
	for _, w := range ways {
		if on := p.app.Find("id:onboarding-way-" + w).Props.Checked; on != (w == "nas") {
			t.Fatalf("with NAS chosen the %s card is checked %v", w, on)
		}
	}
	if p.app.Find("id:onboarding-continue").Props.Disabled {
		t.Fatal("the introduction does not go on once a way is chosen")
	}
}

// Kando on the computer chosen, the introduction pairs, as its last step, which starts once paired.
func TestTheIntroductionPairsWhenTheComputerIsChosen(t *testing.T) {
	d := newDesktop(t)
	p := startPhone(t, "")
	p.toChoosing()
	p.chooseWay("computer")
	p.app.WaitFor("id:onboarding-pairing", wait)
	if got, want := p.steps(), msgs.OnboardingStep(3, 3); got != want {
		t.Fatalf("the pairing step's dots say %q, want %q", got, want)
	}
	p.fillPairing(d.Addr.String(), "123456")
	p.app.Press("id:pair-continue")
	p.app.WaitFor("id:pair-confirm", wait)
	p.app.Press("id:pair-confirm")
	p.finishPairing(d)
	if p.app.Exists("id:films-sources") {
		t.Fatal("the Library shows a NAS group though none was added")
	}
}

// A NAS chosen, the introduction adds it with the form Settings has, Save at the page's foot; saved, it names the source and where it is, and starts on the Library, whose NAS group opens its folder.
func TestTheIntroductionAddsTheNASChosen(t *testing.T) {
	f := davFilms(t, false)
	p := startPhone(t, "")
	p.toChoosing()
	p.chooseWay("nas")
	p.app.WaitFor("id:onboarding-adding", wait)
	if got, want := p.steps(), msgs.OnboardingStep(3, 3); got != want {
		t.Fatalf("the adding step's dots say %q, want %q", got, want)
	}
	if p.app.Exists("id:onboarding-pairing") || p.app.Exists("id:pair-address") {
		t.Fatal("the NAS way shows the pairing form")
	}
	p.addDav(f)
	list, err := store.Open(p.dir).Sources()
	if err != nil || len(list) != 1 {
		t.Fatalf("sources.json holds %v, %v", list, err)
	}
	src := list[0]
	if src.Kind != nas.KindWebDAV || src.Name != "Home NAS" || src.Config[nas.URL] != f.URL {
		t.Fatalf("sources.json keeps %+v", src)
	}
	if got, ok := p.secret(store.SourceSecretAccount(src.ID, nas.Password)); !ok || got != davtest.Password {
		t.Fatalf("the secure store keeps %q, %v", got, ok)
	}
	if got, want := p.texts("id:onboarding-added"), msgs.OnboardingAdded("Home NAS"); got != want {
		t.Fatalf("the added card says %q, want %q", got, want)
	}
	if p.app.Exists("id:source-save") || p.app.Exists("id:onboarding-source-later") {
		t.Fatal("the form's buttons stay once the source is added")
	}
	p.app.Press("id:onboarding-start")
	p.app.WaitFor("id:films-source-"+src.ID, wait)
	p.app.WaitUntil(wait, func() bool { prefs, _ := store.Open(p.dir).Prefs(); return prefs.Onboarded })
	p.app.Press("id:films-source-" + src.ID)
	p.app.WaitFor("id:browse-video-Holiday.mp4", wait)
}

// Both chosen, the introduction pairs, goes on rather than starting, then adds the NAS as its fourth and last step; the Library has both the computer's films and the NAS. Back goes back through the steps, keeping what was typed.
func TestTheIntroductionPairsThenAddsForBoth(t *testing.T) {
	d := newDesktop(t)
	f := davFilms(t, false)
	p := startPhone(t, "")
	p.toChoosing()
	if !p.app.Exists("id:onboarding-back") {
		t.Fatal("the choosing step has no Back")
	}
	p.chooseWay("both")
	p.app.WaitFor("id:onboarding-pairing", wait)
	if got, want := p.steps(), msgs.OnboardingStep(3, 4); got != want {
		t.Fatalf("the pairing step's dots say %q, want %q", got, want)
	}
	p.fillPairing(d.Addr.String(), "123456")
	p.app.Press("id:pair-continue")
	p.app.WaitFor("id:pair-confirm", wait)
	p.app.Press("id:pair-confirm")
	p.app.WaitFor("id:onboarding-paired", wait)
	if p.app.Exists("id:onboarding-start") {
		t.Fatal("the pairing step starts though a NAS is still to add")
	}
	p.app.Press("id:onboarding-continue")
	p.app.WaitFor("id:onboarding-adding", wait)
	if got, want := p.steps(), msgs.OnboardingStep(4, 4); got != want {
		t.Fatalf("the adding step's dots say %q, want %q", got, want)
	}

	// Back, the button and the gesture, goes to the pairing step, still paired, and to the choice; on again, the form keeps what was typed.
	p.app.Type("id:source-name", "Home NAS")
	p.app.Press("id:onboarding-back")
	p.app.WaitFor("id:onboarding-paired", wait)
	p.app.Back()
	p.app.WaitFor("id:onboarding-choosing", wait)
	if !p.app.Find("id:onboarding-way-both").Props.Checked {
		t.Fatal("going back forgot the way chosen")
	}
	p.app.Press("id:onboarding-continue")
	p.app.WaitFor("id:onboarding-paired", wait)
	p.app.Press("id:onboarding-continue")
	p.app.WaitFor("id:onboarding-adding", wait)
	if got := p.texts("id:source-name"); got != "Home NAS" {
		t.Fatalf("the form's name is %q after going back and on", got)
	}

	p.addDav(f)
	list, _ := store.Open(p.dir).Sources()
	if len(list) != 1 {
		t.Fatalf("sources.json holds %v", list)
	}
	p.app.Press("id:onboarding-start")
	p.app.WaitFor(talkRow(store.DesktopID(d.Fingerprint)), wait)
	p.app.WaitFor("id:films-source-"+list[0].ID, wait)
}

// Every step after choosing can be skipped: both chosen, going on without a computer goes to the NAS, and going on without a NAS leaves the introduction with neither; Skip at the top leaves from the NAS step as from any other.
func TestTheIntroductionsWaysCanBeSkipped(t *testing.T) {
	p := startPhone(t, "")
	p.toChoosing()
	p.chooseWay("both")
	p.app.WaitFor("id:onboarding-pairing", wait)
	p.app.Press("id:onboarding-later")
	p.app.WaitFor("id:onboarding-adding", wait)
	p.app.Press("id:onboarding-source-later")
	p.app.WaitFor("id:films-unpaired", wait)
	p.app.WaitUntil(wait, func() bool { prefs, _ := store.Open(p.dir).Prefs(); return prefs.Onboarded })
	if list, _ := store.Open(p.dir).Sources(); len(list) != 0 || p.app.Exists("id:films-sources") {
		t.Fatalf("skipping added the sources %v", list)
	}

	p = startPhone(t, "")
	p.toChoosing()
	p.chooseWay("nas")
	p.app.WaitFor("id:onboarding-adding", wait)
	p.app.Press("id:onboarding-skip")
	p.app.WaitFor("id:films-unpaired", wait)
}

// On a tablet the choosing and adding steps are in the card, its buttons across it.
func TestTheIntroductionsWaysOnATablet(t *testing.T) {
	p := startPhone(t, "", nsgotest.Size(1180, 820), nsgotest.Environment(core.Environment{Locale: lang, Idiom: core.IdiomTablet}))
	p.toChoosing()
	card := p.app.Find("id:onboarding-card").Frame.Size
	if w := p.app.Find("id:onboarding-way-nas").Frame.Size.Width; w <= 0 || w > card.Width {
		t.Fatalf("the NAS card is %v wide in a card %v wide", w, card.Width)
	}
	p.chooseWay("nas")
	p.app.WaitFor("id:onboarding-adding", wait)
	if w := p.app.Find("id:source-save").Frame.Size.Width; w <= 0 || w > card.Width {
		t.Fatalf("Save is %v wide in a card %v wide", w, card.Width)
	}
}

// On a phone the step's button stays above the software keyboard while it shows, so a form's Save is not under it.
func TestTheIntroductionsButtonStaysAboveTheKeyboard(t *testing.T) {
	p := startPhone(t, "")
	p.toChoosing()
	p.chooseWay("nas")
	p.app.WaitFor("id:onboarding-adding", wait)
	const keyboard = 336
	p.app.SetEnvironment(core.Environment{Locale: lang, KeyboardHeight: keyboard})
	save := p.app.WaitFor("id:source-save", wait).Bounds()
	if bottom := save.Origin.Y + save.Size.Height; bottom > 844-keyboard {
		t.Fatalf("Save ends at %v, under the keyboard from %v", bottom, 844-keyboard)
	}
	p.app.SetEnvironment(core.Environment{Locale: lang})
	if save := p.app.Find("id:source-save").Bounds(); save.Origin.Y+save.Size.Height <= 844-keyboard {
		t.Fatalf("Save stays up at %v once the keyboard is gone", save.Origin.Y+save.Size.Height)
	}
}
