package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	"kando/packages/sync"
	"kando/packages/sync/synctest"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestMain hands the main thread to the platform under nsgo test -target.
func TestMain(m *testing.M) { nsgotest.Main(m) }

// lang is the language the tests read the interface in, and msgs its messages.
const lang = "en"

var msgs = locales.In(lang)

// wait is how long a step waits for what a goroutine posts back: a request to the desktop on loopback, or the disk.
const wait = 10 * time.Second

// relayEvery is how often the tests' phones read a computer's state.json through its storage relay, away from its network.
const relayEvery = 50 * time.Millisecond

// now is the clock the tests' sessions date pairings and downloads by.
func now() time.Time { return time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC) }

// phone is the app under test with its data folder.
type phone struct {
	t   *testing.T
	app *nsgotest.App
	dir string
}

// startPhone starts the app on a phone of the default size, with its data in dir, "" for a new one.
func startPhone(t *testing.T, dir string, opts ...nsgotest.Option) *phone {
	t.Helper()
	if dir == "" {
		dir = t.TempDir()
	}
	opts = append([]nsgotest.Option{nsgotest.Environment(core.Environment{Locale: lang})}, opts...)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return App(cx, session.Config{Dir: dir, Device: "Test Phone", Platform: sync.PlatformIOS, Now: now, RelayEvery: relayEvery})
	}, opts...)
	return &phone{t: t, app: app, dir: dir}
}

// talk is the film the desktops of the tests serve: ten seconds of four lines, the third untranslated.
var talkCues = []sync.Cue{
	{StartMS: 1000, EndMS: 2000, Original: "Good morning.", Translation: "早上好。"},
	{StartMS: 3000, EndMS: 4500, Original: "How are you?", Translation: "你好吗？"},
	{StartMS: 5000, EndMS: 6500, Original: "Fine, thanks."},
	{StartMS: 7000, EndMS: 9000, Original: "See you.", Translation: "再见。"},
}

// talkVideo is the talk's video: a megabyte, so a download stalled partway shows a share of it.
var talkVideo = bytes.Repeat([]byte("0123456789"), 100_000)

// newDesktop is a desktop named Studio with a library of the talk, offering code 123456.
func newDesktop(t *testing.T) *synctest.Desktop {
	t.Helper()
	d := synctest.New(t, "Studio")
	d.AddLibrary("lib", "Japanese Class")
	d.AddFilm(synctest.Film{Library: "lib", Video: talkVideo, Poster: []byte("jpeg"), Cues: talkCues,
		Film: sync.Film{ID: "f1", Title: "Morning Talk", Source: "youtube", DurationMS: 10_000, Spoken: "ja", Translation: "简体中文", Updated: now()}})
	d.Offer("123456")
	return d
}

// openTab shows tab i (filmsTab, browseTab, learnTab, settingsTab), named name, pressing the drawn bar's tab, which headless always shows. Where the system's bar shows in its place, on an iOS target, it reports the change as the platform would, which nsgotest allows headless only: there it skips the test, so a screenshot scene must not depend on it.
func (p *phone) openTab(i int, name string) {
	p.t.Helper()
	// The system's bar is mounted but takes no room in the tree, so WaitFor, which sees only visible views, would never find it.
	p.app.WaitUntil(wait, func() bool { return len(p.app.QueryMounted("id:tab-bar")) == 1 })
	if p.app.Exists("id:tab-bar >> label:" + name) {
		p.app.Press("id:tab-bar >> label:" + name)
		return
	}
	// A phone's bar shows no Jobs, so a tab's place on it is not its number.
	p.app.Emit("id:tab-bar", "change", float64(slices.Index(shownTabs(false), i)))
}

// startPairing goes through a new phone's introduction to its pairing step.
func (p *phone) startPairing() {
	p.t.Helper()
	p.app.WaitFor("id:onboarding-intro", wait)
	p.app.Press("id:onboarding-continue")
	p.app.WaitFor("id:onboarding-languages", wait)
	p.app.Press("id:onboarding-continue")
	p.app.WaitFor("id:onboarding-pairing", wait)
}

// finishPairing leaves the introduction once the phone paired with d, at the Library, which lists its talk.
func (p *phone) finishPairing(d *synctest.Desktop) {
	p.t.Helper()
	p.app.WaitFor("id:onboarding-paired", wait)
	p.app.Press("id:onboarding-start")
	p.app.WaitFor(talkRow(store.DesktopID(d.Fingerprint)), wait)
}

