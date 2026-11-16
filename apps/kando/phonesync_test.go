package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/phonesync"
	"kando/apps/kando/settings"
	"kando/apps/kando/shelf"
	psync "kando/packages/sync"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// phoneClock is a clock a test moves, which the phone sync server reads (phoneUI.now).
type phoneClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *phoneClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *phoneClock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// phoneSyncApp starts the app with remoteLibrary's library open, settings.json in a folder of its own, beside which the server keeps its certificate and devices, the server listening on loopback in place of the computer's local network, and a clock the test moves. The server stops when the test ends.
func phoneSyncApp(t *testing.T) (*app, *nsgotest.App, chan func(), string, *phoneClock) {
	t.Helper()
	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	clock := &phoneClock{now: time.Now()}
	a.phone.listen = func() []netip.Addr { return []netip.Addr{netip.MustParseAddr("127.0.0.1")} }
	a.phone.now = clock.Now
	dir := remoteLibrary(t, a)
	refresh(tt, a)
	t.Cleanup(a.phoneSyncStop)
	return a, tt, updates, dir, clock
}

// phoneSyncOn opens Settings › 手机同步 and turns the switch on, and waits for the server to start.
func phoneSyncOn(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
	t.Helper()
	settingsOpenAt(a, tt, settings.SectionPhone)
	tt.Press("settings-phone-sync")
	waitUntil(t, tt, a, updates, func() bool { return a.phone.server != nil })
}

// phoneCall runs call, a phone's request, on another goroutine, as a phone on the network would, while the test applies the app's updates on its own goroutine, as the engine would, until it answers; then it refreshes.
func phoneCall[T any](t *testing.T, a *app, tt *nsgotest.App, updates <-chan func(), call func(ctx context.Context) (T, error)) (T, error) {
	t.Helper()
	type answer struct {
		v   T
		err error
	}
	done := make(chan answer, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		v, err := call(ctx)
		done <- answer{v, err}
	}()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case got := <-done:
			// The server may have said something changed on its way out, as after a pairing.
			for drained := false; !drained; {
				select {
				case fn := <-updates:
					onEngine(a, fn)
				default:
					drained = true
				}
			}
			refresh(tt, a)
			return got.v, got.err
		case fn := <-updates:
			onEngine(a, func() {
				fn()
				a.Changed()
			})
		case <-timeout:
			t.Fatal("the phone's request did not answer")
		}
	}
}

// phoneClient is a phone's client of a's server, pinning its certificate, with token, "" before pairing.
func phoneClient(t *testing.T, a *app, token string) *psync.Client {
	t.Helper()
	addrs := a.phone.server.Addrs()
	if len(addrs) == 0 {
		t.Fatal("the server listens nowhere")
	}
	c := psync.NewClient(addrs[0], a.phone.server.Fingerprint(), token)
	t.Cleanup(c.Close)
	return c
}

func phonePair(t *testing.T, a *app, tt *nsgotest.App, updates <-chan func(), c *psync.Client, code string) (psync.PairResponse, error) {
	t.Helper()
	return phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.PairResponse, error) {
		return c.Pair(ctx, psync.PairRequest{Code: code, Device: "Evan's iPhone", Platform: psync.PlatformIOS})
	})
}

func phoneWantCode(t *testing.T, err error, status int, code string) {
	t.Helper()
	var e *psync.Error
	if !errors.As(err, &e) || e.Status != status || e.Code != code {
		t.Fatalf("error %v, want %d %s", err, status, code)
	}
}

// phoneWrongCode is a code that is not code.
func phoneWrongCode(code string) string {
	if code == "000000" {
		return "111111"
	}
	return "000000"
}

