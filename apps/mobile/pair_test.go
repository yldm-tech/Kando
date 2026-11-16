package main

import (
	"net/netip"
	"testing"

	"kando/apps/mobile/pair"
	"kando/apps/mobile/store"
	"kando/packages/sync"
)

// Typed by hand, the address shows the desktop certificate's fingerprint in short before the phone trusts it; once confirmed, the phone pairs and keeps the desktop.
func TestPairingShowsTheFingerprintBeforeTrustingIt(t *testing.T) {
	d := newDesktop(t)
	p := startPhone(t, "")
	p.app.WaitFor("id:home-welcome", wait)
	p.app.Press("id:home-pair")
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
	p.app.WaitFor("id:home-desktop-"+store.DesktopID(d.Fingerprint), wait)
	if d.Paired() != 1 {
		t.Fatalf("%d devices paired, want 1", d.Paired())
	}
	var desktops []store.Desktop
	var err error
	p.app.WaitUntil(wait, func() bool { desktops, err = store.Open(p.dir).Desktops(); return len(desktops) > 0 })
	if err != nil || len(desktops) != 1 || desktops[0].Name != "Studio" || desktops[0].Token == "" || desktops[0].Addr != d.Addr {
		t.Fatalf("kept %+v, %v", desktops, err)
	}
}

// A wrong code is refused with the desktop's reason, and the right one pairs after it.
func TestPairingWithAWrongCodeSaysSo(t *testing.T) {
	d := newDesktop(t)
	p := startPhone(t, "")
	p.app.WaitFor("id:home-welcome", wait)
	p.app.Press("id:home-pair")
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
	p.app.WaitFor("id:home-desktop-"+store.DesktopID(d.Fingerprint), wait)
}

// A pasted kando://pair link pairs without asking when the desktop's certificate is the one it names, and is refused when it is not.
func TestAPairingLinkChecksTheFingerprintItself(t *testing.T) {
	d := newDesktop(t)
	other := sync.Fingerprint([]byte("another desktop's certificate"))
	p := startPhone(t, "")
	p.app.WaitFor("id:home-welcome", wait)
	p.app.Press("id:home-pair")
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
	p.app.WaitFor("id:home-desktop-"+store.DesktopID(d.Fingerprint), wait)
	if p.app.Exists("id:pair-compare") {
		t.Fatal("a link's fingerprint was shown to compare")
	}
}

// Nothing listening at the address says the phone cannot reach it.
func TestPairingWithNoDesktopThere(t *testing.T) {
	d := newDesktop(t)
	addr := d.Addr.String()
	p := startPhone(t, "")
	p.app.WaitFor("id:home-welcome", wait)
	d.Close()
	p.app.Press("id:home-pair")
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
