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
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/phonesync"
	"kando/apps/kando/settings"
	"kando/apps/kando/shelf"
	"kando/packages/core"
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
	tt.Press("settings-phone-revoke-confirm-" + res.DeviceID)
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

// phonePaired is phoneSyncApp with the server on and a phone paired, whose client it answers; the queue's pipeline blocks, so a job stays where the test leaves it.
func phonePaired(t *testing.T) (*app, *nsgotest.App, chan func(), string, *psync.Client) {
	t.Helper()
	a, tt, updates, dir, _ := phoneSyncApp(t)
	a.pipeline = &fakePipeline{block: true}
	phoneSyncOn(t, a, tt, updates)
	c := phoneClient(t, a, "")
	if _, err := phonePair(t, a, tt, updates, c, a.phone.offer.Code); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		a.queue.stopActive(context.Canceled)
		waitUntil(t, tt, a, updates, func() bool { return !a.running() })
	})
	return a, tt, updates, dir, c
}

func phoneSend(t *testing.T, a *app, tt *nsgotest.App, updates <-chan func(), c *psync.Client, link string) (psync.JobCreated, error) {
	t.Helper()
	return phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.JobCreated, error) { return c.CreateJob(ctx, link, psync.JobOptions{}) })
}

// A link a phone sends is queued at once, with the phone's name on its card, and the phone follows the job through the list and the events.
func TestPhoneSyncQueuesALinkAPhoneSends(t *testing.T) {
	a, tt, updates, dir, c := phonePaired(t)
	m := testlocale.Messages()
	h, err := phoneCall(t, a, tt, updates, c.Hello)
	if err != nil || h.Receive != psync.ReceiveQueue || !slices.Contains(h.Ops, psync.OpJobCreate) || !slices.Contains(h.Ops, psync.OpJobPause) || !slices.Contains(h.Ops, psync.OpJobResume) || h.Desktop != a.phone.name {
		t.Fatalf("hello %+v, %v", h, err)
	}
	list, err := phoneCall(t, a, tt, updates, c.Jobs)
	if err != nil {
		t.Fatal(err)
	}
	// The open library's waiting job, but not its done film the queue did not run.
	if len(list.Jobs) != 1 || list.Jobs[0].ID != "clip" || list.Jobs[0].State != psync.JobWaiting || list.Jobs[0].Library != phoneLibraryID(dir) || list.Jobs[0].Link != "" {
		t.Fatalf("jobs %+v", list.Jobs)
	}

	// A phone is served films by their video, so its link keeps the video though the desktop downloads links as 仅字幕.
	a.online.quality = core.QualitySubtitles
	link := "https://www.youtube.com/watch?v=jNQXAC9IVRw"
	created, err := phoneSend(t, a, tt, updates, c, link)
	if err != nil || created.Job == nil || created.Pending || created.Existing || created.Job.Device != "Evan's iPhone" || created.Job.Link != link {
		t.Fatalf("created %+v, %v", created, err)
	}
	id := created.Job.ID
	if job, ok := a.library.Find(id); !ok || job.Device != "Evan's iPhone" {
		t.Fatalf("job on the shelf %+v", job)
	}
	a.Select(pageFilms)
	a.prefs.open = false
	refresh(tt, a)
	if !hasText(tt, m.FilmsQueueFrom("Evan's iPhone")) || !hasText(tt, m.PhoneSyncLinkQueued("Evan's iPhone")) {
		t.Fatal("the card does not say where the job came from, or the message bar what came of the link")
	}
	// The job's card keeps the phone's name as the pipeline reports it, which it was started without.
	waitUntil(t, tt, a, updates, func() bool {
		job, _ := a.library.Find(id)
		return job.Stage == core.StageDownloading || job.Stage == core.StageTranscribing
	})
	if job, _ := a.library.Find(id); job.Device != "Evan's iPhone" || job.Quality != core.QualityBest {
		t.Fatalf("the running job forgot its phone, or keeps no video: %+v", job)
	}

	ev, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.Events, error) {
		return c.Events(ctx, list.Epoch, list.Rev, 5*time.Second)
	})
	if err != nil || ev.Reset || len(ev.Events) == 0 {
		t.Fatalf("events %+v, %v", ev, err)
	}
	var seen *psync.Job
	for _, e := range ev.Events {
		if e.ID == id {
			seen = e.Job
		}
	}
	if seen == nil || seen.Device != "Evan's iPhone" {
		t.Fatalf("events %+v name no job of the phone's", ev.Events)
	}

	// The same link again answers the job it made.
	again, err := phoneSend(t, a, tt, updates, c, link)
	if err != nil || !again.Existing || again.Job == nil || again.Job.ID != id {
		t.Fatalf("sent again: %+v, %v", again, err)
	}
	// The phone stops it, and the card says it was cancelled.
	stopped, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.Job, error) { return c.ActOnJob(ctx, psync.OpJobCancel, id) })
	if err != nil || stopped.State != psync.JobCanceled {
		t.Fatalf("canceled %+v, %v", stopped, err)
	}
	// A job cancelled is not one to pause.
	_, err = phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.Job, error) { return c.ActOnJob(ctx, psync.OpJobPause, id) })
	phoneWantCode(t, err, http.StatusConflict, psync.CodeConflict)
	_, err = phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.Job, error) { return c.ActOnJob(ctx, psync.OpJobCancel, "nope") })
	phoneWantCode(t, err, http.StatusNotFound, psync.CodeNotFound)
	// A video on the desktop's disk is not a phone's to send.
	_, err = phoneSend(t, a, tt, updates, c, "file:///Users/evan/talk.mp4")
	phoneWantCode(t, err, http.StatusBadRequest, psync.CodeInvalid)
}

