package main

import (
	"slices"
	"strings"
	"testing"

	"kando/apps/mobile/store"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// A phone paired before the introduction existed opens on the Library; each tab keeps the pages it opened while another shows, a film downloaded meanwhile does not take the user away, and the tab shown, pressed again, goes back to its first page.
func TestTheTabsKeepTheirPagesAndGoBackWhenPressedAgain(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.app.WaitFor(talkRow(store.DesktopID(d.Fingerprint)), wait)
	if p.app.Exists("id:onboarding") {
		t.Fatal("a phone paired already shows the introduction")
	}
	p.app.Press(talkRow(store.DesktopID(d.Fingerprint)))
	p.app.WaitFor("id:film-download", wait)
	p.app.Press("id:film-download")
	p.app.WaitFor("id:film-play", wait)
	if !p.app.Exists("id:film-page") {
		t.Fatal("a film downloaded took the user away from its page")
	}

	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.WaitFor("id:settings-computer-"+store.DesktopID(d.Fingerprint), wait)
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitFor("id:learn-empty", wait)
	p.openTab(filmsTab, msgs.FilmsTitle())
	p.app.WaitFor("id:film-page", wait)

	p.openTab(filmsTab, msgs.FilmsTitle())
	p.app.WaitForGone("id:film-page", wait)
	p.app.WaitFor(talkRow(store.DesktopID(d.Fingerprint)), wait)
}

// With films on the phone and nothing paired, the Library lists them under the card that asks to pair; pairing from there shows the computer's libraries in its place, and Settings offers to pair another.
func TestPairingFromTheLibraryAndSettings(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	keepTalk(t, dir)
	p := startPhone(t, dir)
	p.app.WaitFor(talkRow("gonedesk"), wait)
	p.app.Press("id:films-pair")
	p.fillPairing(d.Addr.String(), "123456")
	p.app.Press("id:pair-continue")
	p.app.WaitFor("id:pair-compare", wait)
	p.app.Press("id:pair-confirm")
	p.app.WaitFor(talkRow(store.DesktopID(d.Fingerprint)), wait)
	p.app.WaitForGone("id:films-unpaired", wait)
	if !p.app.Exists(talkRow("gonedesk")) {
		t.Fatal("pairing hid the films on the phone")
	}
	p.openTab(settingsTab, msgs.SettingsTitle())
	if got := p.texts("id:settings-pair"); got != msgs.SettingsPairAnother() {
		t.Fatalf("the row that pairs says %q with a computer paired, want %q", got, msgs.SettingsPairAnother())
	}
	p.app.Press("id:settings-pair")
	p.app.WaitFor("id:pair-page", wait)
}

// Unpaired in Settings, a computer leaves its downloads on the phone: the Library lists them again under the card that asks to pair.
func TestUnpairingInSettingsKeepsTheDownloads(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	keepTalkFrom(t, dir, store.DesktopID(d.Fingerprint))
	p := startPhone(t, dir, secrets)
	p.app.WaitFor(talkRow(store.DesktopID(d.Fingerprint)), wait)
	p.openTab(settingsTab, msgs.SettingsTitle())
	id := store.DesktopID(d.Fingerprint)
	p.app.Press("id:settings-computer-options-" + id)
	if alerts := p.app.Alerts(); len(alerts) == 0 || !alerts[len(alerts)-1].Sheet {
		t.Fatal("a phone asks to unpair in something other than an action sheet")
	}
	p.app.Answer(1)
	p.app.WaitForGone("id:settings-computer-"+id, wait)
	if got := p.texts("id:settings-pair"); got != msgs.SettingsPair() {
		t.Fatalf("the row that pairs says %q with nothing paired, want %q", got, msgs.SettingsPair())
	}
	p.openTab(filmsTab, msgs.FilmsTitle())
	p.app.WaitFor("id:films-unpaired", wait)
	p.app.WaitFor(talkRow(id), wait)
}

// A new phone starts with the introduction; skipped, it shows the Library asking to pair and is not shown again.
func TestTheIntroductionCanBeSkipped(t *testing.T) {
	p := startPhone(t, "")
	p.app.WaitFor("id:onboarding-intro", wait)
	p.app.Press("id:onboarding-skip")
	p.app.WaitFor("id:films-unpaired", wait)
	var prefs store.Prefs
	p.app.WaitUntil(wait, func() bool { prefs, _ = store.Open(p.dir).Prefs(); return prefs.Onboarded })
	// Nothing was chosen, so the target follows the phone's language rather than the one it had when skipped.
	if prefs.Target != "" || len(prefs.Spoken) > 0 {
		t.Fatalf("skipping kept choices the user did not make: %+v", prefs)
	}
	p = startPhone(t, p.dir)
	p.app.WaitFor("id:films-unpaired", wait)
	if p.app.Exists("id:onboarding") {
		t.Fatal("the introduction showed again after it was skipped")
	}
}

// Unpaired in Settings while the Library shows one of its films not on the phone, a computer takes that page off the Library's stack, and its films off the list: with no client left they could not be downloaded.
func TestUnpairingInSettingsClosesTheComputersFilms(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	p.app.WaitFor(talkRow(id), wait)
	p.app.Press(talkRow(id))
	p.app.WaitFor("id:film-download", wait)
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.Press("id:settings-computer-options-" + id)
	p.app.Answer(1)
	p.app.WaitForGone("id:settings-computer-"+id, wait)
	p.openTab(filmsTab, msgs.FilmsTitle())
	p.app.WaitFor("id:films-unpaired", wait)
	if p.app.Exists("id:film-page") || p.app.Exists(talkRow(id)) {
		t.Fatal("a film of a computer unpaired stayed on the Library's stack or in its list")
	}
}

// The languages chosen in the introduction are kept as they are chosen: a phone closed before the introduction ends keeps them, and so does an introduction shown again from Settings and left by the back gesture.
func TestTheIntroductionKeepsTheLanguagesHoweverItIsLeft(t *testing.T) {
	p := startPhone(t, "")
	p.app.WaitFor("id:onboarding-intro", wait)
	p.app.Press("id:onboarding-continue")
	p.app.WaitFor("id:onboarding-languages", wait)
	p.app.Press("id:onboarding-spoken-ja")
	p.app.Press("id:onboarding-target-ko")
	var prefs store.Prefs
	p.app.WaitUntil(wait, func() bool { prefs, _ = store.Open(p.dir).Prefs(); return prefs.Target == "ko" })
	if !slices.Equal(prefs.Spoken, []string{"ja"}) || prefs.Onboarded {
		t.Fatalf("kept %+v before the introduction ended", prefs)
	}

	dir := t.TempDir()
	keepTalk(t, dir)
	p = startPhone(t, dir)
	p.app.WaitFor(talkRow("gonedesk"), wait)
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.Press("id:settings-introduction")
	p.app.WaitFor("id:onboarding-intro", wait)
	p.app.Press("id:onboarding-continue")
	p.app.WaitFor("id:onboarding-languages", wait)
	p.app.Press("id:onboarding-target-ko")
	p.app.Back()
	p.app.WaitForGone("id:onboarding", wait)
	p.app.WaitUntil(wait, func() bool { prefs, _ = store.Open(p.dir).Prefs(); return prefs.Target == "ko" })
	if got := p.texts("id:settings-target"); !strings.Contains(got, "한국어") {
		t.Fatalf("Settings shows the target as %q after the introduction was left by the back gesture", got)
	}
}

// Shown again from Settings, the introduction names the computer it paired with, which may be one paired already rather than the last one paired.
func TestTheIntroductionNamesTheComputerPairedAgain(t *testing.T) {
	studio := newDesktop(t)
	dir := t.TempDir()
	secrets := pairedWith(t, studio, dir)
	// Another computer paired after the Studio; the test desktops share a certificate, so it is kept as paired rather than paired over the protocol.
	st := store.Open(dir)
	list, err := st.Desktops()
	if err != nil {
		t.Fatal(err)
	}
	list = append(list, store.Desktop{ID: "laptop", Name: "Laptop", Addr: studio.Addr, Fingerprint: []byte("laptop"), Paired: now()})
	if err := st.SaveDesktops(list); err != nil {
		t.Fatal(err)
	}
	p := startPhone(t, dir, secrets)
	p.app.WaitFor("id:computers-problem-laptop", wait)
	p.openTab(settingsTab, msgs.SettingsTitle())
	studio.Offer("123456")
	p.app.Press("id:settings-introduction")
	p.startPairing()
	p.fillPairing(studio.Addr.String(), "123456")
	p.app.Press("id:pair-continue")
	p.app.WaitFor("id:pair-confirm", wait)
	p.app.Press("id:pair-confirm")
	p.app.WaitFor("id:onboarding-paired", wait)
	if got, want := p.texts("id:onboarding-paired"), msgs.OnboardingPaired("Studio"); got != want {
		t.Fatalf("pairing the Studio again says %q, want %q", got, want)
	}
}

// The introduction keeps the languages the user watches and the one they want subtitles in, which Settings shows and changes; left at its pairing step it goes to the Library unpaired.
func TestTheIntroductionKeepsTheLanguages(t *testing.T) {
	p := startPhone(t, "")
	p.app.WaitFor("id:onboarding-intro", wait)
	p.app.Press("id:onboarding-continue")
	p.app.WaitFor("id:onboarding-languages", wait)
	// The phone's own language is the target to start with.
	if !p.app.Find("id:onboarding-target-en").Props.Checked {
		t.Fatal("the phone's language is not the target the introduction starts with")
	}
	p.app.Press("id:onboarding-spoken-ja")
	p.app.Press("id:onboarding-spoken-fr")
	p.app.Press("id:onboarding-spoken-fr")
	p.app.Press("id:onboarding-spoken-en")
	p.app.Press("id:onboarding-target-ko")
	p.app.Press("id:onboarding-continue")
	p.app.WaitFor("id:onboarding-pairing", wait)
	p.app.Press("id:onboarding-later")
	p.app.WaitFor("id:films-unpaired", wait)
	var prefs store.Prefs
	p.app.WaitUntil(wait, func() bool { prefs, _ = store.Open(p.dir).Prefs(); return prefs.Onboarded })
	if !slices.Equal(prefs.Spoken, []string{"ja", "en"}) || prefs.Target != "ko" {
		t.Fatalf("kept %+v", prefs)
	}

	p.openTab(settingsTab, msgs.SettingsTitle())
	if got := p.texts("id:settings-target"); !strings.Contains(got, "한국어") {
		t.Fatalf("Settings shows the target as %q", got)
	}
	p.app.Press("id:settings-target")
	p.app.WaitFor("id:settings-target-page", wait)
	if !p.app.Find("id:settings-target-ko").Props.Checked || p.app.Find("id:settings-target-zh-Hans").Props.Checked {
		t.Fatal("the target page does not check the target alone")
	}
	p.app.Press("id:settings-target-zh-Hans")
	p.app.WaitUntil(wait, func() bool { prefs, _ = store.Open(p.dir).Prefs(); return prefs.Target == "zh-Hans" })
	if !slices.Equal(prefs.Spoken, []string{"ja", "en"}) || !prefs.Onboarded {
		t.Fatalf("changing the target lost the other choices: %+v", prefs)
	}
}

// Settings shows the introduction again, over the tabs, which it leaves as they were.
func TestSettingsShowsTheIntroductionAgain(t *testing.T) {
	dir := t.TempDir()
	keepTalk(t, dir)
	p := startPhone(t, dir)
	p.app.WaitFor(talkRow("gonedesk"), wait)
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.Press("id:settings-introduction")
	p.app.WaitFor("id:onboarding-intro", wait)
	p.app.Press("id:onboarding-skip")
	p.app.WaitForGone("id:onboarding", wait)
	p.app.WaitFor("id:settings-page", wait)
}

// On iOS a downloaded film's page puts its delete button in the system header, and Settings opens pairing.
func TestTheIOSHeaders(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	keepTalk(t, dir)
	p := startPhone(t, dir, pairedWith(t, d, dir), nsgotest.Environment(core.Environment{Locale: lang, OS: core.OSIOS}))
	p.app.WaitFor(talkRow("gonedesk"), wait)
	p.app.Press(talkRow("gonedesk"))
	p.app.WaitFor("id:film-delete", wait)
	p.app.HeaderBack("id:film-page")
	p.app.WaitForGone("id:film-page", wait)
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.Press("id:settings-pair")
	p.app.WaitFor("id:pair-page", wait)
}

// Over a tab bar the app draws, which keeps clear of the bottom safe area itself, a film's actions sit just above it rather than a safe area higher.
func TestTheActionBarSitsOnTheTabBar(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("the safe area is the platform's on a target")
	}
	const bottom = 34
	dir := t.TempDir()
	keepTalk(t, dir)
	p := startPhone(t, dir, nsgotest.Environment(core.Environment{Locale: lang, SafeArea: core.Edges{Bottom: bottom}}))
	p.app.WaitFor(talkRow("gonedesk"), wait)
	p.app.Press(talkRow("gonedesk"))
	play := p.app.WaitFor("id:film-play", wait).Bounds()
	bar := p.app.Find("id:tab-bar").Bounds()
	if gap := bar.Origin.Y - (play.Origin.Y + play.Size.Height); gap < 0 || gap >= bottom {
		t.Fatalf("the action bar's button ends %v above the tab bar, want less than the %v of the safe area the bar keeps clear of", gap, core.Points(bottom))
	}
}

