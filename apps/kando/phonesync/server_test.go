package phonesync

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	gosync "sync"
	"testing"
	"time"

	psync "kando/packages/sync"
)

// fakeSource is two libraries, the second locked and so never given, as the app gives them: the first with a film whose video, subtitles and poster are on disk, and one whose video is gone.
type fakeSource struct {
	dir string
}

func (f fakeSource) Libraries(context.Context) ([]Library, error) {
	return []Library{{ID: "lib1", Name: "My Films", Dir: f.dir}}, nil
}

func (f fakeSource) Films(_ context.Context, lib Library) ([]Film, error) {
	if lib.ID != "lib1" {
		return nil, ErrNotFound
	}
	return []Film{
		{ID: "talk", Title: "Rust Talk", Source: "youtube", Spoken: "en", Translation: "简体中文", Duration: 90 * time.Second,
			Video: filepath.Join(f.dir, "talk.mp4"), Subtitles: filepath.Join(f.dir, "talk.bilingual.srt"), Poster: filepath.Join(f.dir, "cover.jpg")},
		{ID: "gone", Title: "Gone", Video: filepath.Join(f.dir, "gone.mp4")},
	}, nil
}

func (f fakeSource) Cues(_ context.Context, film Film) ([]psync.Cue, error) {
	return []psync.Cue{{StartMS: 1000, EndMS: 2000, Original: "Hello " + film.Title, Translation: "你好"}}, nil
}

