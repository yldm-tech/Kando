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
	"github.com/nsgo-dev/nsgo/icons/lucide"
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
	f := New(cx, s, func(store.Desktop) { done() }, func() core.Points { return ui.NavInset(cx) })
	return ui.NavPage(cx, base.NavPage{TitleFunc: func() string { return locales.PairTitle(cx) }, TestID: "pair-page", Children: []core.Node{
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{f.Body, look.ActionBar(cx, f.Actions)}},
	}}, ui.NavHeader{OnBack: back, Scroll: f.Scroll})
}

// Flow is pairing as a page lays it out: Body, the form or the fingerprint to compare, which fills the room it is given, and Actions, the button that goes on, for the page's foot; Scroll is the form's, for a header that follows it.
type Flow struct {
	Body, Actions core.Node
	Scroll        *core.Ref
}

// New is the pairing flow under cx, which a pairing under way ends with; done runs once the phone is paired, with the computer it paired with, and top is the room the fingerprint keeps clear of above it, which does not scroll.
func New(cx *core.Scope, s *session.Session, done func(store.Desktop), top func() core.Points) Flow {
	colors := ui.ColorsOf(cx)
	address := core.NewSignal(cx, "")
	code := core.NewSignal(cx, "")
	at := core.NewSignal(cx, editing)
	problem := core.NewSignal(cx, "")
	// answer is where the computer answered and its certificate's fingerprint, once checked.
	answer := core.NewSignalFunc(cx, found{}, nil)
	// confirmed is a pairing the user confirmed after comparing the fingerprint, rather than one a link's fingerprint let through.
	confirmed := core.NewSignal(cx, false)
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
				d := store.Desktop{ID: store.DesktopID(f.fingerprint), Name: resp.Desktop, Addr: f.addr, Fingerprint: f.fingerprint, Token: resp.Token, DeviceID: resp.DeviceID}
				s.AddDesktop(d)
				done(d)
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
		confirmed.Set(false)
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

	busy := func() bool { return at.Get() == checking || at.Get() == pairing }
	// comparingNow is the fingerprint on show: while the user compares it, and while the code goes out after they confirmed it.
	comparingNow := func() bool { return at.Get() == comparing || (at.Get() == pairing && confirmed.Get()) }
	confirm := func() {
		target, _ := ParseTarget(address.Get(), code.Get())
		confirmed.Set(true)
		send(answer.Get(), target)
	}
	scroll := &core.Ref{}
	form := core.Scroll{Ref: scroll, Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
		core.View{Style: core.Style{Padding: core.All(16), Gap: 24, MaxWidth: core.Pt(480), Width: core.Pct(100), AlignSelf: core.AlignCenter}, Children: []core.Node{
			core.Text{ContentFunc: func() string { return locales.PairHint(cx) }, Style: look.Subhead, StyleFunc: ui.Colored(colors.Muted)},
			field(cx, locales.PairAddress(cx), ui.Input{TestID: "pair-address", Label: locales.PairAddress(cx), Value: address.Get, OnChange: func(v string) { address.Set(v); problem.Set("") },
				Placeholder: locales.PairAddressExample(cx), KeyboardType: core.KeyboardURL, AutoCapitalize: core.AutoCapitalizeNone, NoAutoCorrect: true, Disabled: busy,
				Style: core.Style{Height: core.Pt(50), FontSize: 17, CornerRadius: 12, Padding: core.Symmetric(0, 14)}}),
			field(cx, locales.PairCode(cx), codeInput(cx, code.Get, func(v string) { code.Set(v); problem.Set("") }, busy)),
			core.Show{When: func() bool { return problem.Get() != "" }, Then: func(cx *core.Scope) core.Node {
				return core.Text{TestID: "pair-problem", ContentFunc: problem.Get, LiveRegion: core.LiveRegionPolite, Style: look.Subhead, StyleFunc: ui.Colored(colors.Danger)}
			}},
		}},
	}}
	compare := core.View{TestID: "pair-compare", Style: core.Style{FlexGrow: 1, JustifyContent: core.JustifyCenter, AlignItems: core.AlignCenter, Gap: 20},
		// The comparison does not scroll, so it keeps clear of iOS's header itself.
		StyleFunc: func() core.Style { return core.Style{Padding: core.Edges{Top: top(), Right: 24, Left: 24}} },
		Children: []core.Node{
			ui.Icon(lucide.ShieldCheck, 40, colors.Accent),
			core.Text{ContentFunc: func() string { return locales.PairCompareTitle(cx) }, Role: core.RoleHeader, Style: look.Heading.Merge(core.Style{TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(colors.Text)},
			core.View{Style: core.Style{CornerRadius: 16, Padding: core.Symmetric(20, 24)}, StyleFunc: ui.Filled(colors.Inset), Children: []core.Node{
				core.Text{TestID: "pair-fingerprint", ContentFunc: func() string { return sync.ShortFingerprint(answer.Get().fingerprint) },
					Style: core.Style{FontSize: 30, LineHeight: 38, FontFamily: ui.Mono, FontWeight: core.FontWeightSemibold, LetterSpacing: 2, TextAlign: core.TextAlignCenter}, StyleFunc: ui.Colored(colors.Text)},
			}},
		}}
	actions := core.Show{When: comparingNow,
		Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
				look.Button{Kind: look.Primary, Text: func() string { return locales.PairConfirm(cx) }, TestID: "pair-confirm", Busy: func() bool { return at.Get() == pairing },
					BusyLabel: func() string { return locales.PairConnecting(cx) }, OnPress: confirm},
				// Once the code goes out it cannot be called back: the computer may take it whether or not the phone waits for the answer, so Cancel waits too.
				look.Button{Kind: look.Plain, Text: func() string { return locales.CommonCancel(cx) }, TestID: "pair-reject", Disabled: func() bool { return at.Get() == pairing }, OnPress: func() { at.Set(editing) }},
			}}
		},
		Else: func(cx *core.Scope) core.Node {
			return look.Button{Kind: look.Primary, Text: func() string { return locales.PairContinue(cx) }, TestID: "pair-continue", Busy: busy,
				BusyLabel: func() string { return locales.PairConnecting(cx) }, Disabled: func() bool { return !ready() }, OnPress: check}
		}}
	body := core.Show{When: comparingNow, Then: func(*core.Scope) core.Node { return compare }, Else: func(*core.Scope) core.Node { return form }}
	return Flow{Body: body, Actions: actions, Scroll: scroll}
}

