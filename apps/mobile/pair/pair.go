// Package pair is the page that pairs the phone with Kando on a computer: the address and the one-time code Settings › Phone Sync shows there, then, for an address typed by hand, the certificate's fingerprint in short to compare with the computer's before the phone trusts it. A kando://pair link pasted into the address field brings its code and fingerprint, which the phone checks without asking. QR scanning waits for NSGo (nsgo-dev/nsgo#429).
package pair

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// stage is how far a pairing has gone.
type stage uint8

const (
	editing stage = iota
	// checking connects to the address for the certificate's fingerprint.
	checking
	// comparing shows the fingerprint for the user to compare with the computer's.
	comparing
	// pairing sends the code.
	pairing
)

// connectTimeout is how long the phone waits for a computer to answer.
const connectTimeout = 8 * time.Second

// Target is what the address field says to pair with: the addresses to try, the code, and the fingerprint a pasted link names, nil for an address typed by hand.
type Target struct {
	Addrs    []netip.AddrPort
	Code     string
	Expected []byte
}

// ParseTarget reads the address field and the code: an IP address with its port, as 192.168.1.20:47123, or a kando://pair link, whose own code replaces code. ok is false for anything else.
func ParseTarget(address, code string) (Target, bool) {
	address = strings.TrimSpace(address)
	if u, err := sync.ParsePairURL(address); err == nil {
		return Target{Addrs: u.Addrs, Code: u.Code, Expected: u.Fingerprint}, true
	}
	addr, err := netip.ParseAddrPort(address)
	if err != nil || addr.Port() == 0 {
		return Target{}, false
	}
	return Target{Addrs: []netip.AddrPort{addr}, Code: code}, true
}

// Page is the pairing page; done runs once the phone is paired, back leaves it.
func Page(cx *core.Scope, s *session.Session, done, back func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	address := core.NewSignal(cx, "")
	code := core.NewSignal(cx, "")
	at := core.NewSignal(cx, editing)
	problem := core.NewSignal(cx, "")
	// answer is where the computer answered and its certificate's fingerprint, once checked.
	answer := core.NewSignalFunc(cx, found{}, nil)
	// page ends as the page goes, which stops a pairing under way. It is the page's own, read as the page is built: a handler's cx is the region of the button pressed, which the page's stages replace as a pairing goes on.
	page := cx.Context()

	ready := func() bool {
		t, ok := ParseTarget(address.Get(), code.Get())
		return ok && sync.ValidCode(t.Code)
	}
	send := func(f found, target Target) {
		at.Set(pairing)
		go func() {
			ctx, cancel := context.WithTimeout(page, connectTimeout)
			defer cancel()
			c := sync.NewClient(f.addr, f.fingerprint, "")
			defer c.Close()
			resp, err := c.Pair(ctx, sync.PairRequest{Code: target.Code, Device: s.Device(), Platform: s.Platform()})
			cx.Post(func() {
				if err != nil {
					problem.Set(pairProblem(cx, err, f.addr))
					at.Set(editing)
					return
				}
				s.AddDesktop(store.Desktop{ID: store.DesktopID(f.fingerprint), Name: resp.Desktop, Addr: f.addr, Fingerprint: f.fingerprint, Token: resp.Token, DeviceID: resp.DeviceID})
				done()
			})
		}()
	}
	check := func() {
		target, ok := ParseTarget(address.Get(), code.Get())
		if !ok {
			problem.Set(locales.PairErrAddress(cx))
			return
		}
		problem.Set("")
		at.Set(checking)
		go func() {
			f, err := reach(page, target.Addrs)
			cx.Post(func() {
				switch {
				case err != nil:
					problem.Set(locales.PairErrUnreachable(cx, target.Addrs[0].String()))
					at.Set(editing)
				case target.Expected != nil && !bytes.Equal(target.Expected, f.fingerprint):
					problem.Set(locales.PairErrFingerprint(cx))
					at.Set(editing)
				case target.Expected != nil:
					answer.Set(f)
					send(f, target)
				default:
					answer.Set(f)
					at.Set(comparing)
				}
			})
		}()
	}

	errorText := core.Show{When: func() bool { return problem.Get() != "" }, Then: func(cx *core.Scope) core.Node {
		return core.Text{TestID: "pair-problem", ContentFunc: problem.Get, LiveRegion: core.LiveRegionPolite, Style: look.Body, StyleFunc: ui.Colored(colors.Danger)}
	}}
	busy := func() bool { return at.Get() == checking || at.Get() == pairing }
	form := core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		core.Text{ContentFunc: func() string { return locales.PairHint(cx) }, Style: look.Body, StyleFunc: ui.Colored(colors.Muted)},
		field(cx, locales.PairAddress(cx), ui.Input{TestID: "pair-address", Label: locales.PairAddress(cx), Value: address.Get, OnChange: func(v string) { address.Set(v); problem.Set("") },
			Placeholder: locales.PairAddressExample(cx), KeyboardType: core.KeyboardURL, AutoCapitalize: core.AutoCapitalizeNone, NoAutoCorrect: true, Disabled: busy}),
		field(cx, locales.PairCode(cx), ui.OtpInput{TestID: "pair-code", Label: locales.PairCode(cx), Length: sync.CodeDigits, Groups: 2, Value: code.Get, OnChange: func(v string) { code.Set(v); problem.Set("") }, Disabled: busy}),
		errorText,
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12}, Children: []core.Node{
			ui.Button{TestID: "pair-continue", Text: locales.PairContinue(cx), Size: ui.SizeLarge, Disabled: func() bool { return busy() || !ready() }, OnPress: check},
			core.Show{When: busy, Then: func(cx *core.Scope) core.Node { return ui.Spinner{Label: locales.PairConnecting(cx)} }},
		}},
	}}
	compare := core.View{TestID: "pair-compare", Style: core.Style{Gap: 16}, Children: []core.Node{
		core.Text{ContentFunc: func() string { return locales.PairCompareTitle(cx) }, Role: core.RoleHeader, Style: look.Title, StyleFunc: ui.Colored(colors.Text)},
		core.Text{TestID: "pair-fingerprint", ContentFunc: func() string { return sync.ShortFingerprint(answer.Get().fingerprint) },
			Style: core.Style{FontSize: 26, LineHeight: 34, FontFamily: ui.Mono, FontWeight: core.FontWeightSemibold, LetterSpacing: 1}, StyleFunc: ui.Colored(colors.Text)},
		core.Text{ContentFunc: func() string { return locales.PairCompareHint(cx) }, Style: look.Body, StyleFunc: ui.Colored(colors.Muted)},
		core.View{Style: core.Style{Direction: core.Row, Gap: 12}, Children: []core.Node{
			ui.Button{TestID: "pair-confirm", Text: locales.PairConfirm(cx), Size: ui.SizeLarge, OnPress: func() {
				target, _ := ParseTarget(address.Get(), code.Get())
				send(answer.Get(), target)
			}},
			ui.Button{TestID: "pair-reject", Text: locales.CommonCancel(cx), Variant: ui.ButtonSecondary, Size: ui.SizeLarge, OnPress: func() { at.Set(editing) }},
		}},
	}}
	return ui.NavPage(cx, base.NavPage{TitleFunc: func() string { return locales.PairTitle(cx) }, TestID: "pair-page", Children: []core.Node{
		core.Scroll{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
			core.View{Style: core.Style{Padding: core.All(20), PaddingEnv: core.EnvEdges{Bottom: core.EnvKeyboard}, MaxWidth: core.Pt(560), Width: core.Pct(100), AlignSelf: core.AlignCenter}, Children: []core.Node{
				core.Show{When: func() bool { return at.Get() == comparing }, Then: func(*core.Scope) core.Node { return compare }, Else: func(*core.Scope) core.Node { return form }},
			}},
		}},
	}}, ui.NavHeader{OnBack: back})
}

