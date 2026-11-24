package main

import (
	"bytes"
	"crypto/ed25519"
	"strings"
	"testing"

	"kando/apps/kando/account"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings"
	"kando/apps/kando/storage"

	"golang.org/x/crypto/ssh"
)

// TestScreenshotsStorageLocations renders the 存储位置 card at the end of 账号与上传 and what it opens over the settings window: three locations, one connected, one testing and one whose password the server refused (settings-account-locations), and the same card when the system's secure store did not take their secrets, which the settings file keeps instead (settings-account-locations-in-file); the dialog adding an SFTP location whose server key was trusted (settings-account-location-dialog); the question at the first connection to an SFTP server (settings-account-location-trust), and the warning about a key that changed, with Replace Key (settings-account-location-key-changed). The servers are names in the scene, never reached, and the keys are made from fixed seeds, so the fingerprints stay the same from run to run.
func TestScreenshotsStorageLocations(t *testing.T) {
	f := newScreenshotFixture(t)
	shown, models, turbo := settingsShotFolders(t, f)
	shootSettingsScrolled(t, f, settings.SectionAccount, "settings-account-locations", shown, models, turbo, shotLocations)
	shootSettingsScrolled(t, f, settings.SectionAccount, "settings-account-locations-in-file", shown, models, turbo, func(a *app) {
		shotLocations(a)
		a.locations.plain["nas"] = true
	})
	shootSettings(t, f, settings.SectionAccount, "settings-account-location-dialog", shown, models, turbo, func(a *app) {
		shotLocations(a)
		a.openLocationDialog("", account.Draft{Name: "Studio", Kind: string(storage.KindSFTP), Host: "studio.lan", User: "reader", Password: "secret", Root: "films", HostKey: shotHostKey(1)})
		a.locations.tests[""] = locationTest{ok: true}
	})
	shootSettings(t, f, settings.SectionAccount, "settings-account-location-trust", shown, models, turbo, func(a *app) {
		shotLocations(a)
		key := shotHostKey(1)
		a.askLocation(locationAlert{kind: account.AlertTrust, target: "studio", hk: &storage.HostKeyError{Host: "studio.lan", Key: key, Fingerprint: storage.Fingerprint(key)}})
	})
	shootSettings(t, f, settings.SectionAccount, "settings-account-location-key-changed", shown, models, turbo, func(a *app) {
		shotLocations(a)
		key, old := shotHostKey(2), shotHostKey(1)
		a.askLocation(locationAlert{kind: account.AlertKeyChanged, target: "studio", hk: &storage.HostKeyError{Host: "studio.lan", Key: key, Fingerprint: storage.Fingerprint(key), Trusted: old}})
	})
}

// shotLocations sets up three storage locations, one of each kind, as their Tests left them.
func shotLocations(a *app) {
	a.locations.list = []account.Location{
		{ID: "nas", Name: "Home NAS", Remote: storage.Remote{Kind: storage.KindSMB, Host: "nas.local", Share: "media", Root: "films", User: "reader", Password: "secret"}},
		{ID: "dav", Name: "Nextcloud", Remote: storage.Remote{Kind: storage.KindWebDAV, URL: "https://cloud.example.com/remote.php/dav/files/reader", Root: "Kando", User: "reader", Password: "secret"}},
		{ID: "studio", Name: "Studio", Remote: storage.Remote{Kind: storage.KindSFTP, Host: "studio.lan", Root: "films", User: "reader", Password: "secret", HostKey: shotHostKey(1)}},
	}
	a.locations.tests["nas"] = locationTest{ok: true}
	a.locations.tests["dav"] = locationTest{busy: true}
	a.locations.tests["studio"] = locationTest{err: locales.Messages.AccountLocationFailSignIn}
}

// shotHostKey is an ed25519 host key made from a seed of n, as a line of authorized_keys.
func shotHostKey(n byte) string {
	signer, err := ssh.NewSignerFromKey(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{n}, ed25519.SeedSize)))
	if err != nil {
		panic(err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
}
