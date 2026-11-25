package sources

import (
	"errors"
	"maps"
	"strconv"
	"strings"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/nas"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	"kando/packages/remotefs"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// tested is how the form's last Test went: running, connected (ok, with what it says, as an SMB server's shares), or why not.
type tested struct {
	busy, ok bool
	text     string
}

// form is the state of a source's form: the source edited (id, "" for a new one), its kind, name and values as typed, the secrets among them, how its last Test went, why Save refused it and whether it is saving.
type form struct {
	cx       *core.Scope
	s        *session.Session
	id       string
	kind     *core.Signal[string]
	name     *core.Signal[string]
	values   *core.Signal[map[string]string]
	test     *core.Signal[tested]
	problem  *core.Signal[string]
	saving   *core.Signal[bool]
	testGen  int
	editing  bool
	finished func()
}

// EditPage is the form of the source id, or with "" of a new one, as the computer's media source dialog has it: its name, its kind (a new one's alone, among SMB, SFTP and WebDAV), the kind's fields, what SMB and plain HTTP risk, Test, which connects and reads its root, asking to trust an SFTP server's key or a WebDAV server's certificate the first time and warning when it changed, Save, and Remove for a source saved, which asks first. back leaves the page, as Save and Remove do once done.
func EditPage(cx *core.Scope, s *session.Session, id string, back func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	src, editing := s.Source(id)
	if !editing {
		src = store.Source{Kind: nas.KindSMB}
	}
	f := &form{cx: cx, s: s, id: src.ID, editing: editing, finished: back,
		kind:    core.NewSignal(cx, src.Kind),
		name:    core.NewSignal(cx, src.Name),
		values:  core.NewSignalFunc(cx, maps.Clone(src.Config), nil),
		test:    core.NewSignal(cx, tested{}),
		problem: core.NewSignal(cx, ""),
		saving:  core.NewSignal(cx, false),
	}
	title := func() string {
		if editing {
			return locales.SourceEditTitle(cx)
		}
		return locales.SourcesAdd(cx)
	}
	ref := &core.Ref{}
	blocks := []core.Node{
		field(cx, func() string { return locales.SourceName(cx) }, "", ui.Input{TestID: "source-name", LabelFunc: func() string { return locales.SourceName(cx) },
			Value: f.name.Get, OnChange: func(v string) { f.name.Set(v); f.changed() }, PlaceholderFunc: func() string { return locales.SourceNamePlaceholder(cx) },
			Style: inputStyle}),
	}
	if !editing {
		var segs []look.Segment
		for _, k := range nas.Kinds {
			segs = append(segs, look.Segment{Value: k, Text: func() string { return KindName(cx, k) }, TestID: "source-kind-" + k})
		}
		blocks = append(blocks, field(cx, func() string { return locales.SourceKind(cx) }, "",
			look.Segmented{Segments: segs, Value: f.kind.Get, Label: func() string { return locales.SourceKind(cx) }, OnChange: func(k string) { f.kind.Set(k); f.changed() }}))
	}
	for _, k := range nas.Kinds {
		blocks = append(blocks, core.Show{When: func() bool { return f.kind.Get() == k }, Then: func(cx *core.Scope) core.Node { return f.fields(cx, k) }})
	}
	blocks = append(blocks,
		core.Each(f.warnings, func(w string) string { return w }, func(cx *core.Scope, w func() string, _ func() int) core.Node {
			return core.View{Style: core.Style{Direction: core.Row, Gap: 8, Padding: core.Symmetric(0, 4)}, Children: []core.Node{
				ui.Icon(lucide.TriangleAlert, 15, colors.Warn),
				core.Text{ContentFunc: w, Style: look.Caption.Merge(core.Style{FlexShrink: 1}), StyleFunc: ui.Colored(colors.Muted)},
			}}
		}),
		core.View{Style: core.Style{Gap: 10}, Children: []core.Node{
			look.Button{TestID: "source-test", Kind: look.Tinted, Icon: lucide.PlugZap, Text: func() string { return locales.SourceTest(cx) },
				Busy: func() bool { return f.test.Get().busy }, BusyLabel: func() string { return locales.SourceTesting(cx) }, Disabled: f.saving.Get, OnPress: f.runTest},
			core.Show{When: func() bool { return f.test.Get().text != "" }, Then: func(cx *core.Scope) core.Node {
				return core.Text{TestID: "source-test-result", ContentFunc: func() string { return f.test.Get().text }, LiveRegion: core.LiveRegionPolite, Style: look.Subhead,
					StyleFunc: func() core.Style {
						if f.test.Get().ok {
							return core.Style{Color: colors.Up()}
						}
						return core.Style{Color: colors.Danger()}
					}}
			}},
			core.Show{When: func() bool { return f.problem.Get() != "" }, Then: func(cx *core.Scope) core.Node {
				return core.Text{TestID: "source-problem", ContentFunc: f.problem.Get, LiveRegion: core.LiveRegionPolite, Style: look.Subhead, StyleFunc: ui.Colored(colors.Danger)}
			}},
			look.Button{TestID: "source-save", Text: func() string { return locales.SourceSave(cx) }, Busy: f.saving.Get, BusyLabel: func() string { return locales.SourceSaving(cx) }, OnPress: f.save},
		}},
	)
	if editing {
		blocks = append(blocks, look.Button{TestID: "source-remove", Kind: look.Danger, Text: func() string { return locales.SourceRemove(cx) }, Disabled: f.saving.Get, OnPress: f.remove})
	}
	return look.NavPage(cx, base.NavPage{TitleFunc: title, TestID: "source-page", Children: []core.Node{
		core.Scroll{TestID: "source-scroll", Ref: ref, Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Behavior: core.ScrollBehavior{KeyboardDismissMode: core.KeyboardDismissOnDrag},
			Children: []core.Node{core.View{StyleFunc: formColumn(cx), Children: blocks}}},
	}}, ui.NavHeader{OnBack: back, Scroll: ref})
}

// inputStyle is a field's input: the design's 50 tall, 12 round, 17pt.
var inputStyle = core.Style{Height: core.Pt(50), FontSize: 17, CornerRadius: 12, Padding: core.Symmetric(0, 14)}

// formColumn is the form's column: its fields 18 apart.
func formColumn(cx *core.Scope) func() core.Style {
	col := look.Column(cx)
	return func() core.Style {
		st := col()
		st.Gap = 18
		return st
	}
}

// field is a field of the form: its title over control and the hint under it, when there is one.
func field(cx *core.Scope, title func() string, hint string, control core.Node) core.Node {
	colors := ui.ColorsOf(cx)
	children := []core.Node{
		core.Text{ContentFunc: title, Style: look.Section.Merge(core.Style{Padding: core.Edges{Left: 4}}), StyleFunc: ui.Colored(colors.Muted)},
		control,
	}
	if hint != "" {
		children = append(children, core.Text{ContentFunc: func() string { return hint }, Style: look.Caption.Merge(core.Style{Padding: core.Edges{Left: 4}}), StyleFunc: ui.Colored(colors.Muted)})
	}
	return core.View{Style: core.Style{Gap: 6}, Children: children}
}

// value is what the field key holds as typed.
func (f *form) value(key string) string { return f.values.Get()[key] }

// set types v into the field key.
func (f *form) set(key, v string) {
	m := maps.Clone(f.values.GetUntracked())
	if m == nil {
		m = map[string]string{}
	}
	m[key] = v
	f.values.Set(m)
	f.changed()
}

// changed forgets how the last Test went and why Save refused, which say nothing of what is typed now.
func (f *form) changed() {
	f.testGen++
	f.test.Set(tested{})
	f.problem.Set("")
}

// fields are kind's fields, each as its type is filled in.
func (f *form) fields(cx *core.Scope, kind string) core.Node {
	colors := ui.ColorsOf(cx)
	var out []core.Node
	for _, fl := range nas.Fields(kind) {
		key := fl.Key
		title := func() string { return fieldTitle(cx, kind, key) }
		hint := fieldHint(cx, kind, key)
		testID := "source-field-" + key
		switch fl.Type {
		case nas.Switch:
			out = append(out, core.View{Style: core.Style{Gap: 6}, Children: []core.Node{
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Symmetric(12, 16), MinHeight: core.Pt(52)},
					StyleFunc: func() core.Style { return look.Card(cx) },
					Children: []core.Node{
						core.Text{ContentFunc: title, Style: look.Body.Merge(core.Style{FlexGrow: 1, FlexShrink: 1}), StyleFunc: ui.Colored(colors.Text)},
						look.Switch{TestID: testID, Label: title, Checked: func() bool { return f.value(key) == "true" }, OnChange: func(on bool) {
							v := ""
							if on {
								v = "true"
							}
							f.set(key, v)
						}},
					}},
				core.Text{ContentFunc: func() string { return hint }, Style: look.Caption.Merge(core.Style{Padding: core.Edges{Left: 4}}), StyleFunc: ui.Colored(colors.Muted)},
			}})
		case nas.Trusted:
			out = append(out, field(cx, title, "", core.Text{TestID: testID, ContentFunc: func() string {
				switch v := f.value(key); {
				case v == "":
					return locales.SourceTrustedNone(cx)
				case key == nas.HostKey:
					return remotefs.Fingerprint(v)
				default:
					return v
				}
			}, Style: look.Subhead.Merge(core.Style{FontFamily: ui.Mono, Padding: core.Edges{Left: 4}}), StyleFunc: ui.Colored(colors.Text)}))
		default:
			in := ui.Input{TestID: testID, LabelFunc: title, Value: func() string { return f.value(key) }, OnChange: func(v string) { f.set(key, v) },
				AutoCapitalize: core.AutoCapitalizeNone, NoAutoCorrect: true, Masked: fl.Type == nas.Secret && !fl.Multiline, Multiline: fl.Multiline, Style: inputStyle}
			switch {
			case fl.Multiline:
				in.Style = core.Style{MinHeight: core.Pt(110), FontSize: 13, FontFamily: ui.Mono, CornerRadius: 12, Padding: core.All(12)}
			case fl.Type == nas.Number:
				in.KeyboardType = core.KeyboardNumber
				in.PlaceholderFunc = func() string { return strconv.Itoa(nas.DefaultPort(kind)) }
			case key == nas.URL:
				in.KeyboardType = core.KeyboardURL
				in.PlaceholderFunc = func() string { return "https://nas.local:5006" }
			case key == nas.Host:
				in.KeyboardType = core.KeyboardURL
			}
			if fl.Type == nas.Secret && f.editing {
				// A secret saved stays in the secure store, which the field never shows: left empty, it is kept.
				in.PlaceholderFunc = func() string { return locales.SourceSecretKept(cx) }
			}
			out = append(out, field(cx, title, hint, in))
		}
	}
	return core.View{Style: core.Style{Gap: 18}, Children: out}
}