// field is a labeled control of the form.
func field(cx *core.Scope, label string, control core.Node) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{Gap: 6}, Children: []core.Node{
		core.Text{Content: label, Style: look.Section, StyleFunc: ui.Colored(colors.Muted)},
		control,
	}}
}

// found is a computer that answered: where, and its certificate's fingerprint.
type found struct {
	addr        netip.AddrPort
	fingerprint []byte
}

// reach tries each address in turn for the computer's certificate, and returns the first that answers.
func reach(ctx context.Context, addrs []netip.AddrPort) (found, error) {
	var err error
	for _, a := range addrs {
		tctx, cancel := context.WithTimeout(ctx, connectTimeout)
		var fp []byte
		fp, err = sync.FetchFingerprint(tctx, a)
		cancel()
		if err == nil {
			return found{a, fp}, nil
		}
	}
	return found{}, err
}

// pairProblem is what the page says when pairing with the computer at addr failed for err.
func pairProblem(cx *core.Scope, err error, addr netip.AddrPort) string {
	var e *sync.Error
	if errors.As(err, &e) {
		switch e.Code {
		case sync.CodeWrongCode:
			return locales.PairErrWrongCode(cx)
		case sync.CodeCodeUsed:
			return locales.PairErrCodeUsed(cx)
		case sync.CodeCodeExpired:
			return locales.PairErrCodeExpired(cx)
		case sync.CodeNoCode:
			return locales.PairErrNoCode(cx)
		case sync.CodeTooMany:
			return locales.PairErrTooMany(cx)
		}
		if e.Status == http.StatusTooManyRequests {
			return locales.PairErrTooMany(cx)
		}
		return locales.PairErrFailed(cx)
	}
	if errors.Is(err, sync.ErrFingerprint) {
		return locales.PairErrFingerprint(cx)
	}
	return locales.PairErrUnreachable(cx, addr.String())
}
