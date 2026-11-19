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
