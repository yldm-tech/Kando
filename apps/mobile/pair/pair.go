// Package pair is the page that pairs the phone with Kando on a computer: the address and the one-time code Settings › Phone Sync shows there, then, for an address typed by hand, the certificate's fingerprint in short to compare with the computer's before the phone trusts it. A kando://pair link, scanned from the QR code the computer shows or pasted into the address field, brings its code and fingerprint, which the phone checks without asking.
package pair

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/netip"
	"strconv"
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
	f := New(cx, s, func(store.Desktop) { done() }, func() core.Points { return ui.NavInset(cx) }, false)
	return look.NavPage(cx, base.NavPage{TitleFunc: func() string { return locales.PairTitle(cx) }, TestID: "pair-page", Children: []core.Node{
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{f.Body, look.ActionBar(cx, f.Actions)}},
	}}, ui.NavHeader{OnBack: back, Scroll: f.Scroll})
}

// Flow is pairing as a page lays it out: Body, the form or the fingerprint to compare, which fills the room it is given, and Actions, the button that goes on, for the page's foot; Scroll is the form's, for a header that follows it. Content is Body without its scroll view, for a page that scrolls the form with what is around it, as the introduction does. Comparing is whether the fingerprint is on show, and Sending whether the code is out: it cannot be called back then, so a page's own ways out wait too, as Cancel does.
type Flow struct {
	Body, Content, Actions core.Node
	Scroll                 *core.Ref
	Comparing, Sending     func() bool
}