// field is a labeled control of the form.
func field(cx *core.Scope, label string, control core.Node) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
		core.Text{Content: label, Style: look.Section.Merge(core.Style{Padding: core.Edges{Left: 4}}), StyleFunc: ui.Colored(colors.Muted)},
		control,
	}}
}

// codeInput is the field of the pairing code, in cells a phone's thumb and eye find at once.
// TODO(nsgo#482): ui.OtpInput's cells are ButtonHeight+8 square whatever its Style says (otpinput.go:35-39), so the code is base.OtpInput in the app's own cells; use ui.OtpInput once it takes a cell size https://github.com/nsgo-dev/nsgo/issues/482
func codeInput(cx *core.Scope, value func() string, change func(string), disabled func() bool) core.Node {
	colors := ui.ColorsOf(cx)
	return base.OtpInput{TestID: "pair-code", Label: locales.PairCode(cx), Length: sync.CodeDigits, Groups: 2, Value: value, OnChange: change, Disabled: disabled,
		Style:      core.Style{Direction: core.Row, AlignItems: core.AlignCenter, AlignSelf: core.AlignStart, Gap: 12},
		GroupStyle: core.Style{Direction: core.Row, Gap: 6},
		CellStyle: core.Style{Width: core.Pt(44), Height: core.Pt(52), CornerRadius: 10, BorderCurve: core.BorderCurveContinuous, BorderWidth: 1,
			FontSize: 22, FontWeight: core.FontWeightSemibold, FontVariant: core.FontTabularNums, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
			Disabled: ui.Dimmed()},
		CellStyleFunc: func() core.Style {
			return core.Style{Color: colors.Text(), Background: colors.Field(), BorderColor: colors.FieldBorder(), Focus: &core.Style{BorderColor: colors.Accent()}}
		},
		CaretStyle:     core.Style{Width: core.Pt(1.5), Height: core.Pt(24)},
		CaretStyleFunc: ui.Filled(colors.Accent)}
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
