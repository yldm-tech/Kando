package main

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kando/apps/mobile/pair"
	"kando/apps/mobile/store"
	"kando/packages/sync"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// Typed by hand, the address shows the desktop certificate's fingerprint in short before the phone trusts it; once confirmed, the phone pairs and keeps the desktop.
func TestPairingShowsTheFingerprintBeforeTrustingIt(t *testing.T) {
	d := newDesktop(t)
	p := startPhone(t, "")
	p.startPairing()
	p.fillPairing(d.Addr.String(), "123456")
	p.app.Press("id:pair-continue")
	p.app.WaitFor("id:pair-compare", wait)
	if d.Paired() != 0 {
		t.Fatal("the phone sent the code before the fingerprint was compared")
	}
	// Turning the fingerprint down sends nothing.
	p.app.Press("id:pair-reject")
	p.app.WaitFor("id:pair-continue", wait)
	if d.Paired() != 0 {
		t.Fatal("the phone paired after the fingerprint was turned down")
	}
	p.app.Press("id:pair-continue")
	if got := p.app.WaitFor("id:pair-fingerprint", wait).Text; got != sync.ShortFingerprint(d.Fingerprint) {
		t.Fatalf("fingerprint %q, want %q", got, sync.ShortFingerprint(d.Fingerprint))
	}
	p.app.Press("id:pair-confirm")
	p.finishPairing(d)
	if d.Paired() != 1 {
		t.Fatalf("%d devices paired, want 1", d.Paired())
	}
	var desktops []store.Desktop
	var err error
	p.app.WaitUntil(wait, func() bool { desktops, err = store.Open(p.dir).Desktops(); return len(desktops) > 0 })
	if err != nil || len(desktops) != 1 || desktops[0].Name != "Studio" || desktops[0].Addr != d.Addr {
		t.Fatalf("kept %+v, %v", desktops, err)
	}
}

// The device token is kept in the platform's secure store, readable after the first unlock and on this device alone, and not in desktops.json; the phone started again reads it from there, and unpairing deletes it.
func TestTheTokenIsKeptInTheSecureStore(t *testing.T) {
	d := newDesktop(t)
	p := startPhone(t, "")
	p.pair(d)
	account := store.TokenAccount(store.DesktopID(d.Fingerprint))
	var token string
	var accessible core.SecretAccessible
	p.app.WaitUntil(wait, func() bool { token, accessible = secretOf(p, account); return token != "" })
	if accessible != core.SecretAfterFirstUnlockThisDeviceOnly {
		t.Errorf("the token is kept with accessibility %v, want after the first unlock, this device only", accessible)
	}
	p.app.WaitUntil(wait, func() bool { kept, _ := store.Open(p.dir).Desktops(); return len(kept) == 1 })
	if data, err := os.ReadFile(filepath.Join(p.dir, "desktops.json")); err != nil || bytes.Contains(data, []byte(token)) {
		t.Fatalf("desktops.json holds the token: %v", err)
	}

	p = startPhone(t, p.dir, nsgotest.Secrets(map[string]string{account: token}))
	p.app.WaitFor(talkRow(store.DesktopID(d.Fingerprint)), wait)
	id := store.DesktopID(d.Fingerprint)
	p.openComputers()
	// Unpair's Cancel keeps the computer.
	p.app.Press("id:computer-unpair")
	p.app.Answer(0)
	if !p.app.Exists("id:settings-computer-"+id) || func() string { token, _ := secretOf(p, account); return token }() == "" {
		t.Fatal("cancelling Unpair unpaired it")
	}
	p.app.Press("id:computer-unpair")
	p.app.Answer(1)
	p.app.WaitUntil(wait, func() bool { token, _ := secretOf(p, account); return token == "" })
	p.app.WaitForGone("id:settings-computer-"+id, wait)
	// With nothing paired, the Library offers to pair again.
	p.openTab(filmsTab, msgs.FilmsTitle())
	p.app.WaitFor("id:films-unpaired", wait)
}