// New is the pairing flow under cx, which a pairing under way ends with; done runs once the phone is paired, with the computer it paired with, and top is the room the fingerprint keeps clear of above it, which does not scroll. intro lays the flow out for the introduction's white page: its main button in the tall shape (look.Button's Tall), and its cards in the page grey rather than the fill a card on the grey page has.
func New(cx *core.Scope, s *session.Session, done func(store.Desktop), top func() core.Points, intro bool) Flow {
	colors := ui.ColorsOf(cx)
	tall, fill := intro, CardFill(cx, intro)
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

	// scanning is a photo of the computer's QR code being read.
	scanning := core.NewSignal(cx, false)
	busy := func() bool { return scanning.Get() || at.Get() == checking || at.Get() == pairing }
	// comparingNow is the fingerprint on show: while the user compares it, and while the code goes out after they confirmed it.
	comparingNow := func() bool { return at.Get() == comparing || (at.Get() == pairing && confirmed.Get()) }
	confirm := func() {
		target, _ := ParseTarget(address.Get(), code.Get())
		confirmed.Set(true)
		send(answer.Get(), target)
	}
	canScan := func() bool { return CanScan(cx) }
	// read goes on with the text of a QR code as with a link pasted, when it is a pairing link.
	read := func(text string) {
		if _, err := sync.ParsePairURL(text); err != nil {
			problem.Set(locales.PairErrNotPairQR(cx))
			return
		}
		address.Set(text)
		check()
	}
	// scan reads the QR code the computer shows as the camera sees it, or, on a phone that can only take a photo, from a photo of it.
	scan := func() {
		problem.Set("")
		if cx.Env().CanScanCode() {
			cx.ScanCode(core.ScanOptions{Kinds: []core.CodeKind{core.CodeQR}}, func(c *core.ScannedCode, err error) {
				if err != nil {
					problem.Set(locales.PairErrCamera(cx))
					return
				}
				if c != nil {
					read(c.Text)
				}
			})
			return
		}
		cx.TakePhoto(core.PhotoOptions{MaxDimension: photoSize}, func(photo *core.PickedFile, err error) {
			if err != nil {
				problem.Set(locales.PairErrCamera(cx))
				return
			}
			if photo == nil {
				return
			}
			scanning.Set(true)
			data := photo.Data
			go func() {
				text, err := ReadCode(data)
				cx.Post(func() {
					scanning.Set(false)
					if err != nil {
						problem.Set(locales.PairErrNoQR(cx))
						return
					}
					read(text)
				})
			}()
		})
	}
	scroll := &core.Ref{}
	fields := core.View{Style: core.Style{Padding: core.All(16), Gap: 24, MaxWidth: core.Pt(480), Width: core.Pct(100), AlignSelf: core.AlignCenter}, Children: []core.Node{
		Steps(cx, fill),
		core.Show{When: canScan, Then: func(cx *core.Scope) core.Node {
			return look.Button{Kind: look.Tinted, Icon: lucide.ScanQrCode, Text: func() string { return locales.PairScan(cx) }, TestID: "pair-scan",
				Busy: scanning.Get, BusyLabel: func() string { return locales.PairScanning(cx) }, Disabled: busy, OnPress: scan}
		}},
		field(cx, locales.PairAddress(cx), ui.Input{TestID: "pair-address", Label: locales.PairAddress(cx), Value: address.Get, OnChange: func(v string) { address.Set(v); problem.Set("") },
			Placeholder: locales.PairAddressExample(cx), KeyboardType: core.KeyboardURL, AutoCapitalize: core.AutoCapitalizeNone, NoAutoCorrect: true, Disabled: busy,
			Style: core.Style{Height: core.Pt(50), FontSize: 17, CornerRadius: 12, Padding: core.Symmetric(0, 14)}}),
		field(cx, locales.PairCode(cx), codeInput(cx, code.Get, func(v string) { code.Set(v); problem.Set("") }, busy)),
		core.Show{When: func() bool { return problem.Get() != "" }, Then: func(cx *core.Scope) core.Node {
			return core.Text{TestID: "pair-problem", ContentFunc: problem.Get, LiveRegion: core.LiveRegionPolite, Style: look.Subhead, StyleFunc: ui.Colored(colors.Danger)}
		}},
		core.Show{When: busy, Then: func(cx *core.Scope) core.Node { return connecting(cx, fill) }},
	}}
	form := core.Scroll{Ref: scroll, Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{fields}}
	compare := core.View{TestID: "pair-compare", Style: core.Style{FlexGrow: 1, JustifyContent: core.JustifyCenter, AlignItems: core.AlignCenter, Gap: 20},
		// The comparison does not scroll, so it keeps clear of iOS's header itself.
		StyleFunc: func() core.Style { return core.Style{Padding: core.Edges{Top: top(), Right: 24, Left: 24}} },
		Children: []core.Node{
			ui.Icon(lucide.ShieldCheck, 40, colors.Accent),
			core.Text{ContentFunc: func() string { return locales.PairCompareTitle(cx) }, Role: core.RoleHeader, Style: look.Heading.Merge(core.Style{TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(colors.Text)},
			core.View{Style: core.Style{CornerRadius: 16, Padding: core.Symmetric(20, 24)}, StyleFunc: ui.Filled(fill), Children: []core.Node{
				core.Text{TestID: "pair-fingerprint", ContentFunc: func() string { return sync.ShortFingerprint(answer.Get().fingerprint) },
					Style: core.Style{FontSize: 30, LineHeight: 38, FontFamily: ui.Mono, FontWeight: core.FontWeightSemibold, LetterSpacing: 2, TextAlign: core.TextAlignCenter}, StyleFunc: ui.Colored(colors.Text)},
			}},
		}}
	actions := core.Show{When: comparingNow,
		Then: func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
				look.Button{Kind: look.Primary, Tall: tall, Text: func() string { return locales.PairConfirm(cx) }, TestID: "pair-confirm", Busy: func() bool { return at.Get() == pairing },
					BusyLabel: func() string { return locales.PairConnecting(cx) }, OnPress: confirm},
				// Once the code goes out it cannot be called back: the computer may take it whether or not the phone waits for the answer, so Cancel waits too.
				look.Button{Kind: look.Plain, Text: func() string { return locales.CommonCancel(cx) }, TestID: "pair-reject", Disabled: func() bool { return at.Get() == pairing }, OnPress: func() { at.Set(editing) }},
			}}
		},
		Else: func(cx *core.Scope) core.Node {
			// The connecting card over it says the phone is reaching the computer, so the button only waits.
			return look.Button{Kind: look.Primary, Tall: tall, Text: func() string { return locales.PairContinue(cx) }, TestID: "pair-continue",
				Disabled: func() bool { return busy() || !ready() }, OnPress: check}
		}}
	body := core.Show{When: comparingNow, Then: func(*core.Scope) core.Node { return compare }, Else: func(*core.Scope) core.Node { return form }}
	content := core.Show{When: comparingNow, Then: func(*core.Scope) core.Node { return compare }, Else: func(*core.Scope) core.Node { return fields }}
	return Flow{Body: body, Content: content, Actions: actions, Scroll: scroll, Comparing: comparingNow, Sending: func() bool { return at.Get() == pairing }}
}