// A tablet on its side shows the sections in a sidebar, and upright in a tab bar, keeping the pages each section opened as it turns; it asks to unpair in a dialog rather than an action sheet.
func TestATabletShowsASidebarOnItsSideAndATabBarUpright(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("the idiom is the platform's on a target")
	}
	d := newDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir), nsgotest.Size(1180, 820), nsgotest.Environment(core.Environment{Locale: lang, Idiom: core.IdiomTablet}))
	p.app.WaitFor(talkRow(store.DesktopID(d.Fingerprint)), wait)
	if !p.app.Exists("id:sidebar") || p.app.Exists("id:tab-bar") {
		t.Fatal("a tablet on its side shows no sidebar, or a tab bar")
	}
	if got := p.texts("id:sidebar-computers"); got != "Studio" {
		t.Fatalf("the sidebar's foot shows %q, want the computer paired", got)
	}
	p.app.Press("id:sidebar-tab-settings")
	p.app.WaitFor("id:settings-page", wait)
	p.app.Press("id:settings-target")
	p.app.WaitFor("id:settings-target-page", wait)
	sidebar, page := p.app.Find("id:sidebar").Frame, p.app.Find("id:settings-target-page").Bounds()
	if page.Origin.X < sidebar.Size.Width {
		t.Fatalf("the page at %v lies under the sidebar %v", page, sidebar)
	}

	p.app.Resize(820, 1180)
	p.app.WaitFor("id:tab-bar", wait)
	if p.app.Exists("id:sidebar") {
		t.Fatal("an upright tablet still shows the sidebar")
	}
	if !p.app.Exists("id:settings-target-page") {
		t.Fatal("turning the tablet lost the page Settings opened")
	}
	p.openTab(filmsTab, msgs.FilmsTitle())
	p.app.WaitFor(talkRow(store.DesktopID(d.Fingerprint)), wait)
	p.app.Resize(1180, 820)
	p.app.WaitFor("id:sidebar", wait)
	p.app.Press("id:sidebar-computers")
	p.app.WaitFor("id:settings-target-page", wait)
	p.app.Press("id:sidebar-tab-settings")
	p.app.WaitFor("id:settings-page", wait)
	p.app.Press("id:settings-computer-options-" + store.DesktopID(d.Fingerprint))
	if alerts := p.app.Alerts(); len(alerts) == 0 || alerts[len(alerts)-1].Sheet {
		t.Fatal("a tablet asks to unpair in an action sheet, not a dialog")
	}
	p.app.Answer(0)
}

// Unpaired in Settings while the Library shows the page of one of its films kept on the phone, a computer leaves that page open: the film still plays from the phone.
func TestUnpairingInSettingsKeepsThePageOfAFilmOnThePhone(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	id := store.DesktopID(d.Fingerprint)
	keepTalkFrom(t, dir, id)
	p := startPhone(t, dir, secrets)
	p.app.WaitFor(talkRow(id), wait)
	p.app.Press(talkRow(id))
	p.app.WaitFor("id:film-play", wait)
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.Press("id:settings-computer-options-" + id)
	p.app.Answer(1)
	p.app.WaitForGone("id:settings-computer-"+id, wait)
	p.openTab(filmsTab, msgs.FilmsTitle())
	p.app.WaitFor("id:film-play", wait)
	p.app.HeaderBack("id:film-page")
	p.app.WaitFor("id:films-unpaired", wait)
	p.app.Find(talkRow(id))
}
