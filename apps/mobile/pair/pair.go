// Package pair is the page that pairs the phone with Kando on a computer: the address and the one-time code Settings › Phone Sync shows there, then, for an address typed by hand, the certificate's fingerprint in short to compare with the computer's before the phone trusts it. A kando://pair link, scanned from the QR code the computer shows or pasted into the address field, brings its code and fingerprint, which the phone checks without asking, and, while the computer's storage relay is on, the code's pairing mailbox there: a phone that cannot reach the computer on its network pairs through it (sync.PairThroughRelay, yldm-tech/Kando#468) and follows the computer through the relay from then on.
package pair

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"syscall"
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

// relayTimeout is how long the phone waits for a pairing through the computer's relay, which the computer reads every sync.RelayPairEvery while it runs.
const relayTimeout = 2 * time.Minute

// RelayEvery is how often the phone reads the answer to a pairing through the relay, 0 for sync.RelayPairEvery. Tests shorten it.
var RelayEvery time.Duration

// Target is what the address field says to pair with: the addresses to try, the code, and the fingerprint a pasted link names, nil for an address typed by hand; Relay is the link's pairing mailbox, nil for none.
type Target struct {
	Addrs    []netip.AddrPort
	Code     string
	Expected []byte
	Relay    *sync.PairRelay
}