func TestPhoneSyncPairsWithTheCodeShownAndServesTheLibrary(t *testing.T) {
	a, tt, updates, dir, _ := phoneSyncApp(t)
	phoneSyncOn(t, a, tt, updates)
	m := testlocale.Messages()

	// The page shows the code it offers as it opens, with its QR code, where the desktop listens and the fingerprint's short form.
	code := a.phone.offer.Code
	if !psync.ValidCode(code) || a.phone.offer.State != phonesync.OfferLive {
		t.Fatalf("offer %+v once the page shows", a.phone.offer)
	}
	if got := tt.Find("id:settings-phone-code").Text; got != code[:3]+" "+code[3:] {
		t.Fatalf("the page shows code %q, want %s", got, code)
	}
	if !tt.Exists("id:settings-phone-qr") || len(a.phone.qr) == 0 {
		t.Fatal("no QR code")
	}
	if got, want := tt.Find("id:settings-phone-address").Text, a.phone.server.Addrs()[0].String(); got != want {
		t.Fatalf("the page shows address %q, want %q", got, want)
	}
	if got := tt.Find("id:settings-phone-fingerprint").Text; got != psync.ShortFingerprint(a.phone.server.Fingerprint()) {
		t.Fatalf("the page shows fingerprint %q", got)
	}
	if !tt.Exists("id:settings-phone-no-devices") {
		t.Fatal("the page lists devices before any paired")
	}
	// What the QR code holds is the pairing URL a phone reads.
	pairURL, err := psync.ParsePairURL(a.phone.qrText)
	if err != nil || pairURL.Code != code || len(pairURL.Addrs) != 1 || string(pairURL.Fingerprint) != string(a.phone.server.Fingerprint()) {
		t.Fatalf("pairing URL %q: %+v, %v", a.phone.qrText, pairURL, err)
	}

	c := phoneClient(t, a, "")
	res, err := phonePair(t, a, tt, updates, c, code)
	if err != nil {
		t.Fatal(err)
	}
	if !hasText(tt, "Evan's iPhone") || !hasText(tt, m.SettingsPhoneCodeUsed()) || tt.Exists("id:settings-phone-qr") {
		t.Fatal("the page does not list the phone paired, or still shows the code it used")
	}
	if !tt.Exists("id:settings-phone-revoke-" + res.DeviceID) {
		t.Fatal("no 撤销 for the phone")
	}

	libs, err := phoneCall(t, a, tt, updates, c.Libraries)
	if err != nil {
		t.Fatal(err)
	}
	if len(libs) != 1 || libs[0].ID != phoneLibraryID(dir) || libs[0].Films != 2 {
		t.Fatalf("libraries %+v", libs)
	}
	list, err := phoneCall(t, a, tt, updates, func(ctx context.Context) ([]psync.Film, error) { return c.Films(ctx, libs[0].ID) })
	if err != nil {
		t.Fatal(err)
	}
	// The finished YouTube film and the video found in the folder, not the job still waiting.
	byTitle := map[string]psync.Film{}
	for _, f := range list {
		byTitle[f.Title] = f
	}
	talk, interview := byTitle["Rust Talk"], byTitle["interview"]
	if len(list) != 2 || talk.ID != "yt" || talk.Source != "youtube" || talk.Translation != "简体中文" || talk.VideoSize != int64(len("video")) || interview.ID == "" || interview.Source != "local" {
		t.Fatalf("films %+v", list)
	}

	cues, err := phoneCall(t, a, tt, updates, func(ctx context.Context) ([]psync.Cue, error) { return c.Subtitles(ctx, "yt") })
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 2 || cues[0].Original != "Hello there" || cues[0].Translation != "你好" || cues[0].StartMS != 1000 || cues[1].Original != "General Kenobi" || cues[1].Translation != "" {
		t.Fatalf("cues %+v", cues)
	}

	// The body is read inside the call: phoneCall cancels its context once the call returns, which ends the body.
	var body []byte
	part, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (*psync.Part, error) {
		p, err := c.Video(ctx, "yt", 1, 3, "")
		if err != nil {
			return nil, err
		}
		defer p.Body.Close()
		body, err = io.ReadAll(p.Body)
		return p, err
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ide" || part.Start != 1 || part.End != 3 || part.Total != 5 || part.ETag == "" {
		t.Fatalf("part %q %+v", body, part)
	}

	// A film with no poster yet has none to give.
	_, err = phoneCall(t, a, tt, updates, func(ctx context.Context) ([]byte, error) {
		data, _, err := c.Poster(ctx, "yt")
		return data, err
	})
	phoneWantCode(t, err, http.StatusNotFound, psync.CodeNotFound)
	// The poster the library found for a film, as its card shows it, is the one a phone gets, once the films are listed again.
	a.lib.posterDir = t.TempDir()
	remoteWriteFile(t, posterDest(a.lib.posterDir, filepath.Join(dir, "Rust Talk", "talk.mp4")), "jpeg")
	list, err = phoneCall(t, a, tt, updates, func(ctx context.Context) ([]psync.Film, error) { return c.Films(ctx, libs[0].ID) })
	if err != nil || len(list) != 2 || (list[0].Poster != psync.PosterPath("yt") && list[1].Poster != psync.PosterPath("yt")) {
		t.Fatalf("films %+v, %v once the talk has a poster", list, err)
	}
	poster, err := phoneCall(t, a, tt, updates, func(ctx context.Context) ([]byte, error) {
		data, _, err := c.Poster(ctx, "yt")
		return data, err
	})
	if err != nil || string(poster) != "jpeg" {
		t.Fatalf("poster %q, %v", poster, err)
	}
}