// talkRow selects the talk's row in the Library, as listed by desktop or kept from it.
func talkRow(desktop string) string { return filmRow(desktop, "f1") }

// filmRow selects film's row in the Library, as listed by desktop or kept from it.
func filmRow(desktop, film string) string { return "id:films-film-" + desktop + "-" + film }

// pair pairs a new phone with d at the introduction's pairing step, comparing the fingerprint; paired, the phone shows the Library.
func (p *phone) pair(d *synctest.Desktop) {
	p.t.Helper()
	p.startPairing()
	p.fillPairing(d.Addr.String(), "123456")
	p.app.Press("id:pair-continue")
	got := p.app.WaitFor("id:pair-fingerprint", wait).Text
	if want := sync.ShortFingerprint(d.Fingerprint); got != want {
		p.t.Fatalf("the pairing page shows fingerprint %q, the desktop's is %q", got, want)
	}
	p.app.Press("id:pair-confirm")
	p.finishPairing(d)
}

// pairedWith keeps d in dir as paired, pairing over the protocol rather than through the pairing page, and returns the option that starts the phone with the token in its secure store.
func pairedWith(t *testing.T, d *synctest.Desktop, dir string) nsgotest.Option {
	t.Helper()
	c := sync.NewClient(d.Addr, d.Fingerprint, "")
	defer c.Close()
	resp, err := c.Pair(context.Background(), sync.PairRequest{Code: "123456", Device: "Test Phone", Platform: sync.PlatformIOS})
	if err != nil {
		t.Fatal(err)
	}
	desk := store.Desktop{ID: store.DesktopID(d.Fingerprint), Name: resp.Desktop, Addr: d.Addr, Fingerprint: d.Fingerprint, DeviceID: resp.DeviceID, Paired: now()}
	if err := store.Open(dir).SaveDesktops([]store.Desktop{desk}); err != nil {
		t.Fatal(err)
	}
	return nsgotest.Secrets(map[string]string{store.TokenAccount(desk.ID): resp.Token})
}

// fillPairing types the address and the code into the pairing page.
func (p *phone) fillPairing(address, code string) {
	p.t.Helper()
	p.app.WaitFor("id:pair-address", wait)
	p.app.Type("id:pair-address", address)
	p.app.Type("id:pair-code >> role:textbox", code)
}

// waitFile waits until the file at path, under the phone's data folder, exists and has size bytes, or with size -1 until it is gone.
func (p *phone) waitFile(path string, size int64) {
	p.t.Helper()
	p.app.WaitUntil(wait, func() bool {
		info, err := os.Stat(filepath.Join(p.dir, path))
		if size < 0 {
			return os.IsNotExist(err)
		}
		return err == nil && info.Size() == size
	})
}

// texts are the texts under the view selector names, joined by newlines.
func (p *phone) texts(selector string) string {
	p.t.Helper()
	var out []string
	var walk func(v *nsgotest.View)
	walk = func(v *nsgotest.View) {
		if v.Text != "" {
			out = append(out, v.Text)
		}
		for _, c := range v.Children {
			walk(c)
		}
	}
	walk(p.app.Find(selector))
	return strings.Join(out, "\n")
}

// keepTalk puts the talk on the phone as downloaded from a desktop that is gone, as a film to play offline: its record, its lines and a video file.
func keepTalk(t *testing.T, dir string) store.Film {
	t.Helper()
	return keepTalkFrom(t, dir, "gonedesk")
}

// keepTalkFrom puts the talk on the phone as downloaded from desktop.
func keepTalkFrom(t *testing.T, dir, desktop string) store.Film {
	t.Helper()
	st := store.Open(dir)
	f := store.Film{Desktop: desktop, Library: "Japanese Class", Video: "video.mp4", Added: now(),
		Film: sync.Film{ID: "f1", Title: "Morning Talk", DurationMS: 10_000, Spoken: "ja", Translation: "简体中文", VideoSize: int64(len(talkVideo))}}
	if err := st.SaveFilm(f); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveCues(f.Desktop, f.Film.ID, talkCues); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(st.FilmDir(f.Desktop, f.Film.ID), f.Video), talkVideo, 0o600); err != nil {
		t.Fatal(err)
	}
	return f
}