// ParseTarget reads the address field and the code: an IP address with its port, as 192.168.1.20:47123, or a kando://pair link, whose own code replaces code. ok is false for anything else.
func ParseTarget(address, code string) (Target, bool) {
	address = strings.TrimSpace(address)
	if u, err := sync.ParsePairURL(address); err == nil {
		return Target{Addrs: u.Addrs, Code: u.Code, Expected: u.Fingerprint, Relay: u.Relay}, true
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
	// localRefused is the problem on show being the local network the phone may not reach, which Settings turns on: the page offers to open them.
	localRefused := core.NewSignal(cx, false)
	// relaying is a pairing through the computer's relay under way.
	relaying := core.NewSignal(cx, false)

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
	// viaRelay pairs through the link's mailbox in the computer's relay, which a link brings with the fingerprint it names.
	viaRelay := func(target Target) {
		at.Set(pairing)
		relaying.Set(true)
		go func() {
			ctx, cancel := context.WithTimeout(page, relayTimeout)
			defer cancel()
			resp, joined, err := sync.PairThroughRelay(ctx, nil, *target.Relay, sync.PairRequest{Code: target.Code, Device: s.Device(), Platform: s.Platform()}, RelayEvery)
			cx.Post(func() {
				relaying.Set(false)
				if err != nil {
					problem.Set(relayProblem(cx, err))
					at.Set(editing)
					return
				}
				d := store.Desktop{ID: store.DesktopID(target.Expected), Name: resp.Desktop, Addr: target.Addrs[0], Fingerprint: target.Expected, Token: resp.Token, DeviceID: resp.DeviceID}
				if err := s.AdoptRelay(d.ID, resp.DeviceID, joined); err != nil {
					problem.Set(locales.PairErrFailed(cx))
					at.Set(editing)
					return
				}
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
		localRefused.Set(false)
		confirmed.Set(false)
		at.Set(checking)
		viaLink := target.Relay != nil && target.Expected != nil
		// iOS asks the user before the app first reaches the local network; asking here, before the first connection, shows its question at once and tells a refusal for what it is. Elsewhere the answer is not gated.
		cx.RequestPermission(core.PermissionLocalNetwork, func(st core.PermissionStatus) {
			if at.Get() != checking {
				return
			}
			if st == core.PermissionDenied {
				if viaLink {
					// The relay goes through the internet, which the setting does not keep from the app.
					viaRelay(target)
					return
				}
				problem.Set(locales.PairErrLocalNetwork(cx, target.Addrs[0].String()))
				localRefused.Set(true)
				at.Set(editing)
				return
			}
			go func() {
				f, err := reach(page, target.Addrs)
				cx.Post(func() {
					switch {
					case err != nil && viaLink:
						viaRelay(target)
					case err != nil:
						msg, refused := unreachable(cx, s, target, err)
						problem.Set(msg)
						localRefused.Set(refused)
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
		})
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
		localRefused.Set(false)
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
		field(cx, func() string { return locales.PairAddress(cx) }, ui.Input{TestID: "pair-address", LabelFunc: func() string { return locales.PairAddress(cx) }, Value: address.Get, OnChange: func(v string) { address.Set(v); problem.Set("") },
			PlaceholderFunc: func() string { return locales.PairAddressExample(cx) }, KeyboardType: core.KeyboardURL, AutoCapitalize: core.AutoCapitalizeNone, NoAutoCorrect: true, Disabled: busy,
			Style: core.Style{Height: core.Pt(50), FontSize: 17, CornerRadius: 12, Padding: core.Symmetric(0, 14)}}),
		field(cx, func() string { return locales.PairCode(cx) }, codeInput(cx, code.Get, func(v string) { code.Set(v); problem.Set("") }, busy)),
		core.Show{When: func() bool { return problem.Get() != "" }, Then: func(cx *core.Scope) core.Node {
			return core.Text{TestID: "pair-problem", ContentFunc: problem.Get, LiveRegion: core.LiveRegionPolite, Style: look.Subhead, StyleFunc: ui.Colored(colors.Danger)}
		}},
		core.Show{When: func() bool { return localRefused.Get() && problem.Get() != "" }, Then: func(cx *core.Scope) core.Node {
			return look.Button{Kind: look.Tinted, Icon: lucide.Settings, Text: func() string { return locales.PairOpenSettings(cx) }, TestID: "pair-settings",
				OnPress: func() { cx.OpenPermissionSettings(core.PermissionLocalNetwork, nil) }}
		}},
		core.Show{When: busy, Then: func(cx *core.Scope) core.Node { return connecting(cx, fill, relaying.Get) }},
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

// connecting is the card in fill that says the phone is reaching the computer, while it does, through the computer's relay while relaying.
func connecting(cx *core.Scope, fill func() core.Color, relaying func() bool) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{TestID: "pair-connecting", Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Symmetric(12, 14), CornerRadius: 16, BorderCurve: core.BorderCurveContinuous},
		StyleFunc: ui.Filled(fill),
		Children: []core.Node{
			ui.Spinner{Size: 22, Tint: colors.Accent},
			core.Text{ContentFunc: func() string {
				if relaying() {
					return locales.PairConnectingRelay(cx)
				}
				return locales.PairConnecting(cx)
			}, LiveRegion: core.LiveRegionPolite, Style: core.Style{FontSize: 15, LineHeight: 20, FontWeight: core.FontWeightBold, FlexShrink: 1}, StyleFunc: ui.Colored(colors.Text)},
		}}
}

// field is a labeled control of the form.
func field(cx *core.Scope, label func() string, control core.Node) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
		core.Text{ContentFunc: label, Style: look.Section.Merge(core.Style{Padding: core.Edges{Left: 4}}), StyleFunc: ui.Colored(colors.Muted)},
		control,
	}}
}

// codeInput is the field of the pairing code, in cells a phone's thumb and eye find at once: the design's 44 by 52, 22pt semibold tabular digits, 10 round, under a 24pt caret.
func codeInput(cx *core.Scope, value func() string, change func(string), disabled func() bool) core.Node {
	// TODO(nsgo#1861): ui.OtpInput.Label and base.OtpInput.Label are strings only, with no LabelFunc (ui/otpinput.go:34, base/otpinput.go:46, listed in NSGo's textFuncExceptions), so the field's spoken name keeps the language it was built in; it cannot sit in a core.Dynamic, which would take its focus, so pass a LabelFunc once OtpInput has one https://github.com/nsgo-dev/nsgo/issues/1861
	return ui.OtpInput{TestID: "pair-code", Label: locales.PairCode(cx), Length: sync.CodeDigits, Groups: 2, Value: value, OnChange: change, Disabled: disabled,
		CellWidth: 44, CellHeight: 52, CornerRadius: 10, CaretHeight: 24,
		TextStyle: core.Style{FontSize: 22, FontWeight: core.FontWeightSemibold, FontVariant: core.FontTabularNums}}
}

// found is a computer that answered: where, and its certificate's fingerprint.
type found struct {
	addr        netip.AddrPort
	fingerprint []byte
}

// reach tries every address at once for the computer's certificate, and returns the first that answers. The error is of an address that failed for another reason than being blocked when there is one, so blocked is true of it only when every address was.
func reach(ctx context.Context, addrs []netip.AddrPort) (found, error) {
	ctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	type result struct {
		found found
		err   error
	}
	results := make(chan result, len(addrs))
	for _, a := range addrs {
		go func() {
			fp, err := sync.FetchFingerprint(ctx, a)
			results <- result{found{a, fp}, err}
		}()
	}
	var err error
	for range addrs {
		r := <-results
		if r.err == nil {
			return r.found, nil
		}
		if err == nil || blocked(err) {
			err = r.err
		}
	}
	return found{}, err
}

// blocked reports whether err is a connection the system refused to make rather than one nothing answered: no route to the host or its network, or not permitted, as iOS fails a connection to the local network while the app may not make one (Apple's TN3179).
func blocked(err error) bool {
	return errors.Is(err, syscall.EHOSTUNREACH) || errors.Is(err, syscall.ENETUNREACH) || errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES)
}

// unreachable is what the page says when no address of target answered with err, and whether it is the local network access Settings turns on: on iOS, a connection the system refused is most likely that, as when the user turned it off after the app asked.
func unreachable(cx *core.Scope, s *session.Session, target Target, err error) (string, bool) {
	if s.Platform() == sync.PlatformIOS && blocked(err) {
		return locales.PairErrLocalNetwork(cx, target.Addrs[0].String()), true
	}
	return locales.PairErrUnreachable(cx, target.Addrs[0].String()), false
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

// relayProblem is what the page says when pairing through the computer's relay failed for err: a code the computer did not take as on its network, else that it did not answer.
func relayProblem(cx *core.Scope, err error) string {
	var e *sync.Error
	if errors.As(err, &e) {
		return pairProblem(cx, err, netip.AddrPort{})
	}
	return locales.PairErrRelay(cx)
}