func TestPhoneSyncRefusesCodesThatDoNotPairAndRevokedPhones(t *testing.T) {
	a, tt, updates, _, clock := phoneSyncApp(t)
	phoneSyncOn(t, a, tt, updates)
	c := phoneClient(t, a, "")
	code := a.phone.offer.Code

	_, err := phonePair(t, a, tt, updates, c, phoneWrongCode(code))
	phoneWantCode(t, err, http.StatusForbidden, psync.CodeWrongCode)
	res, err := phonePair(t, a, tt, updates, c, code)
	if err != nil {
		t.Fatal(err)
	}
	// The code a phone used pairs no other.
	_, err = phonePair(t, a, tt, updates, phoneClient(t, a, ""), code)
	phoneWantCode(t, err, http.StatusForbidden, psync.CodeCodeUsed)

	// 新配对码 offers another, which pairs only for its five minutes.
	tt.Press("settings-phone-new-code")
	fresh := a.phone.offer.Code
	if a.phone.offer.State != phonesync.OfferLive || !tt.Exists("id:settings-phone-qr") {
		t.Fatalf("offer %+v after 新配对码", a.phone.offer)
	}
	clock.Add(psync.CodeLifetime)
	_, err = phonePair(t, a, tt, updates, phoneClient(t, a, ""), fresh)
	phoneWantCode(t, err, http.StatusForbidden, psync.CodeCodeExpired)

	// No code is on offer once the page shows something else.
	tt.Press("settings-phone-new-code")
	fresh = a.phone.offer.Code
	tt.Press("settings-section-" + settings.SectionGeneral)
	if a.phone.offer.State != phonesync.OfferNone {
		t.Fatalf("offer %+v after leaving the page", a.phone.offer)
	}
	_, err = phonePair(t, a, tt, updates, phoneClient(t, a, ""), fresh)
	phoneWantCode(t, err, http.StatusForbidden, psync.CodeNoCode)

	// A phone revoked is refused from then on.
	paired := phoneClient(t, a, res.Token)
	if _, err := phoneCall(t, a, tt, updates, paired.Libraries); err != nil {
		t.Fatal(err)
	}
	settingsOpenAt(a, tt, settings.SectionPhone)
	tt.Press("settings-phone-revoke-" + res.DeviceID)
	if !tt.Exists("id:settings-phone-no-devices") {
		t.Fatal("the page still lists the phone revoked")
	}
	_, err = phoneCall(t, a, tt, updates, paired.Libraries)
	phoneWantCode(t, err, http.StatusUnauthorized, psync.CodeUnauthorized)
	_, err = phoneCall(t, a, tt, updates, phoneClient(t, a, "").Libraries)
	phoneWantCode(t, err, http.StatusUnauthorized, psync.CodeUnauthorized)
}

func TestPhoneSyncNeverServesALockedLibrary(t *testing.T) {
	a, tt, updates, dir, _ := phoneSyncApp(t)
	other := t.TempDir()
	remoteWriteFile(t, filepath.Join(other, "clip.mp4"), "clip")
	a.edit(func(s *session) { s.Libraries = []string{dir, other} })
	l, err := shelf.NewLock("pw")
	if err != nil {
		t.Fatal(err)
	}
	a.library.SetLock(dir, l)
	phoneSyncOn(t, a, tt, updates)
	c := phoneClient(t, a, "")
	if _, err := phonePair(t, a, tt, updates, c, a.phone.offer.Code); err != nil {
		t.Fatal(err)
	}
	libs, err := phoneCall(t, a, tt, updates, c.Libraries)
	if err != nil {
		t.Fatal(err)
	}
	if len(libs) != 1 || libs[0].ID != phoneLibraryID(other) || libs[0].Films != 1 {
		t.Fatalf("libraries %+v, want only the one not locked", libs)
	}
	_, err = phoneCall(t, a, tt, updates, func(ctx context.Context) ([]psync.Film, error) { return c.Films(ctx, phoneLibraryID(dir)) })
	phoneWantCode(t, err, http.StatusNotFound, psync.CodeNotFound)
	for _, get := range []func(ctx context.Context) (any, error){
		func(ctx context.Context) (any, error) { return c.Subtitles(ctx, "yt") },
		func(ctx context.Context) (any, error) { return c.Video(ctx, "yt", 0, -1, "") },
	} {
		_, err := phoneCall(t, a, tt, updates, get)
		phoneWantCode(t, err, http.StatusNotFound, psync.CodeNotFound)
	}
}

func TestPhoneSyncOffListensNowhere(t *testing.T) {
	a, tt, updates, _, _ := phoneSyncApp(t)
	phoneSyncOn(t, a, tt, updates)
	addr := a.phone.server.Addrs()[0]
	conn, err := net.DialTimeout("tcp", addr.String(), time.Second)
	if err != nil {
		t.Fatalf("nothing listens while on: %v", err)
	}
	conn.Close()
	data, err := os.ReadFile(a.settingsPath)
	if err != nil || !strings.Contains(string(data), `"phoneSync": true`) || !strings.Contains(string(data), `"phoneSyncPort"`) {
		t.Fatalf("settings.json once on: %s, %v", data, err)
	}

	tt.Press("settings-phone-sync")
	if a.phone.server != nil || tt.Exists("id:settings-phone-pairing") {
		t.Fatal("the server or its pairing card is still there once off")
	}
	if conn, err := net.DialTimeout("tcp", addr.String(), time.Second); err == nil {
		conn.Close()
		t.Fatalf("still listening on %s once off", addr)
	}
	data, err = os.ReadFile(a.settingsPath)
	if err != nil || strings.Contains(string(data), `"phoneSync"`) {
		t.Fatalf("settings.json once off: %s, %v", data, err)
	}
}