// secretOf is what the phone's secure store keeps for account, and how it was written.
func secretOf(p *phone, account string) (string, core.SecretAccessible) {
	for item, v := range p.app.Secrets.Items() {
		if item.Account == account {
			return v, item.Accessible
		}
	}
	return "", core.SecretAccessibleDefault
}

// A wrong code is refused with the desktop's reason, and the right one pairs after it.
func TestPairingWithAWrongCodeSaysSo(t *testing.T) {
	d := newDesktop(t)
	p := startPhone(t, "")
	p.startPairing()
	p.fillPairing(d.Addr.String(), "654321")
	p.app.Press("id:pair-continue")
	p.app.WaitFor("id:pair-compare", wait)
	p.app.Press("id:pair-confirm")
	if got := p.app.WaitFor("id:pair-problem", wait).Text; got != msgs.PairErrWrongCode() {
		t.Fatalf("problem %q, want %q", got, msgs.PairErrWrongCode())
	}
	if d.Paired() != 0 {
		t.Fatal("a wrong code paired")
	}
	p.app.Clear("id:pair-code >> role:textbox")
	p.app.Type("id:pair-code >> role:textbox", "123456")
	p.app.Press("id:pair-continue")
	p.app.WaitFor("id:pair-compare", wait)
	p.app.Press("id:pair-confirm")
	p.finishPairing(d)
}

// A pasted kando://pair link pairs without asking when the desktop's certificate is the one it names, and is refused when it is not.
func TestAPairingLinkChecksTheFingerprintItself(t *testing.T) {
	d := newDesktop(t)
	other := sync.Fingerprint([]byte("another desktop's certificate"))
	p := startPhone(t, "")
	p.startPairing()
	p.app.WaitFor("id:pair-address", wait)
	p.app.Type("id:pair-address", sync.PairURL{Addrs: []netip.AddrPort{d.Addr}, Code: "123456", Fingerprint: other, Name: "Studio"}.String())
	p.app.Press("id:pair-continue")
	if got := p.app.WaitFor("id:pair-problem", wait).Text; got != msgs.PairErrFingerprint() {
		t.Fatalf("problem %q, want %q", got, msgs.PairErrFingerprint())
	}
	if d.Paired() != 0 {
		t.Fatal("the phone paired with a desktop whose certificate the link does not name")
	}
	p.app.Clear("id:pair-address")
	p.app.Type("id:pair-address", sync.PairURL{Addrs: []netip.AddrPort{d.Addr}, Code: "123456", Fingerprint: d.Fingerprint, Name: "Studio"}.String())
	p.app.Press("id:pair-continue")
	p.finishPairing(d)
	if p.app.Exists("id:pair-compare") {
		t.Fatal("a link's fingerprint was shown to compare")
	}
}

// Nothing listening at the address says the phone cannot reach it.
func TestPairingWithNoDesktopThere(t *testing.T) {
	d := newDesktop(t)
	addr := d.Addr.String()
	p := startPhone(t, "")
	d.Close()
	p.startPairing()
	p.fillPairing(addr, "123456")
	p.app.Press("id:pair-continue")
	if got, want := p.app.WaitFor("id:pair-problem", wait).Text, msgs.PairErrUnreachable(addr); got != want {
		t.Fatalf("problem %q, want %q", got, want)
	}
}

func TestParseTargetTakesAnAddressOrALink(t *testing.T) {
	if _, ok := pair.ParseTarget("192.168.1.20", "123456"); ok {
		t.Error("an address without a port was taken")
	}
	if _, ok := pair.ParseTarget("studio.local:47123", "123456"); ok {
		t.Error("a host name was taken")
	}
	got, ok := pair.ParseTarget(" 192.168.1.20:47123 ", "123456")
	if !ok || len(got.Addrs) != 1 || got.Addrs[0].String() != "192.168.1.20:47123" || got.Code != "123456" || got.Expected != nil {
		t.Errorf("typed: %+v %v", got, ok)
	}
	link := sync.PairURL{Addrs: []netip.AddrPort{netip.MustParseAddrPort("10.0.0.2:1"), netip.MustParseAddrPort("192.168.1.20:47123")}, Code: "000111", Fingerprint: sync.Fingerprint([]byte("c")), Name: "Mac"}.String()
	got, ok = pair.ParseTarget(link, "")
	if !ok || len(got.Addrs) != 2 || got.Code != "000111" || got.Expected == nil {
		t.Errorf("link: %+v %v", got, ok)
	}
}