// draft is the source the form describes, and the secrets typed in it, by key.
func (f *form) draft() (store.Source, map[string]string) {
	kind := f.kind.GetUntracked()
	values := f.values.GetUntracked()
	src := store.Source{ID: f.id, Kind: kind, Name: strings.TrimSpace(f.name.GetUntracked()), Config: nas.Config(kind, values)}
	typed := map[string]string{}
	for _, key := range nas.SecretKeys(kind) {
		if v := values[key]; v != "" {
			typed[key] = v
		}
	}
	return src, typed
}

// warnings are what the form says under the fields: for SMB that it cannot prove which server it reached, and while it does without signing what that risks; for WebDAV at an http:// address that its password crosses the network unencrypted.
func (f *form) warnings() []string {
	switch f.kind.Get() {
	case nas.KindSMB:
		out := []string{locales.SourceSMBWarning(f.cx)}
		if f.value(nas.NoSigning) == "true" {
			out = append(out, locales.SourceNoSigningRisk(f.cx))
		}
		return out
	case nas.KindWebDAV:
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(f.value(nas.URL))), "http://") {
			return []string{locales.SourcePlainHTTP(f.cx)}
		}
	}
	return nil
}

// runTest connects to the draft and reads its root, saying how it went; a server whose key or certificate is not trusted asks first (ask).
func (f *form) runTest() {
	if f.test.GetUntracked().busy {
		return
	}
	src, typed := f.draft()
	f.problem.Set("")
	if p := nas.Check(src, typed, f.editing); p != nas.NoProblem {
		f.test.Set(tested{text: problemText(f.cx, p)})
		return
	}
	f.testGen++
	gen := f.testGen
	f.test.Set(tested{busy: true})
	f.s.TestSource(src, typed, func(t session.Tested) {
		if gen != f.testGen {
			return
		}
		var hk *remotefs.HostKeyError
		var cert *remotefs.CertError
		switch {
		case t.Err == nil && src.Kind == nas.KindSMB && nas.Remote(src, nil).Share == "" && len(t.Shares) == 0:
			f.test.Set(tested{ok: true, text: locales.SourceNoShares(f.cx)})
		case t.Err == nil && len(t.Shares) > 0:
			f.test.Set(tested{ok: true, text: locales.SourceShares(f.cx, strings.Join(t.Shares, ", "))})
		case t.Err == nil:
			f.test.Set(tested{ok: true, text: locales.SourceConnected(f.cx)})
		case errors.As(t.Err, &hk):
			f.test.Set(tested{})
			f.askKey(hk)
		case errors.As(t.Err, &cert):
			f.test.Set(tested{})
			f.askCert(src, cert)
		default:
			f.test.Set(tested{text: Failure(f.cx, src, t.Err)})
		}
	})
}

