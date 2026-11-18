package main

import (
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"kando/apps/kando/cloud"
	"kando/apps/kando/phonesync"
	"kando/apps/kando/settings"
	psync "kando/packages/sync"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsPhoneSync renders what the desktop shows of the paired phones (screenshot_helpers_test.go): the library offering to continue The Tide Pools of Monterey (j8) where an iPhone left it at 2:05 ten minutes ago, the hero naming the iPhone (phone-handoff); Ocean Currents Explained (j6) paused on the iPhone, with 在这台电脑上继续 (queue-paused-on-phone); and Settings › 手机同步 with an iPhone online, and an iPad made read-only (settings-phone-devices), and further down the learning records synced, a translation and a note in conflict, and what the phones did (settings-phone-conflicts); and the storage relay on through an R2 bucket that answers (settings-phone-relay). No server starts: the scenes set what a running server and the ledger of the open library would hold.
func TestScreenshotsPhoneSync(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "phone-handoff", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		shotPhoneLinked(a, f.job(t, "j8").VideoPath)
		refresh(tt, a)
		if !tt.Exists("id:films-handoff") {
			t.Fatalf("no offer to continue: %q", tt.Texts())
		}
	})
	f.shoot(t, "queue-paused-on-phone", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		job := f.job(t, "j6")
		job.PausedBy = "iPhone 15"
		a.library.Upsert(job)
		refresh(tt, a)
		if !tt.Exists("id:queue-continue-here") {
			t.Fatalf("no 在这台电脑上继续: %q", tt.Texts())
		}
	})
	shown, models, turbo := settingsShotFolders(t, f)
	shootSettings(t, f, settings.SectionPhone, "settings-phone-devices", shown, models, turbo, func(a *app) {
		shotPhoneSync(a)
		shotPhoneLinked(a, "")
	})
	shootSettingsScrolled(t, f, settings.SectionPhone, "settings-phone-conflicts", shown, models, turbo, func(a *app) {
		shotPhoneSync(a)
		shotPhoneLinked(a, "")
	})
	// 不在同一网络时 with the storage relay on, through an R2 bucket whose last turn went through.
	shootSettingsScrolled(t, f, settings.SectionPhone, "settings-phone-relay", shown, models, turbo, func(a *app) {
		shotPhoneSync(a)
		a.cloud.storage = cloud.KindR2
		a.cloud.r2Endpoint, a.cloud.r2Bucket = "https://0123456789abcdef.r2.cloudflarestorage.com", "kando-films"
		a.cloud.r2AccessKey, a.cloud.r2Secret = "access-key", "secret"
		a.phone.relay, a.phone.relayOK = true, true
	})
}

// shotPhoneLinked shows Phone Sync running with an iPhone 15 seen two minutes ago and an iPad Air, read-only, seen yesterday, and the open library's ledger as phones were told it: where the iPhone left the film whose video is at video ten minutes ago (none for ""), with the player resuming it there, a word, two saved lines, a translation the iPhone and this computer both changed and a note of a word the iPad and this computer both changed, as conflicts; and what the phones did lately.
func shotPhoneLinked(a *app, video string) {
	now := time.Now()
	a.phone.on, a.phone.running = true, true
	a.saved.device = "mac"
	a.phone.devices = []phonesync.Device{
		{ID: "iphone", Name: "iPhone 15", Platform: psync.PlatformIOS, Paired: now.Add(-72 * time.Hour), LastSeen: now.Add(-2 * time.Minute)},
		{ID: "ipad", Name: "iPad Air", Platform: psync.PlatformIOS, Paired: now.Add(-240 * time.Hour), LastSeen: now.Add(-20 * time.Hour), ReadOnly: true},
	}
	n := 0
	newID := func() string { n++; return "c" + strconv.Itoa(n) }
	field := func(v any, at time.Time, device string) psync.Field {
		return psync.Field{Value: psync.Value(v), Stamp: psync.Stamp{At: at, Device: device}}
	}
	l := psync.NewLedger()
	var films []phonesync.Film
	if video != "" {
		left := now.Add(-10 * time.Minute)
		films = append(films, phonesync.Film{ID: "j8", Title: "The Tide Pools of Monterey", Video: video})
		l.Put([]psync.Record{{Kind: psync.KindPosition, ID: "j8", Film: "j8", Fields: map[string]psync.Field{psync.FieldPositionMS: field(125000, left, "iphone")}}}, newID)
		a.player.positions[video] = 125 * time.Second
		if a.player.played == nil {
			a.player.played = map[string]time.Time{}
		}
		a.player.played[video] = left
	}
	films = append(films, phonesync.Film{ID: "j1", Title: "How Rivers Shape Landscapes"}, phonesync.Film{ID: "j3", Title: "Linear Algebra, Lecture 1"})
	desk, phone := now.Add(-3*time.Hour), now.Add(-2*time.Hour)
	l.Put([]psync.Record{
		{Kind: psync.KindPosition, ID: "j1", Film: "j1", Fields: map[string]psync.Field{psync.FieldPositionMS: field(100000, desk, "mac")}},
		{Kind: psync.KindLine, ID: "l1", Film: "j1", Fields: map[string]psync.Field{
			psync.FieldStartMS: field(52000, desk, "mac"), psync.FieldEndMS: field(55000, desk, "mac"),
			psync.FieldOriginal: field("That is how steep V-shaped valleys are formed.", desk, "mac"), psync.FieldTranslation: field("这就是陡峭的 V 形河谷的成因。", desk, "mac"),
		}},
		{Kind: psync.KindLine, ID: "l2", Film: "j3", Fields: map[string]psync.Field{psync.FieldStartMS: field(8000, desk, "mac"), psync.FieldOriginal: field("A basis spans the whole space.", desk, "mac")}},
		{Kind: psync.KindWord, ID: "w1", Fields: map[string]psync.Field{psync.FieldWord: field("basis vector", desk, "mac"), psync.FieldNote: field("basis = 坐标轴的方向，坐标就是沿它们走几步", desk, "mac")}},
	}, newID)
	l.Put([]psync.Record{
		{Kind: psync.KindLine, ID: "l1", Fields: map[string]psync.Field{psync.FieldTranslation: field("陡峭的 V 形谷就是这样切出来的。", phone, "iphone")}},
		{Kind: psync.KindWord, ID: "w1", Fields: map[string]psync.Field{psync.FieldNote: field("基向量：能张成整个空间、又不多余的一组向量", phone, "ipad")}},
	}, newID)
	if a.phone.ledgers == nil {
		a.phone.ledgers = map[string]*phoneLedger{}
	}
	a.phone.ledgers[filepath.Clean(a.snap().LibraryDir)] = &phoneLedger{ledger: l, kept: l.Rev(), films: films, listed: true}
	a.phone.activity = []settings.PhoneActivity{
		{At: now.Add(-2 * time.Minute), Device: "iPhone 15", Kind: settings.ActivityPaused, Title: "Ocean Currents Explained"},
		{At: now.Add(-10 * time.Minute), Device: "iPhone 15", Kind: settings.ActivityPosition, Title: "The Tide Pools of Monterey", Position: 125 * time.Second},
		{At: now.Add(-30 * time.Minute), Device: "iPad Air", Kind: settings.ActivityWords, Count: 2},
		{At: now.Add(-20 * time.Hour), Device: "", Kind: settings.ActivityReadOnly, Title: "iPad Air"},
	}
}
