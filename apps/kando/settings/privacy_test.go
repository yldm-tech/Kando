package settings

import (
	"slices"
	"testing"

	"kando/apps/kando/locales/testlocale"
)

func TestPrivacyPageSaysWhatUsesTheNetwork(t *testing.T) {
	a := newFakeApp()
	a.section = SectionPrivacy
	app := start(t, a)
	msgs := testlocale.Messages()
	tag := func(id string) string { return app.Find("id:settings-privacy-tag-" + id).Text }
	// Online with a cloud AI service: downloads go over the network, the translation sends the subtitles to EveryAPI, speech stays here.
	if tag("download") != msgs.SettingsPrivacyTagNetwork() || tag("stream") != msgs.SettingsPrivacyTagNetwork() || tag("speech") != msgs.SettingsPrivacyTagLocal() || tag("ai") != msgs.SettingsPrivacyTagCloud() || tag("phone") != msgs.SettingsPrivacyTagLAN() {
		t.Fatalf("tags online: %q", app.Texts())
	}
	if !app.Exists(whole(msgs.SettingsPrivacyStepAICloud("EveryAPI"))) {
		t.Fatalf("the AI row does not name the cloud service: %q", app.Texts())
	}
	app.Press("id:settings-privacy-offline")
	if !slices.Contains(a.calls, "offline true") {
		t.Fatalf("calls %q", a.calls)
	}
	app.Settle()
	// Offline: every step over the internet is off, the update row says the daily check goes on, and Phone Sync stays on the local network.
	for _, id := range []string{"download", "ai", "upload", "stream", "tools", "browse"} {
		if tag(id) != msgs.SettingsPrivacyTagOff() {
			t.Errorf("tag of %s offline = %q", id, tag(id))
		}
	}
	if tag("speech") != msgs.SettingsPrivacyTagLocal() || tag("phone") != msgs.SettingsPrivacyTagLAN() || !app.Exists(whole(msgs.SettingsPrivacyStepUpdatesOffline())) {
		t.Fatalf("texts offline: %q", app.Texts())
	}
	// A service on this computer goes on offline, and the row says it runs here.
	change(app, a, func() { a.provider, a.aiLocal = "ollama", true })
	if tag("ai") != msgs.SettingsPrivacyTagLocal() || !app.Exists(whole(msgs.SettingsPrivacyStepAILocal("Ollama"))) {
		t.Fatalf("local AI offline: tag %q, texts %q", tag("ai"), app.Texts())
	}
}

func TestPrivacyPageClearsTheCache(t *testing.T) {
	a := newFakeApp()
	a.section = SectionPrivacy
	app := start(t, a)
	msgs := testlocale.Messages()
	if !app.Exists(whole(msgs.SettingsPrivacyCacheHint())) {
		t.Fatalf("texts %q", app.Texts())
	}
	app.Press("id:settings-privacy-clear-cache")
	app.Settle()
	if !slices.Contains(a.calls, "clear cache") || !app.Exists(whole(msgs.SettingsPrivacyClearing())) {
		t.Fatalf("calls %q, texts %q", a.calls, app.Texts())
	}
	change(app, a, func() { a.cacheClear = CacheClear{Done: true, Freed: 3 << 20} })
	if !app.Exists(whole(msgs.SettingsPrivacyCleared("3.0 MB"))) {
		t.Fatalf("texts %q after clearing", app.Texts())
	}
	change(app, a, func() { a.cacheClear = CacheClear{Done: true} })
	if !app.Exists(whole(msgs.SettingsPrivacyClearedNothing())) {
		t.Fatalf("texts %q after clearing nothing", app.Texts())
	}
}

func TestByteSize(t *testing.T) {
	for n, want := range map[int64]string{1: "1 KB", 2048: "2 KB", 5 << 20: "5.0 MB", 3 << 30: "3.0 GB"} {
		if got := byteSize(n); got != want {
			t.Errorf("byteSize(%d) = %q, want %q", n, got, want)
		}
	}
}