// With 收到后 set to 先问我 a link waits in the link field for the user, and with 接收手机发来的链接 off it is refused; both are kept in settings.json.
func TestPhoneSyncAsksFirstOrRefusesLinks(t *testing.T) {
	a, tt, updates, _, c := phonePaired(t)
	m := testlocale.Messages()
	if !tt.Exists("id:settings-phone-receive") || !hasText(tt, m.SettingsPhoneLinks()) {
		t.Fatal("the page has no 手机发来的链接")
	}
	a.phoneSyncSetAsk(true)
	refresh(tt, a)
	jobs := len(a.library.Jobs)
	link := "https://www.youtube.com/watch?v=jNQXAC9IVRw"
	created, err := phoneSend(t, a, tt, updates, c, link)
	if err != nil || !created.Pending || created.Job != nil {
		t.Fatalf("created %+v, %v", created, err)
	}
	if len(a.library.Jobs) != jobs || a.queue.source != link || a.page != pageFilms {
		t.Fatalf("asking first queued %d jobs, link field %q, page %q", len(a.library.Jobs)-jobs, a.queue.source, a.page)
	}
	if a.lib.notice.Text != m.PhoneSyncLinkAsk("Evan's iPhone") {
		t.Fatalf("message bar %q", a.lib.notice.Text)
	}
	// Another link joins what the field holds, the user's typing or links phones sent before, rather than replacing it, and the same link again is not added twice.
	a.queue.source = link + " https://youtu.be/typed"
	other := "https://www.youtube.com/watch?v=M7lc1UVf-VE"
	for _, l := range []string{other, link} {
		if created, err := phoneSend(t, a, tt, updates, c, l); err != nil || !created.Pending {
			t.Fatalf("created %+v, %v", created, err)
		}
	}
	if want := link + " https://youtu.be/typed " + other; a.queue.source != want || len(a.library.Jobs) != jobs {
		t.Fatalf("link field %q, want %q; %d jobs queued", a.queue.source, want, len(a.library.Jobs)-jobs)
	}

	tt.Press("settings-phone-receive")
	if !a.phone.receiveOff {
		t.Fatal("the switch did not turn receiving off")
	}
	_, err = phoneSend(t, a, tt, updates, c, link)
	phoneWantCode(t, err, http.StatusForbidden, psync.CodeLinksOff)
	h, err := phoneCall(t, a, tt, updates, c.Hello)
	if err != nil || h.Receive != psync.ReceiveOff {
		t.Fatalf("hello %+v, %v", h, err)
	}
	data, err := os.ReadFile(a.settingsPath)
	if err != nil || !strings.Contains(string(data), `"phoneReceiveOff": true`) || !strings.Contains(string(data), `"phoneReceiveAsk": true`) {
		t.Fatalf("settings.json: %s, %v", data, err)
	}
}