// qrPhoto is a JPEG of text's QR code on a grey page, as the camera takes a photo of the computer's screen.
func qrPhoto(t *testing.T, text string) []byte {
	t.Helper()
	code, err := qrcode.NewQRCodeWriter().Encode(text, gozxing.BarcodeFormat_QR_CODE, 300, 300, nil)
	if err != nil {
		t.Fatal(err)
	}
	photo := image.NewRGBA(image.Rect(0, 0, 1200, 900))
	draw.Draw(photo, photo.Bounds(), image.NewUniform(color.Gray{Y: 120}), image.Point{}, draw.Src)
	draw.Draw(photo, image.Rect(450, 300, 750, 600), code, image.Point{}, draw.Src)
	var b bytes.Buffer
	if err := jpeg.Encode(&b, photo, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// startScanningPhone starts a new phone whose camera reads codes as scan answers, at its pairing step.
func startScanningPhone(t *testing.T, scan func(nsgotest.CodeScan) (*core.ScannedCode, error)) *phone {
	t.Helper()
	p := startPhone(t, "", nsgotest.Environment(core.Environment{Locale: lang, Camera: true, ScanCode: true}))
	p.app.ScanCode = scan
	p.startPairing()
	return p
}

// Scanning the QR code the desktop shows pairs as its link does, checking the fingerprint the code names without asking, and asks the camera for QR codes alone, taking no photo.
func TestScanningTheDesktopsQRCodeLivePairs(t *testing.T) {
	d := newDesktop(t)
	link := sync.PairURL{Addrs: []netip.AddrPort{d.Addr}, Code: "123456", Fingerprint: d.Fingerprint, Name: "Studio"}.String()
	p := startScanningPhone(t, func(nsgotest.CodeScan) (*core.ScannedCode, error) {
		return &core.ScannedCode{Kind: core.CodeQR, Text: link}, nil
	})
	p.app.WaitFor("id:pair-scan", wait)
	p.app.Press("id:pair-scan")
	p.finishPairing(d)
	if len(p.app.Scans) != 1 || len(p.app.Photos) != 0 {
		t.Fatalf("%d scans and %d photos, want 1 scan and no photo", len(p.app.Scans), len(p.app.Photos))
	}
	if r := p.app.Scans[0]; !r.Wants(core.CodeQR) || r.Wants(core.CodeEAN13) {
		t.Fatalf("the scan asks for %v, want QR codes alone", r.Kinds)
	}
	if d.Paired() != 1 {
		t.Fatalf("%d devices paired, want 1", d.Paired())
	}
}

// A scanned code that is not a pairing code says so; a scanner that fails says the camera could not open; closing the scanner changes nothing. None of them pairs.
func TestScanningSomethingElseSaysSo(t *testing.T) {
	d := newDesktop(t)
	answer := func(nsgotest.CodeScan) (*core.ScannedCode, error) {
		return &core.ScannedCode{Kind: core.CodeQR, Text: "https://example.com"}, nil
	}
	p := startScanningPhone(t, func(r nsgotest.CodeScan) (*core.ScannedCode, error) { return answer(r) })
	p.app.WaitFor("id:pair-scan", wait)
	p.app.Press("id:pair-scan")
	if got := p.app.WaitFor("id:pair-problem", wait).Text; got != msgs.PairErrNotPairQR() {
		t.Fatalf("problem %q, want %q", got, msgs.PairErrNotPairQR())
	}
	answer = func(nsgotest.CodeScan) (*core.ScannedCode, error) { return nil, errors.New("denied") }
	p.app.Press("id:pair-scan")
	p.app.WaitUntil(wait, func() bool {
		vs := p.app.Query("id:pair-problem")
		return len(vs) == 1 && vs[0].Text == msgs.PairErrCamera()
	})
	answer = func(nsgotest.CodeScan) (*core.ScannedCode, error) { return nil, nil }
	p.app.Press("id:pair-scan")
	p.app.Settle()
	if len(p.app.Scans) != 3 {
		t.Fatalf("%d scans asked for, want 3", len(p.app.Scans))
	}
	if p.app.Exists("id:pair-problem") {
		t.Fatalf("a closed scanner shows a problem: %q", p.app.Find("id:pair-problem").Text)
	}
	if p.app.Find("id:pair-scan").Props.Disabled {
		t.Fatal("scanning stays busy after the scanner was closed")
	}
	if d.Paired() != 0 {
		t.Fatal("the phone paired from no pairing code")
	}
}

// startCameraPhone starts a new phone with a camera that takes photos but reads no codes, as those photos gives, at its pairing step.
func startCameraPhone(t *testing.T, photos func() []byte) *phone {
	t.Helper()
	p := startPhone(t, "", nsgotest.Environment(core.Environment{Locale: lang, Camera: true}))
	p.app.TakePhoto = func(nsgotest.ImagePick) (*core.PickedFile, error) {
		return &core.PickedFile{Name: "IMG_0001.jpg", MIME: "image/jpeg", Data: photos()}, nil
	}
	p.startPairing()
	return p
}

// On a phone whose camera only takes photos, a photo of the QR code the desktop shows pairs as its link does, checking the fingerprint the code names without asking.
func TestScanningTheDesktopsQRCodePairs(t *testing.T) {
	d := newDesktop(t)
	link := sync.PairURL{Addrs: []netip.AddrPort{d.Addr}, Code: "123456", Fingerprint: d.Fingerprint, Name: "Studio"}.String()
	p := startCameraPhone(t, func() []byte { return qrPhoto(t, link) })
	p.app.WaitFor("id:pair-scan", wait)
	p.app.Press("id:pair-scan")
	p.finishPairing(d)
	if len(p.app.Photos) != 1 {
		t.Fatalf("%d photos taken, want 1", len(p.app.Photos))
	}
	if d.Paired() != 1 {
		t.Fatalf("%d devices paired, want 1", d.Paired())
	}
}

// A photo with no QR code, or with one that is not a pairing code, says so and pairs with nothing.
func TestScanningAnotherPhotoSaysSo(t *testing.T) {
	d := newDesktop(t)
	photo := qrPhoto(t, "https://example.com")
	p := startCameraPhone(t, func() []byte { return photo })
	p.app.WaitFor("id:pair-scan", wait)
	p.app.Press("id:pair-scan")
	if got := p.app.WaitFor("id:pair-problem", wait).Text; got != msgs.PairErrNotPairQR() {
		t.Fatalf("problem %q, want %q", got, msgs.PairErrNotPairQR())
	}
	photo = []byte("not a photo")
	p.app.Press("id:pair-scan")
	p.app.WaitUntil(wait, func() bool {
		vs := p.app.Query("id:pair-problem")
		return len(vs) == 1 && vs[0].Text == msgs.PairErrNoQR()
	})
	if d.Paired() != 0 {
		t.Fatal("the phone paired from a photo of no pairing code")
	}
}

// Where the phone has no camera the pairing page offers no scanning: its steps end in typing the address and code, with no QR code beside them.
func TestNoCameraNoScanning(t *testing.T) {
	p := startPhone(t, "")
	p.startPairing()
	p.app.WaitFor("id:pair-address", wait)
	if p.app.Exists("id:pair-scan") || p.app.Exists("id:pair-steps-qr") {
		t.Fatal("the pairing page offers scanning without a camera")
	}
	if got := p.texts("id:pair-steps"); !strings.HasSuffix(got, msgs.PairStepEnter()) {
		t.Fatalf("the steps read %q, want them to end in %q", got, msgs.PairStepEnter())
	}
}

// A phone that reads codes shows a QR code beside the steps, as the design does, and the steps end in scanning the computer's.
func TestTheStepsShowTheQRCodeToScan(t *testing.T) {
	p := startScanningPhone(t, nil)
	p.app.WaitFor("id:pair-scan", wait)
	if !p.app.Exists("id:pair-steps-qr") || !p.app.Find("id:pair-steps-qr").Props.ElementsHidden {
		t.Fatal("the steps have no QR code beside them, or assistive technology reads it")
	}
	if got := p.texts("id:pair-steps"); !strings.HasSuffix(got, msgs.PairStepScan()) {
		t.Fatalf("the steps read %q, want them to end in %q", got, msgs.PairStepScan())
	}
}

// A camera that fails says so, and pairs with nothing.
func TestScanningWithAFailingCameraSaysSo(t *testing.T) {
	d := newDesktop(t)
	p := startPhone(t, "", nsgotest.Environment(core.Environment{Locale: lang, Camera: true}))
	p.app.TakePhoto = func(nsgotest.ImagePick) (*core.PickedFile, error) { return nil, errors.New("denied") }
	p.startPairing()
	p.app.WaitFor("id:pair-scan", wait)
	p.app.Press("id:pair-scan")
	if got := p.app.WaitFor("id:pair-problem", wait).Text; got != msgs.PairErrCamera() {
		t.Fatalf("problem %q, want %q", got, msgs.PairErrCamera())
	}
	if d.Paired() != 0 {
		t.Fatal("the phone paired though the camera failed")
	}
}

// Cancelling the photo leaves the pairing page as it was, ready to scan again.
func TestCancellingTheScanChangesNothing(t *testing.T) {
	d := newDesktop(t)
	p := startPhone(t, "", nsgotest.Environment(core.Environment{Locale: lang, Camera: true}))
	p.app.TakePhoto = func(nsgotest.ImagePick) (*core.PickedFile, error) { return nil, nil }
	p.startPairing()
	p.app.WaitFor("id:pair-scan", wait)
	p.app.Press("id:pair-scan")
	p.app.Settle()
	if len(p.app.Photos) != 1 {
		t.Fatalf("%d photos asked for, want 1", len(p.app.Photos))
	}
	if p.app.Exists("id:pair-problem") {
		t.Fatalf("a cancelled photo shows a problem: %q", p.app.Find("id:pair-problem").Text)
	}
	if p.app.Find("id:pair-scan").Props.Disabled {
		t.Fatal("scanning stays busy after the photo was cancelled")
	}
	if d.Paired() != 0 {
		t.Fatal("the phone paired from a cancelled photo")
	}
}

// A phone that may not use the local network says so before it tries the computer, and offers the Settings that turn it on; a link with the computer's relay pairs through it all the same.
func TestPairingWithTheLocalNetworkRefusedOpensSettings(t *testing.T) {
	d := newDesktop(t)
	p := startPhone(t, "")
	p.app.PermissionAnswers[core.PermissionLocalNetwork] = core.PermissionDenied
	p.startPairing()
	p.app.WaitFor("id:pair-address", wait)
	addr := d.Addr.String()
	p.app.Type("id:pair-address", sync.PairURL{Addrs: []netip.AddrPort{d.Addr}, Code: "123456", Fingerprint: d.Fingerprint, Name: "Studio"}.String())
	p.app.Press("id:pair-continue")
	if got, want := p.app.WaitFor("id:pair-problem", wait).Text, msgs.PairErrLocalNetwork(addr); got != want {
		t.Fatalf("problem %q, want %q", got, want)
	}
	if d.Paired() != 0 {
		t.Fatal("the phone reached the desktop with the local network refused")
	}
	p.app.Press("id:pair-settings")
	if got := p.app.PermissionSettings; len(got) != 1 || got[0] != core.PermissionLocalNetwork {
		t.Fatalf("settings opened %v, want the local network's", got)
	}
}