// askKey asks whether to trust the key an SFTP server offered, at the first connection, or warns that it is not the key trusted before; trusted, the form keeps it as the server's key, saved with Save, and tests again.
func (f *form) askKey(hk *remotefs.HostKeyError) {
	cx := f.cx
	cfg := core.AlertConfig{Title: locales.SourceTrustTitle(cx, hk.Host), Message: locales.SourceTrustMessage(cx, hk.Fingerprint), Cancelable: true,
		Actions: []core.AlertAction{{Label: locales.CommonCancel(cx), Role: core.AlertCancel}, {Label: locales.SourceTrust(cx)}}}
	if hk.Trusted != "" {
		cfg = core.AlertConfig{Title: locales.SourceKeyChangedTitle(cx, hk.Host), Message: locales.SourceKeyChangedMessage(cx, remotefs.Fingerprint(hk.Trusted), hk.Fingerprint), Cancelable: true,
			Actions: []core.AlertAction{{Label: locales.CommonCancel(cx), Role: core.AlertCancel}, {Label: locales.SourceReplaceKey(cx), Role: core.AlertDestructive}}}
	}
	cx.ShowAlert(cfg, func(i int) { f.trust(i == 1, nas.HostKey, hk.Key) })
}

// askCert is askKey for the certificate of a WebDAV server over HTTPS that the system does not trust, as a NAS's own, which the form keeps by its fingerprint.
func (f *form) askCert(src store.Source, cert *remotefs.CertError) {
	cx := f.cx
	host := cert.Host
	if host == "" {
		host = nas.HostOf(src)
	}
	cfg := core.AlertConfig{Title: locales.SourceTrustTitle(cx, host), Message: locales.SourceCertTrustMessage(cx, cert.Fingerprint), Cancelable: true,
		Actions: []core.AlertAction{{Label: locales.CommonCancel(cx), Role: core.AlertCancel}, {Label: locales.SourceTrust(cx)}}}
	if cert.Pinned != "" {
		cfg = core.AlertConfig{Title: locales.SourceCertChangedTitle(cx, host), Message: locales.SourceCertChangedMessage(cx, cert.Pinned, cert.Fingerprint), Cancelable: true,
			Actions: []core.AlertAction{{Label: locales.CommonCancel(cx), Role: core.AlertCancel}, {Label: locales.SourceReplaceCert(cx), Role: core.AlertDestructive}}}
	}
	cx.ShowAlert(cfg, func(i int) { f.trust(i == 1, nas.CertKey, cert.Fingerprint) })
}