// A phone's link comes with the picture and the source of original lines the phone chose, which its job keeps over the desktop's Settings › 通用 › 在线视频; a value the protocol does not name is refused, and a link with none takes the desktop's.
func TestPhoneSyncTakesTheOptionsAPhoneChose(t *testing.T) {
	a, tt, updates, _, c := phonePaired(t)
	h, err := phoneCall(t, a, tt, updates, c.Hello)
	if err != nil || !slices.Contains(h.Options, psync.OptionQuality) || !slices.Contains(h.Options, psync.OptionSubtitles) {
		t.Fatalf("hello %+v, %v", h, err)
	}
	a.online.quality, a.online.heardAlone = core.QualityBest, false
	send := func(link string, opts psync.JobOptions) (psync.JobCreated, error) {
		return phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.JobCreated, error) { return c.CreateJob(ctx, link, opts) })
	}
	_, err = send("https://www.youtube.com/watch?v=M7lc1UVf-VE", psync.JobOptions{Quality: "4320p"})
	phoneWantCode(t, err, http.StatusBadRequest, psync.CodeInvalid)
	_, err = send("https://www.youtube.com/watch?v=M7lc1UVf-VE", psync.JobOptions{Subtitles: "guess"})
	phoneWantCode(t, err, http.StatusBadRequest, psync.CodeInvalid)

	chosen, err := send("https://www.youtube.com/watch?v=jNQXAC9IVRw", psync.JobOptions{Quality: psync.Quality720, Subtitles: psync.SubtitlesRecognize})
	if err != nil || chosen.Job == nil {
		t.Fatalf("created %+v, %v", chosen, err)
	}
	plain, err := send("https://www.youtube.com/watch?v=M7lc1UVf-VE", psync.JobOptions{})
	if err != nil || plain.Job == nil {
		t.Fatalf("created %+v, %v", plain, err)
	}
	// The first runs and is given the desktop's settings where it has none of its own (onlineFor): the phone's stay.
	waitUntil(t, tt, a, updates, func() bool {
		job, _ := a.library.Find(chosen.Job.ID)
		return job.Stage == core.StageDownloading || job.Stage == core.StageTranscribing
	})
	if job, _ := a.library.Find(chosen.Job.ID); job.Quality != core.Quality720 || job.SubtitleSource != core.SubtitlesLocal {
		t.Fatalf("the job the phone chose for runs at %q from %q", job.Quality, job.SubtitleSource)
	}
	if job, _ := a.library.Find(plain.Job.ID); job.Quality != "" || job.SubtitleSource != "" {
		t.Fatalf("the job the phone chose nothing for was given %q from %q before it ran", job.Quality, job.SubtitleSource)
	}
	// Both stop here, so the queue starts neither as the test ends.
	for _, id := range []string{plain.Job.ID, chosen.Job.ID} {
		if _, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.Job, error) { return c.ActOnJob(ctx, psync.OpJobCancel, id) }); err != nil {
			t.Fatal(err)
		}
	}
}

// A phone cancels and retries the jobs it lists of a library that is not the open one: the job the queue runs for it, one waiting for it, and one the queue ran to its end, as the list keeps it.
func TestPhoneSyncActsOnJobsOfAnotherLibrary(t *testing.T) {
	a, tt, updates, dir, c := phonePaired(t)
	second := t.TempDir()
	a.edit(func(s *session) { s.Libraries = []string{dir, second} })
	if _, err := phoneSend(t, a, tt, updates, c, "https://www.youtube.com/watch?v=jNQXAC9IVRw"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, tt, a, updates, func() bool { return a.RunningJob() != "" })
	running := a.RunningJob()
	var waiting string
	for _, job := range a.library.Jobs {
		if job.Stage == core.StageQueued {
			waiting = job.ID
		}
	}
	if waiting == "" {
		t.Fatalf("no job waits behind %s: %+v", running, a.library.Jobs)
	}
	a.edit(func(s *session) { s.LibraryDir = second })
	refresh(tt, a)

	act := func(op, id string) (psync.Job, error) {
		return phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.Job, error) { return c.ActOnJob(ctx, op, id) })
	}
	for _, id := range []string{waiting, running} {
		if job, err := act(psync.OpJobCancel, id); err != nil || job.ID != id || job.State != psync.JobCanceled {
			t.Fatalf("canceled %s: %+v, %v", id, job, err)
		}
	}
	list, err := phoneCall(t, a, tt, updates, c.Jobs)
	if err != nil || !slices.ContainsFunc(list.Jobs, func(j psync.Job) bool { return j.ID == running && j.State == psync.JobCanceled }) {
		t.Fatalf("jobs %+v, %v: the stopped job the queue ran is not listed", list.Jobs, err)
	}
	if job, err := act(psync.OpJobRetry, running); err != nil || job.ID != running || (job.State != psync.JobWaiting && job.State != psync.JobRunning) {
		t.Fatalf("retried %+v, %v", job, err)
	}
	if job, _ := a.library.Find(running); job.LibraryDir != dir {
		t.Fatalf("the retried job saves into %q, want %q", job.LibraryDir, dir)
	}
}

// Nothing is added to a locked library, and a locked library's jobs are neither listed nor acted on.
func TestPhoneSyncNeverWritesALockedLibrary(t *testing.T) {
	a, tt, updates, dir, c := phonePaired(t)
	l, err := shelf.NewLock("pw")
	if err != nil {
		t.Fatal(err)
	}
	a.library.SetLock(dir, l)
	refresh(tt, a)
	_, err = phoneSend(t, a, tt, updates, c, "https://www.youtube.com/watch?v=jNQXAC9IVRw")
	phoneWantCode(t, err, http.StatusConflict, psync.CodeNotReady)
	list, err := phoneCall(t, a, tt, updates, c.Jobs)
	if err != nil || len(list.Jobs) != 0 {
		t.Fatalf("jobs %+v, %v of a locked library", list.Jobs, err)
	}
	_, err = phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.Job, error) { return c.ActOnJob(ctx, psync.OpJobCancel, "clip") })
	phoneWantCode(t, err, http.StatusNotFound, psync.CodeNotFound)
	if job, _ := a.library.Find("clip"); job.Stage != core.StageQueued {
		t.Fatalf("the locked library's job %+v", job)
	}
}