// clock is a clock a test moves.
type clock struct {
	mu  gosync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// startTest starts a server on loopback with fakeSource's library in a folder of its own, and returns it with its config folder and its clock.
func startTest(t *testing.T) (*Server, string, *clock) {
	t.Helper()
	lib := t.TempDir()
	for name, data := range map[string]string{"talk.mp4": "0123456789abcdefghij", "talk.bilingual.srt": "1\n00:00:01,000 --> 00:00:02,000\n你好\nHello\n", "cover.jpg": "jpeg"} {
		if err := os.WriteFile(filepath.Join(lib, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	c := &clock{now: time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)}
	srv, err := Start(Config{Dir: dir, Name: "Test Mac", Source: fakeSource{dir: lib}, Addrs: loopback, Now: c.Now, Logf: testLogf(t)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	return srv, dir, c
}

// testLogf is t.Logf until the test ends, then nothing: http.Server's connection goroutines can still log a failed handshake after Close returns, and t.Logf panics once the test is over.
func testLogf(t *testing.T) func(string, ...any) {
	var mu gosync.Mutex
	over := false
	t.Cleanup(func() {
		mu.Lock()
		over = true
		mu.Unlock()
	})
	return func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		if !over {
			t.Logf(format, args...)
		}
	}
}

func loopback() []netip.Addr { return []netip.Addr{netip.MustParseAddr("127.0.0.1")} }

// client is a phone's client of srv, pinning its certificate.
func client(t *testing.T, srv *Server, token string) *psync.Client {
	t.Helper()
	addrs := srv.Addrs()
	if len(addrs) != 1 {
		t.Fatalf("listening on %v", addrs)
	}
	c := psync.NewClient(addrs[0], srv.Fingerprint(), token)
	t.Cleanup(c.Close)
	return c
}

func wantCode(t *testing.T, err error, status int, code string) {
	t.Helper()
	var e *psync.Error
	if !errors.As(err, &e) || e.Status != status || e.Code != code {
		t.Fatalf("error %v, want %d %s", err, status, code)
	}
}

func pair(t *testing.T, c *psync.Client, code string) (psync.PairResponse, error) {
	t.Helper()
	return c.Pair(context.Background(), psync.PairRequest{Code: code, Device: "Evan's iPhone", Platform: psync.PlatformIOS})
}

func TestPairingTakesTheCodeOnOfferOnce(t *testing.T) {
	srv, dir, _ := startTest(t)
	c := client(t, srv, "")
	if _, err := pair(t, c, "123456"); err == nil {
		t.Fatal("paired with no code on offer")
	} else {
		wantCode(t, err, http.StatusForbidden, psync.CodeNoCode)
	}
	offer := srv.NewOffer()
	if !psync.ValidCode(offer.Code) || offer.State != OfferLive {
		t.Fatalf("offer %+v", offer)
	}
	wrong := "000000"
	if offer.Code == wrong {
		wrong = "111111"
	}
	_, err := pair(t, c, wrong)
	wantCode(t, err, http.StatusForbidden, psync.CodeWrongCode)
	res, err := pair(t, c, offer.Code)
	if err != nil {
		t.Fatal(err)
	}
	if res.Token == "" || res.Desktop != "Test Mac" || srv.Offer().State != OfferUsed {
		t.Fatalf("pairing answered %+v; offer %+v", res, srv.Offer())
	}
	_, err = pair(t, client(t, srv, ""), offer.Code)
	wantCode(t, err, http.StatusForbidden, psync.CodeCodeUsed)

	devs := srv.Devices()
	if len(devs) != 1 || devs[0].ID != res.DeviceID || devs[0].Name != "Evan's iPhone" || devs[0].Platform != psync.PlatformIOS {
		t.Fatalf("devices %+v", devs)
	}
	// The device list keeps the token's hash, never the token.
	data, err := os.ReadFile(filepath.Join(dir, DevicesFile))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(res.Token)) || !bytes.Contains(data, []byte(tokenHash(res.Token))) {
		t.Fatalf("the device list holds the token or not its hash:\n%s", data)
	}
	wantPrivate(t, filepath.Join(dir, DevicesFile))
}

func TestPairingRefusesAnExpiredCodeAndOneTriedTooOften(t *testing.T) {
	srv, _, clk := startTest(t)
	c := client(t, srv, "")
	offer := srv.NewOffer()
	clk.Add(psync.CodeLifetime)
	_, err := pair(t, c, offer.Code)
	wantCode(t, err, http.StatusForbidden, psync.CodeCodeExpired)
	if srv.Offer().State != OfferExpired {
		t.Fatalf("offer %+v after its time", srv.Offer())
	}

	offer = srv.NewOffer()
	wrong := "000000"
	if offer.Code == wrong {
		wrong = "111111"
	}
	for range psync.CodeTries {
		_, err := pair(t, c, wrong)
		wantCode(t, err, http.StatusForbidden, psync.CodeWrongCode)
	}
	_, err = pair(t, c, offer.Code)
	wantCode(t, err, http.StatusForbidden, psync.CodeCodeExpired)

	// Past the limit of pairings a minute, which the 7 above and 3 more reach, even a right code waits.
	offer = srv.NewOffer()
	if wrong = "000000"; offer.Code == wrong {
		wrong = "111111"
	}
	for range pairLimit - psync.CodeTries - 2 {
		_, err := pair(t, c, wrong)
		wantCode(t, err, http.StatusForbidden, psync.CodeWrongCode)
	}
	_, err = pair(t, c, offer.Code)
	wantCode(t, err, http.StatusTooManyRequests, psync.CodeTooMany)
	clk.Add(pairWindow)
	if _, err := pair(t, c, offer.Code); err != nil {
		t.Fatalf("a minute later: %v", err)
	}
}

func TestReadsNeedAPairedDevice(t *testing.T) {
	srv, _, _ := startTest(t)
	ctx := context.Background()
	_, err := client(t, srv, "").Libraries(ctx)
	wantCode(t, err, http.StatusUnauthorized, psync.CodeUnauthorized)
	_, err = client(t, srv, "made-up").Films(ctx, "lib1")
	wantCode(t, err, http.StatusUnauthorized, psync.CodeUnauthorized)

	c := client(t, srv, "")
	res, err := pair(t, c, srv.NewOffer().Code)
	if err != nil {
		t.Fatal(err)
	}
	libs, err := c.Libraries(ctx)
	if err != nil || len(libs) != 1 || libs[0].ID != "lib1" || libs[0].Name != "My Films" || libs[0].Films != 1 {
		t.Fatalf("libraries %+v, %v", libs, err)
	}
	if err := srv.Revoke(res.DeviceID); err != nil {
		t.Fatal(err)
	}
	_, err = c.Libraries(ctx)
	wantCode(t, err, http.StatusUnauthorized, psync.CodeUnauthorized)
	if len(srv.Devices()) != 0 {
		t.Fatalf("devices %+v after revoking", srv.Devices())
	}
}

func TestServesTheFilmsAndTheirFiles(t *testing.T) {
	srv, _, _ := startTest(t)
	ctx := context.Background()
	c := client(t, srv, "")
	if _, err := pair(t, c, srv.NewOffer().Code); err != nil {
		t.Fatal(err)
	}
	films, err := c.Films(ctx, "lib1")
	if err != nil {
		t.Fatal(err)
	}
	// The film whose video is gone is left out.
	if len(films) != 1 {
		t.Fatalf("films %+v", films)
	}
	f := films[0]
	if f.ID != "talk" || f.Title != "Rust Talk" || f.Source != "youtube" || f.Spoken != "en" || f.Translation != "简体中文" || f.DurationMS != 90000 || f.VideoSize != 20 || f.Poster != psync.PosterPath("talk") || f.Updated.IsZero() {
		t.Fatalf("film %+v", f)
	}
	_, err = c.Films(ctx, "lib2")
	wantCode(t, err, http.StatusNotFound, psync.CodeNotFound)

	cues, err := c.Subtitles(ctx, "talk")
	if err != nil || len(cues) != 1 || cues[0].Original != "Hello Rust Talk" {
		t.Fatalf("cues %+v, %v", cues, err)
	}
	_, err = c.Subtitles(ctx, "gone")
	wantCode(t, err, http.StatusNotFound, psync.CodeNotFound)
	poster, kind, err := c.Poster(ctx, "talk")
	if err != nil || string(poster) != "jpeg" || kind != "image/jpeg" {
		t.Fatalf("poster %q %q, %v", poster, kind, err)
	}

	part, err := c.Video(ctx, "talk", 10, 14, "")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(part.Body)
	part.Body.Close()
	if string(body) != "abcde" || part.Start != 10 || part.End != 14 || part.Total != 20 || part.ETag == "" {
		t.Fatalf("part %q %+v", body, part)
	}
	again, err := c.Video(ctx, "talk", 15, -1, part.ETag)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(again.Body)
	again.Body.Close()
	if string(body) != "fghij" || again.ETag != part.ETag {
		t.Fatalf("the rest %q %+v", body, again)
	}
	_, err = c.Video(ctx, "gone", 0, -1, "")
	wantCode(t, err, http.StatusNotFound, psync.CodeNotFound)
}

func TestTheCertificateIsMadeOnceAndKeptPrivate(t *testing.T) {
	srv, dir, _ := startTest(t)
	fp := srv.Fingerprint()
	for _, name := range []string{CertFile, KeyFile} {
		wantPrivate(t, filepath.Join(dir, name))
	}
	srv.Close()
	again, err := Start(Config{Dir: dir, Source: fakeSource{}, Addrs: loopback, Logf: testLogf(t)})
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if !bytes.Equal(again.Fingerprint(), fp) {
		t.Fatal("the certificate changed when the server started again")
	}
}

func TestClosedListensNowhere(t *testing.T) {
	srv, _, _ := startTest(t)
	addr := srv.Addrs()[0]
	conn, err := net.DialTimeout("tcp", addr.String(), time.Second)
	if err != nil {
		t.Fatalf("not listening while on: %v", err)
	}
	conn.Close()
	if err := srv.Close(); err != nil {
		t.Fatal(err)
	}
	if conn, err := net.DialTimeout("tcp", addr.String(), time.Second); err == nil {
		conn.Close()
		t.Fatalf("still listening on %s once closed", addr)
	}
	if len(srv.Addrs()) != 0 {
		t.Fatalf("addresses %v once closed", srv.Addrs())
	}
}

func TestThePortIsKeptAcrossStarts(t *testing.T) {
	srv, dir, _ := startTest(t)
	port := srv.Port()
	srv.Close()
	again, err := Start(Config{Dir: dir, Port: port, Source: fakeSource{}, Addrs: loopback, Logf: testLogf(t)})
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if again.Port() != port {
		t.Fatalf("port %d, want %d as before", again.Port(), port)
	}
}

func TestPickAddrsKeepsTheLocalNetwork(t *testing.T) {
	a := netip.MustParseAddr
	got := pickAddrs([]candidate{
		{addr: a("127.0.0.1")},
		{addr: a("8.8.8.8")},
		{addr: a("fd00::1")},
		{addr: a("100.101.102.103")},
		{addr: a("10.8.0.2"), vpn: true},
		{addr: a("192.168.1.20")},
		{addr: a("172.20.10.3")},
		{addr: a("192.168.1.20")},
	})
	if want := []netip.Addr{a("192.168.1.20"), a("172.20.10.3")}; !equalAddrs(got, want) {
		t.Fatalf("addresses %v, want %v", got, want)
	}
	// With nothing else, the VPN's and Tailscale's are used.
	got = pickAddrs([]candidate{{addr: a("100.101.102.103")}, {addr: a("10.8.0.2"), vpn: true}, {addr: a("127.0.0.1")}})
	if want := []netip.Addr{a("100.101.102.103"), a("10.8.0.2")}; !equalAddrs(got, want) {
		t.Fatalf("addresses %v, want %v", got, want)
	}
	for _, name := range []string{"utun3", "Tailscale", "wg0", "ProtonVPN"} {
		if !vpnName(name) {
			t.Errorf("%s is not taken for a VPN", name)
		}
	}
	for _, name := range []string{"en0", "eth0", "Wi-Fi", "wlan0"} {
		if vpnName(name) {
			t.Errorf("%s is taken for a VPN", name)
		}
	}
}

func TestQRCodeIsAPNG(t *testing.T) {
	png, modules, err := QRCode("kando://pair?a=192.168.1.20%3A52817&c=123456&fp=" + strings.Repeat("A", 43) + "&n=Mac")
	if err != nil || !bytes.HasPrefix(png, []byte("\x89PNG")) || modules < 29 {
		t.Fatalf("QR code of %d bytes, %d modules, %v", len(png), modules, err)
	}
}

func equalAddrs(a, b []netip.Addr) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// wantPrivate fails t unless the file at path is the user's alone, where the system says so (not on Windows, whose config folder is the user's own).
func wantPrivate(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("%s is %v, want 0600", filepath.Base(path), info.Mode().Perm())
	}
}

// TestACorrectionTellsThePhoneTheFilmChanged checks that a film's time, which a phone that has it compares to read its lines again, is that of its corrections file once the user corrects a line after it was processed.
func TestACorrectionTellsThePhoneTheFilmChanged(t *testing.T) {
	dir := t.TempDir()
	f := Film{ID: "talk", Video: filepath.Join(dir, "video.mp4"), Subtitles: filepath.Join(dir, "video.bilingual.srt"), Corrections: filepath.Join(dir, ".video.edits.json")}
	processed := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	for _, p := range []string{f.Video, f.Subtitles} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, processed, processed); err != nil {
			t.Fatal(err)
		}
	}
	if got := playable([]Film{f})[0].film.Updated; !got.Equal(processed) {
		t.Fatalf("before any correction the film changed at %v, want %v", got, processed)
	}
	corrected := processed.Add(time.Hour)
	if err := os.WriteFile(f.Corrections, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(f.Corrections, corrected, corrected); err != nil {
		t.Fatal(err)
	}
	if got := playable([]Film{f})[0].film.Updated; !got.Equal(corrected) {
		t.Fatalf("after a correction the film changed at %v, want %v", got, corrected)
	}
}

// A call waits for the app as long as an ordinary request does, but subscribing, which reads the whole list first, waits as long as the phone does for it.
func TestCallTimeout(t *testing.T) {
	if got := callTimeout(psync.OpSubscribe); got != psync.SubscribeWait {
		t.Fatalf("subscribing waits %v, want %v", got, psync.SubscribeWait)
	}
	for _, op := range []string{psync.OpJobs, psync.OpSubscriptions, psync.OpSubscriptionTake, ""} {
		if got := callTimeout(op); got != sourceTimeout {
			t.Fatalf("%q waits %v, want %v", op, got, sourceTimeout)
		}
	}
}

func (fakeSource) Making(context.Context, Library) ([]Film, error) { return nil, nil }
