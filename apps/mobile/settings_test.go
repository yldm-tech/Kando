package main

import (
	"strings"
	"testing"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/store"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// Settings › Language shows the interface in the language picked there at once, keeps it, and follows the system's again with Automatic.
func TestTheInterfaceLanguageIsTheOneChosenInSettings(t *testing.T) {
	dir := t.TempDir()
	keepTalk(t, dir)
	p := startPhone(t, dir)
	p.app.WaitFor(talkRow("gonedesk"), wait)
	p.openTab(settingsTab, msgs.SettingsTitle())
	if got, want := p.texts("id:settings-language"), msgs.SettingsLanguage()+"\n"+msgs.SettingsLanguageSystem(); got != want {
		t.Fatalf("the Language row reads %q, want %q", got, want)
	}
	p.app.Press("id:settings-language")
	p.app.WaitFor("id:settings-language-page", wait)
	p.app.Press("id:settings-language-en")
	en := locales.In("en")
	p.app.WaitUntil(wait, func() bool { return p.app.Find("id:settings-language-en").Props.Checked })
	p.app.WaitFor("text:"+en.SettingsLanguageSystem(), wait)
	p.app.WaitUntil(wait, func() bool { prefs, _ := store.Open(dir).Prefs(); return prefs.Language == "en" })
	// The Settings page under the Language page follows too, its switches' spoken names with it.
	p.app.Back()
	p.app.WaitFor("id:settings-page", wait)
	p.app.WaitUntil(wait, func() bool { return p.app.Find("id:settings-remind-switch").Props.Label == en.SettingsRemind() })
	p.app.Press("id:settings-language")
	p.app.WaitFor("id:settings-language-page", wait)

	p.app.Press("id:settings-language-system")
	p.app.WaitFor("text:"+msgs.SettingsLanguageSystem(), wait)
	p.app.WaitUntil(wait, func() bool { prefs, _ := store.Open(dir).Prefs(); return prefs.Language == "" })
}

// What another tab shows follows a change of the interface language where it stands, built once: the tab bar's name and its tabs', and Learn's review modes and a saved line's remove action, which the screen reader reads from bindings.
func TestOtherTabsFollowTheInterfaceLanguage(t *testing.T) {
	d := studyDesktop(t)
	dir, f := keptFrom(t, d)
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.waitLearned(f.Desktop)
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitFor("id:learn-line-l-fine", wait)
	p.app.WaitFor("id:learn-review-mode", wait)
	// mounted is the one view of selector mounted, shown or in a tab that is not.
	mounted := func(selector string) *nsgotest.View {
		views := p.app.QueryMounted(selector)
		if len(views) != 1 {
			t.Fatalf("%d views %s are mounted, want 1", len(views), selector)
		}
		return views[0]
	}
	line := mounted("id:learn-line-l-fine")

	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.Press("id:settings-language")
	p.app.WaitFor("id:settings-language-page", wait)
	p.app.Press("id:settings-language-zh-Hans")
	zh := locales.In("zh-Hans")
	p.app.WaitUntil(wait, func() bool { return mounted("id:tab-bar").Props.Label == zh.AppName() })
	if got, want := mounted("id:bar-tab-learn").Props.Label, zh.LearnTitle(); got != want {
		t.Errorf("Learn's tab is read as %q, want %q", got, want)
	}
	if got, want := mounted("id:learn-review-mode").Props.Label, zh.LearnReviewMode(); got != want {
		t.Errorf("the review modes are read as %q, want %q", got, want)
	}
	if got, want := mounted("id:learn-review-cards").Props.Label, zh.LearnReviewCards(); got != want {
		t.Errorf("the cards mode is read as %q, want %q", got, want)
	}
	if got := mounted("id:learn-line-l-fine").Props.ActionList(); len(got) != 1 || got[0].Label != zh.LearnRemove() {
		t.Errorf("the saved line offers %+v, want its remove action read as %q", got, zh.LearnRemove())
	}
	if mounted("id:learn-line-l-fine") != line {
		t.Error("the saved line's card was built again")
	}
}

// Settings › Offline Films lists the films saved on the device, and Delete, once confirmed, deletes one from it. Where the system does not say the room on the device, the page shows what the films take alone.
func TestOfflineFilmsListsAndDeletesTheSavedFilms(t *testing.T) {
	dir := t.TempDir()
	f := keepTalk(t, dir)
	p := startPhone(t, dir)
	p.app.WaitFor(talkRow("gonedesk"), wait)
	p.openTab(settingsTab, msgs.SettingsTitle())
	if got, want := p.texts("id:settings-offline"), msgs.SettingsOffline()+"\n"+msgs.SettingsOfflineCount(1); got != want {
		t.Fatalf("the Offline Films row reads %q, want %q", got, want)
	}
	p.app.Press("id:settings-offline")
	p.app.WaitFor("id:offline-film-"+f.Key(), wait)
	if got := p.texts("id:offline-film-" + f.Key()); !strings.Contains(got, msgs.SettingsOfflineFilmSaved()) {
		t.Fatalf("the saved film's row reads %q, without %q", got, msgs.SettingsOfflineFilmSaved())
	}
	if p.app.Exists("id:offline-space-bar") || p.app.Exists("id:offline-space-free") {
		t.Fatal("the page shows the room on a device that does not say it")
	}
	// Delete is named for what it does, not by the question it asks.
	if got, want := p.app.Find("id:offline-delete-"+f.Key()).Props.Label, msgs.SettingsOfflineDelete(f.Film.Title); got != want {
		t.Fatalf("Delete is read as %q, want %q", got, want)
	}
	p.app.Press("id:offline-delete-" + f.Key())
	p.app.Answer(0)
	if !p.app.Exists("id:offline-film-" + f.Key()) {
		t.Fatal("cancelling deleted the film")
	}
	p.app.Press("id:offline-delete-" + f.Key())
	p.app.Answer(1)
	p.app.WaitFor("id:offline-empty", wait)
	p.app.WaitUntil(wait, func() bool { films, _ := store.Open(dir).Films(); return len(films) == 0 })
}

// Settings › Offline Films shows the films' share of the device on a bar, beside what the rest of the device holds, and about how much of it is free, as the system says.
func TestOfflineFilmsShowTheirShareOfTheDevice(t *testing.T) {
	dir := t.TempDir()
	f := keepTalk(t, dir)
	p := startPhone(t, dir, nsgotest.DiskSpace(core.DiskSpace{Total: 128 << 30, Available: 38<<30 + 300<<20, Opportunistic: 30 << 30}, nil))
	p.app.WaitFor(talkRow("gonedesk"), wait)
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.Press("id:settings-offline")
	p.app.WaitFor("id:offline-space-free", wait)
	if got, want := p.texts("id:offline-space-free"), msgs.SettingsOfflineFree("38 GB"); got != want {
		t.Errorf("the page says %q, want %q", got, want)
	}
	for _, part := range []string{"id:offline-space-films", "id:offline-space-others"} {
		if !p.app.Exists(part) {
			t.Errorf("the bar has no %s", part)
		}
	}
	if !p.app.Find("id:offline-space-bar").Props.ElementsHidden {
		t.Error("assistive technology reads the bar, which the texts say")
	}

	// With the film deleted, the films take no room, and their share leaves the bar.
	p.app.Press("id:offline-delete-" + f.Key())
	p.app.Answer(1)
	p.app.WaitFor("id:offline-empty", wait)
	p.app.WaitForGone("id:offline-space-films", wait)
	if !p.app.Exists("id:offline-space-others") {
		t.Error("the rest of the device left the bar")
	}
}

// The subtitle size control shows the sizes' short names and says their full names to assistive technology.
func TestTheSubtitleSizeControlSaysTheSizesInFull(t *testing.T) {
	dir := t.TempDir()
	keepTalk(t, dir)
	p := startPhone(t, dir)
	p.app.WaitFor(talkRow("gonedesk"), wait)
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.Press("id:settings-style")
	p.app.WaitFor("id:style-page", wait)
	if got, want := p.app.Find("id:style-size-small").Props.Label, msgs.StyleSmall(); got != want {
		t.Fatalf("the small size is read as %q, want %q", got, want)
	}
	if got, want := p.texts("id:style-size-small"), msgs.StyleSizeShortSmall(); got != want {
		t.Fatalf("the small size shows %q, want %q", got, want)
	}
}
