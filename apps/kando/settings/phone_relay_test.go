package settings

import (
	"slices"
	"testing"

	"kando/apps/kando/locales/testlocale"
)

// 经你的存储同步 turns the storage relay on, names the storage it goes through with 去设置… to 账号与上传, and while it is on says whether it works; revoking a phone then says what becomes of its messages and links in the storage.
func TestPhonePageShowsTheStorageRelay(t *testing.T) {
	a := newFakeApp()
	a.section = SectionPhone
	a.phone = PhoneSync{On: true, Running: true, Addrs: []string{"192.168.1.20:52817"}, Devices: []PhoneDevice{{ID: "d1", Name: "iPhone 17", Platform: "ios"}}}
	app := start(t, a)
	msgs := testlocale.Messages()
	if !app.Exists(whole(msgs.SettingsPhoneRelayNoStore())) || app.Exists("id:settings-phone-relay-status") {
		t.Fatalf("with the relay off and no storage: texts %q", app.Texts())
	}
	app.Press("id:settings-phone-relay")
	if !slices.Contains(a.calls, "relay true") {
		t.Fatalf("calls %q after the switch", a.calls)
	}
	change(app, a, func() {})
	if !app.Exists("id:settings-phone-relay-status >> " + whole(msgs.SettingsPhoneRelayNeedsStore())) {
		t.Fatalf("with no storage: texts %q", app.Texts())
	}
	for _, c := range []struct {
		relay PhoneRelay
		want  string
	}{
		{PhoneRelay{On: true, Store: "R2 · films"}, msgs.SettingsPhoneRelayChecking()},
		{PhoneRelay{On: true, Store: "R2 · films", Working: true}, msgs.SettingsPhoneRelayWorking()},
		{PhoneRelay{On: true, Store: "R2 · films", Working: true, Err: "The storage does not take the access key"}, "The storage does not take the access key"},
		{PhoneRelay{On: true, Store: "R2 · films", Offline: true}, msgs.SettingsPhoneRelayOffline()},
	} {
		change(app, a, func() { a.phone.Relay = c.relay })
		if !app.Exists("id:settings-phone-relay-status >> "+whole(c.want)) || app.Find("id:settings-phone-relay-store-name").Text != "R2 · films" {
			t.Fatalf("relay %+v: texts %q", c.relay, app.Texts())
		}
	}
	app.Press("id:settings-phone-revoke-d1")
	if !app.Exists(whole(msgs.SettingsPhoneRevokeStorage())) {
		t.Fatalf("revoking with the relay on does not say what becomes of the storage: %q", app.Texts())
	}
	app.Press("id:settings-phone-revoke-cancel-d1")
	app.Press("id:settings-phone-relay-set-up")
	if a.section != SectionAccount {
		t.Fatalf("去设置… shows section %q", a.section)
	}
}

// While the relay works, 存储用量 says what the films and messages take in the storage and which video goes up; 只同步字幕 keeps the videos off it; the watched videos can be cleaned up one by one or all at once, and 换一个存储 asks before it moves to another storage, then says how far the move went.
func TestPhonePageShowsTheStorageUsage(t *testing.T) {
	a := newFakeApp()
	a.section = SectionPhone
	a.phone = PhoneSync{On: true, Running: true, Addrs: []string{"192.168.1.20:52817"}, Relay: PhoneRelay{On: true, Store: "R2 · films", Working: true, Storage: PhoneStorage{
		Measured: true, Videos: 3 << 30, Lines: 2 << 20, Mail: 20 << 10, Films: 3, WithVideo: 2, Uploading: "Morning Talk", Percent: 42, Waiting: 2,
		Watched: []PhoneWatched{{Library: "lib", ID: "f1", Title: "Rivers", Size: 400 << 20}, {Library: "lib", ID: "f2", Title: "Tides", Size: 100 << 20}},
		Targets: []PhoneStore{{Kind: "s3", Name: "S3"}},
	}}}
	app := start(t, a)
	msgs := testlocale.Messages()
	for sel, want := range map[string]string{
		"id:settings-phone-storage-where":     "R2 · films",
		"id:settings-phone-storage-total":     "3.0 GB",
		"id:settings-phone-storage-count":     msgs.SettingsPhoneStorageFilms(3, 2),
		"id:settings-phone-storage-videos":    msgs.SettingsPhoneStorageVideos("3.0 GB"),
		"id:settings-phone-storage-mail":      msgs.SettingsPhoneStorageMail("20 KB"),
		"id:settings-phone-storage-uploading": msgs.SettingsPhoneStorageUploadingMore("Morning Talk", 42, 1),
	} {
		if got := app.Find(sel).Text; got != want {
			t.Errorf("%s says %q, want %q", sel, got, want)
		}
	}
	if !app.Exists("id:settings-phone-storage-watched-f1") || app.Exists("id:settings-phone-storage-none") {
		t.Fatalf("the watched videos: %q", app.Texts())
	}
	app.Press("id:settings-phone-storage-clean-f1")
	app.Press("id:settings-phone-storage-clean-all")
	app.Press("id:settings-phone-relay-subtitles-only")
	if want := []string{"clean f1", "clean f1 f2", "subtitles only true"}; !slices.Equal(a.calls[len(a.calls)-3:], want) {
		t.Fatalf("calls %q, want %q at the end", a.calls, want)
	}
	change(app, a, func() { a.phone.Relay.Storage.Watched = nil })
	if !app.Exists("id:settings-phone-storage-none") {
		t.Fatalf("with nothing to clean up: %q", app.Texts())
	}

	app.Press("id:settings-phone-storage-move")
	if !app.Exists("id:settings-phone-storage-move-ask >> " + whole(msgs.SettingsPhoneStorageMoveAsk("S3"))) {
		t.Fatalf("迁移… does not ask: %q", app.Texts())
	}
	app.Press("id:settings-phone-storage-move-confirm")
	if got := a.calls[len(a.calls)-1]; got != "move s3" {
		t.Fatalf("the move called %q", got)
	}
	change(app, a, func() {
		a.phone.Relay.Storage.Moving, a.phone.Relay.Storage.MoveTo, a.phone.Relay.Storage.MoveFrom, a.phone.Relay.Storage.Done, a.phone.Relay.Storage.Total = true, "S3", "R2 · films", 1, 2
	})
	if got, want := app.Find("id:settings-phone-storage-move-status").Text, msgs.SettingsPhoneStorageMovingCount("S3", 1, 2); got != want || !app.Exists("id:settings-phone-storage-move-progress") {
		t.Fatalf("moving says %q, want %q", got, want)
	}
	change(app, a, func() { a.phone.Relay.Storage.Moving, a.phone.Relay.Storage.Moved = false, true })
	if got, want := app.Find("id:settings-phone-storage-move-status").Text, msgs.SettingsPhoneStorageMoved("S3", "R2 · films"); got != want {
		t.Fatalf("moved says %q, want %q", got, want)
	}

	change(app, a, func() { a.phone.Relay.On = false })
	if app.Exists("id:settings-phone-storage-total") || app.Exists("id:settings-phone-relay-subtitles-only") {
		t.Fatalf("with the relay off: %q", app.Texts())
	}
}
