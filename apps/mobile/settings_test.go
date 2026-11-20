package main

import (
	"strings"
	"testing"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/store"
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

// Settings › Offline Films lists the films saved on the device, and Delete, once confirmed, deletes one from it.
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