// CardFill is the fill of the flow's cards: on the introduction's white page the grouped page's grey, as the design draws them, and on a page of that grey the fill a card has on it.
func CardFill(cx *core.Scope, intro bool) func() core.Color {
	colors := ui.ColorsOf(cx)
	if intro {
		return colors.Background
	}
	return colors.Inset
}

// Steps is how to pair, as a card of numbered steps in fill: open Kando on the computer, its Settings › Phone Sync, and scan the QR code it shows there, beside a picture of one, as the design draws it, on a phone that can read it (CanScan); else enter the address and code it shows. The phone reads the computer's code, so the design's waiting for the computer to scan is the card that says the phone is reaching the computer once it has (connecting).
func Steps(cx *core.Scope, fill func() core.Color) core.Node {
	colors := ui.ColorsOf(cx)
	// The design's steps are a grey between the ink and the muted text (#3A3A3F on white).
	ink := func() core.Color { return ui.Mix(colors.Text(), colors.Muted(), 0.38) }
	scans := CanScan(cx)
	last := locales.PairStepEnter
	if scans {
		last = locales.PairStepScan
	}
	lines := []func(*core.Scope) string{locales.PairStepOpen, locales.PairStepSettings, last}
	var rows []core.Node
	for i, line := range lines {
		text := look.Caption.Merge(core.Style{LineHeight: 21})
		rows = append(rows, core.View{Style: core.Style{Direction: core.Row, Gap: 6}, Children: []core.Node{
			core.Text{Content: strconv.Itoa(i+1) + ".", Style: text.Merge(core.Style{MinWidth: core.Pt(14), FontVariant: core.FontTabularNums}), StyleFunc: ui.Colored(ink)},
			core.Text{ContentFunc: func() string { return line(cx) }, Style: text.Merge(core.Style{FlexShrink: 1}), StyleFunc: ui.Colored(ink)},
		}})
	}
	steps := core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, JustifyContent: core.JustifyCenter}, Children: rows}
	children := []core.Node{steps}
	if scans {
		// The picture of a QR code is the design's 96-point white tile: the steps say what it is, so assistive technology skips it.
		code := core.View{TestID: "pair-steps-qr", AccessibilityElementsHidden: func() bool { return true },
			Style: core.Style{Width: core.Pt(96), Height: core.Pt(96), CornerRadius: 12, BorderCurve: core.BorderCurveContinuous, Background: core.RGB(0xFFFFFF),
				AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, FlexShrink: 0},
			Children: []core.Node{ui.Icon(lucide.QrCode, 80, func() core.Color { return core.RGB(0x000000) })}}
		children = []core.Node{code, steps}
	}
	return core.View{TestID: "pair-steps", Role: core.RoleGroup, LabelFunc: func() string { return locales.PairSteps(cx) },
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 16, Padding: core.All(14), CornerRadius: 16, BorderCurve: core.BorderCurveContinuous}, StyleFunc: ui.Filled(fill), Children: children}
}

// CanScan is whether the phone can read the computer's QR code: from the camera's live picture (Scope.ScanCode), or else from a photo of it.
func CanScan(cx *core.Scope) bool { return cx.Env().CanScanCode() || cx.Env().CanTakePhoto() }

// connecting is the card in fill that says the phone is reaching the computer, while it does.
func connecting(cx *core.Scope, fill func() core.Color) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{TestID: "pair-connecting", Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Symmetric(12, 14), CornerRadius: 16, BorderCurve: core.BorderCurveContinuous},
		StyleFunc: ui.Filled(fill),
		Children: []core.Node{
			ui.Spinner{Size: 22, Tint: colors.Accent},
			core.Text{ContentFunc: func() string { return locales.PairConnecting(cx) }, LiveRegion: core.LiveRegionPolite, Style: core.Style{FontSize: 15, LineHeight: 20, FontWeight: core.FontWeightBold, FlexShrink: 1}, StyleFunc: ui.Colored(colors.Text)},
		}}
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
// TODO(nsgo#1075): ui.OtpInput now takes its cells' side (Size, nsgo#482), but its cells are square, in the body's type, RadiusControl round, under a 16pt caret (ui/otpinput.go:44-50), where the design's are 44 by 52, 22pt semibold tabular digits, 10 round, under a 24pt caret, so the code is base.OtpInput in the app's own cells; use ui.OtpInput once it takes a cell's width and height, type and corner radius and the caret's height https://github.com/nsgo-dev/nsgo/issues/1075
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