// trust answers the question about a server's key or certificate: yes keeps v in the field key and tests again; no says the connection was not made for it.
func (f *form) trust(yes bool, key, v string) {
	if !yes {
		f.test.Set(tested{text: locales.SourceNotTrusted(f.cx)})
		return
	}
	f.set(key, v)
	f.runTest()
}

// save keeps the draft, or says what keeps it from being saved; saved, the page goes back.
func (f *form) save() {
	if f.saving.GetUntracked() {
		return
	}
	src, typed := f.draft()
	if p := nas.Check(src, typed, f.editing); p != nas.NoProblem {
		f.problem.Set(problemText(f.cx, p))
		return
	}
	f.problem.Set("")
	f.saving.Set(true)
	f.s.SaveSource(src, typed, func(_ store.Source, err error) {
		f.saving.Set(false)
		switch {
		case errors.Is(err, core.ErrSecretsUnavailable):
			f.problem.Set(locales.SourceNoSecretStore(f.cx))
		case err != nil:
			f.problem.Set(locales.SourceSaveFailed(f.cx))
		default:
			f.finished()
		}
	})
}

// remove asks whether to remove the source, and removes it and goes back once the user confirms.
func (f *form) remove() {
	cx := f.cx
	src, ok := f.s.Source(f.id)
	if !ok {
		return
	}
	cx.ShowAlert(core.AlertConfig{Title: locales.SourceRemoveTitle(cx, src.Name), Message: locales.SourceRemoveMessage(cx), Sheet: !look.Tablet(cx), Cancelable: true,
		Actions: []core.AlertAction{{Label: locales.CommonCancel(cx), Role: core.AlertCancel}, {Label: locales.SourceRemoveConfirm(cx), Role: core.AlertDestructive}}}, func(i int) {
		if i == 1 {
			f.s.RemoveSource(f.id)
			f.finished()
		}
	})
}
