package main

import (
	"cmp"
	"crypto/ed25519"
	"errors"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/script"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/i18n"
	"github.com/nsgo-dev/nsgo/tsx/pkg"
	"github.com/nsgo-dev/nsgo/ui"
)

// Script plugins (docs/plugin-system.md P3): signed TSX packages, installed as the data packs are, which run sandboxed (plugins/script) once the user turned them on, with what the user granted on their details. After each read of the plugins folder their packages are verified off the UI thread against Kando's official root and, in developer mode, the developer key of this computer (pluginsVerify); then the plugins turned on are loaded one per update, each after its code said again what it is (script.Probe), each with a host of its own whose handlers check, at every call, what it may do now (State.Effective): what the user granted, while the library is not locked and, for the network, while Kando is not offline. Their calls run one at a time under a watchdog (script.Runner), which turns off a plugin that keeps the window from answering, holds too much memory or fails too often, and records why in health.json; a call still running when Kando stopped turns its plugin off at the next start (in-flight marks). What they add: commands (pluginKeys), and they hear film.opened, job.completed and llm.changed. Every install, grant, switch, stop and confirmation goes into plugins/audit.log.

// pluginsScripts is the script plugins' part of the plugins' state (pluginsUI.script). trust verifies them, made once the developer key is read (devRead), with the developer root (dev) in developer mode, again when that changes; trustErr is why it could not be opened. verified are the packages verified, by id, for the content hash of each, and issues why the others were refused; verifyGen counts the verifications, so only the last one's answer is kept. loaded are the plugins running, by id, and loading the ids to load, one per update. health is health.json, which the plugins Kando turned off are in. runner calls them; watch is its watchdog's, which the tests set. calls are each plugin's host calls of the last minute, for its quota. toaster shows their toasts, and toasting is set once one showed. opened is the film film.opened was last sent for, jobs the stages of the jobs as job.completed last saw them. asking is set while a page a plugin asked to open waits for the user's answer. filmKey is the secret the films' ids for the plugins are derived from. values are the plugins' settings, by id; secrets which of their secret settings are kept, by id and key. started is set once the first verification started, which reads health.json and the in-flight marks; developer is developer mode (savedPlugins.PluginDeveloper). keying is set while pluginsConflicts reads the shortcuts without the plugins'. saves counts the saves of state.json pluginsSaveState started. official stands in for Kando's official root in the tests, which sign their plugins with roots of their own; nil otherwise. waiting are the host calls waiting outside the engine, by plugin and kind (pluginsWaiting); client is what the credentials' requests go through, which the tests set (pluginsHTTPClient); noSecrets is set once the system's secure store turned out to be missing, as on Linux without a Secret Service, which keeps the plugins with credentials off; contrib is the state of what the plugins contribute besides commands (state_plugins_contrib.go). revoking is set while the revocation list is fetched, and revokedAt is when it was last fetched (state_plugins_revoke.go). Engine's goroutine only.
type pluginsScripts struct {
	trust     *pkg.Trust
	trustErr  error
	dev       ed25519.PublicKey
	devRead   bool
	devBusy   bool
	verified  map[string]script.Verified
	issues    []plugins.Issue
	verifyGen int
	loaded    map[string]*pluginsScript
	loading   []string
	health    []plugins.Health
	runner    *script.Runner
	watch     script.Watch
	calls     map[string][]time.Time
	// sounds are the exports the user started that are running, by plugin id: the reads of a line's sound each may still make without counting toward the plugin's quota (pluginsSoundAllowance); soundsGen numbers them, so the end of an older export never ends a newer one's allowance.
	sounds    map[string]pluginsSoundAllowance
	soundsGen int
	toaster   *ui.Toaster
	toasting  bool
	opened    string
	jobs      map[string]string
	asking    bool
	filmKey   []byte
	values    map[string]plugins.Values
	secrets   map[string]map[string]bool
	started   bool
	developer bool
	keying    bool
	official  ed25519.PublicKey
	saves     int
	waiting   map[string]bool
	client    *http.Client
	noSecrets bool
	contrib   pluginsContrib
	revoking  bool
	revokedAt time.Time
	// revocationsURL stands in for script.RevocationsURL in the tests.
	revocationsURL string
}

// pluginsScript is a script plugin running: its pack, its package as verified, the plugin the runner calls, its own host and what it was granted as it loaded; the host of the view of its panel shown now, nil while none is (state_plugins_panels.go), and which load of a plugin it is, counting every load (pluginsPanels.runs), so the panel's view is built again for a plugin loaded again.
type pluginsScript struct {
	pack   plugins.Pack
	v      script.Verified
	plugin *script.Plugin
	host   *ns.Host
	grant  plugins.Grant
	panel  *ns.Host
	run    int
}

// The limits of the script plugins: how many may be turned on at once, and how many host calls each may make a minute.
const (
	pluginsScriptsMax   = 12
	pluginsCallsMinute  = 60
	pluginsDevKeyName   = "plugins.developer-key"
	pluginsListMax      = 500
	pluginsSeenDefault  = 20
	pluginsToastMax     = 200
	pluginsCommandGroup = "plugin:"
)

// Why a script plugin's switch or call was refused, besides the plugins package's and plugins/script's errors.
var (
	errPluginsTooMany = errors.New("too many script plugins are turned on")
	errPluginsDevID   = errors.New("a developer plugin cannot replace a plugin that is not one")
	errPluginsNoDev   = errors.New("no developer key")
)

// pluginsWatch is the watchdog the app's runner uses: script.DefaultWatch, unless a test set another.
func (a *app) pluginsWatch() script.Watch {
	if w := a.plugins.script.watch; w.Beat > 0 {
		return w
	}
	return script.DefaultWatch
}

// pluginsRunner is the runner of the plugins' calls, made the first time it is needed, which is once the app's scope exists.
func (a *app) pluginsRunner() *script.Runner {
	s := &a.plugins.script
	if s.runner == nil {
		s.runner = &script.Runner{Post: a.cx.Post, Root: a.pluginsRoot(), Watch: a.pluginsWatch(), PanelWatch: a.plugins.panels.watch, Stopped: a.pluginsStopped}
	}
	return s.runner
}

// pluginsScripts are the script plugins installed.
func (a *app) pluginsScriptPacks() []plugins.Pack {
	var out []plugins.Pack
	for _, p := range a.plugins.packs {
		if p.Kind == plugins.KindScript {
			out = append(out, p)
		}
	}
	return out
}

// pluginsScriptsScanned follows a read of the plugins folder for the script plugins: the packs keep where they come from and who signed them while their content is the one verified before, so a read does not turn them off until they are verified again; then they are verified again, off the UI thread (pluginsVerify).
func (a *app) pluginsScriptsScanned() {
	s := &a.plugins.script
	for i, p := range a.plugins.packs {
		if v, ok := s.verified[p.ID]; ok && p.Kind == plugins.KindScript && v.Hash == p.Hash {
			a.plugins.packs[i].Source, a.plugins.packs[i].Signer = v.Source, v.Signer
		}
	}
	a.pluginsVerify()
}

// pluginsVerify verifies the packages of the script plugins installed off the UI thread, and once they are, loads those turned on (pluginsScriptsSync). The first time, it also reads the developer key in developer mode, which the trust needs, health.json and the in-flight marks a call still running when Kando last stopped left, which turn their plugins off as hung; and it makes the films' secret for the plugins.
func (a *app) pluginsVerify() {
	root := a.pluginsRoot()
	s := &a.plugins.script
	packs := a.pluginsScriptPacks()
	// Without a script plugin there is nothing to verify, and no mark nor secret to read or make: a folder of data packs alone gets no file of the script plugins'.
	if root == "" || len(packs) == 0 {
		s.verified, s.issues = map[string]script.Verified{}, nil
		a.pluginsScriptsSync()
		return
	}
	if !s.devRead && a.plugins.script.developer {
		a.pluginsReadDevKey()
		return
	}
	s.verifyGen++
	gen, first, update := s.verifyGen, !s.started, a.update
	s.started = true
	trust, trustErr := s.trust, s.trustErr
	if trust == nil && trustErr == nil {
		trust, trustErr = script.OpenTrust(root, a.pluginsRoots())
		s.trust, s.trustErr = trust, trustErr
	}
	roots := a.pluginsRoots()
	pluginsWrite(func() {
		verified := map[string]script.Verified{}
		var issues []plugins.Issue
		for _, p := range packs {
			if trustErr != nil {
				issues = append(issues, plugins.Issue{Dir: p.Dir, Err: trustErr})
				continue
			}
			v, err := script.Verify(trust, roots, p)
			if err != nil {
				issues = append(issues, plugins.Issue{Dir: p.Dir, Err: err})
				continue
			}
			verified[p.ID] = v
		}
		var health []plugins.Health
		var hung map[string]string
		var key []byte
		if first {
			health, _ = plugins.LoadHealth(root)
			hung = plugins.InFlight(root)
			key = plugins.FilmKey(root)
		}
		update(func() {
			// What the first verification read once, health.json, the in-flight marks, which it took off the disk, and the films' secret, is kept even when a later verification superseded it, which does not read them again.
			if first {
				s.health, s.filmKey = plugins.MergeHealth(s.health, health), key
				for id, hash := range hung {
					a.pluginsTurnOff(id, hash, plugins.ReasonHung)
				}
			}
			if s.verifyGen != gen {
				return
			}
			a.pluginsVerified(verified, issues)
			a.pluginsScriptsSync()
		})
	})
}

// pluginsVerified takes what a verification found: each pack verified knows where it comes from and who signed it, and the others are listed among the page's problems.
func (a *app) pluginsVerified(verified map[string]script.Verified, issues []plugins.Issue) {
	s := &a.plugins.script
	s.verified, s.issues = verified, issues
	for i, p := range a.plugins.packs {
		if p.Kind != plugins.KindScript {
			continue
		}
		if v, ok := verified[p.ID]; ok && v.Hash == p.Hash {
			a.plugins.packs[i].Source, a.plugins.packs[i].Signer = v.Source, v.Signer
		} else {
			a.plugins.packs[i].Source, a.plugins.packs[i].Signer = plugins.SourceLocal, ""
		}
	}
	// The settings of every plugin verified are read, running or not, so its details show them and a change keeps the others (pluginsSetValue).
	for _, p := range a.plugins.packs {
		if _, ok := s.values[p.ID]; !ok && p.Kind == plugins.KindScript && p.Signer != "" {
			a.pluginsLoadValues(p)
		}
	}
	for _, issue := range issues {
		if errors.Is(issue.Err, script.ErrRevoked) {
			a.pluginsAudit(plugins.AuditRevoked, filepath.Base(issue.Dir), "")
		}
	}
	a.pluginsCheckRevocations()
}

// pluginsRoots are the roots the trust takes: Kando's official root, and the developer root in developer mode.
func (a *app) pluginsRoots() script.Roots {
	r := script.DefaultRoots(a.pluginsDevRoot())
	if a.plugins.script.official != nil {
		r.Official = a.plugins.script.official
	}
	return r
}

// pluginsDevRoot is the developer root the trust takes: the developer key's in developer mode, nil otherwise.
func (a *app) pluginsDevRoot() ed25519.PublicKey {
	if !a.plugins.script.developer {
		return nil
	}
	return a.plugins.script.dev
}

// pluginsReadDevKey reads the developer key from the system's secure store off the UI thread, which may ask the user first, then verifies the plugins again with its root.
func (a *app) pluginsReadDevKey() {
	s := &a.plugins.script
	if a.cx == nil || s.devBusy {
		return
	}
	s.devBusy = true
	secrets, update := a.cx.Secrets(), a.update
	go func() {
		secret, ok, err := secrets.Get(pluginsDevKeyName)
		var public ed25519.PublicKey
		if err == nil && ok {
			_, public, err = script.DevKey(secret)
		}
		update(func() {
			s.devBusy, s.devRead = false, true
			if err != nil {
				a.plugins.err = err
			}
			s.dev = public
			s.trust, s.trustErr = nil, nil
			a.pluginsVerify()
		})
	}()
}

// pluginsScriptsSync brings the plugins running in line with those turned on: it closes those turned off, gone, of another content, refused, turned off by Kando, or whose grant changed since they loaded, as when the user changed it or Kando went offline or the library was locked, as tsx grants a plugin what it was loaded with; and it queues those turned on that are not running, which load one per update (pluginsLoadNext).
func (a *app) pluginsScriptsSync() {
	s := &a.plugins.script
	if s.loaded == nil {
		s.loaded = map[string]*pluginsScript{}
	}
	want := map[string]plugins.Pack{}
	for _, p := range a.pluginsScriptPacks() {
		if a.pluginsRunnable(p) {
			want[p.ID] = p
		}
	}
	for id, l := range s.loaded {
		p, ok := want[id]
		if ok && p.Hash == l.pack.Hash && p.Signer == l.pack.Signer && a.pluginsEffective(p).Key() == l.grant.Key() {
			continue
		}
		a.pluginsCloseScript(id)
	}
	ids := make([]string, 0, len(want))
	for id := range want {
		if _, ok := s.loaded[id]; !ok && !slices.Contains(s.loading, id) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	start := len(s.loading) == 0
	s.loading = append(s.loading, ids...)
	if start && len(s.loading) > 0 {
		a.update(a.pluginsLoadNext)
	}
}

// pluginsRunnable reports whether the script plugin p is to run: verified for its content, turned on for it and its signer, not turned off by Kando for it, for one with credentials, while the system has a secure store to keep them in (docs/plugin-system.md 6.3), and not paused for a revocation list out of date (pluginsStale).
func (a *app) pluginsRunnable(p plugins.Pack) bool {
	s := &a.plugins.script
	v, ok := s.verified[p.ID]
	if !ok || v.Hash != p.Hash || p.Signer == "" || !a.plugins.state.EnabledPack(p) {
		return false
	}
	_, sick := plugins.HealthOf(s.health, p.ID, p.Hash)
	return !sick && !(s.noSecrets && len(p.Credentials) > 0) && !a.pluginsStale(p)
}

// pluginsEffective is what the script plugin p may do now (plugins.State.Effective): its grant, while Kando is online for the network and the library is not locked for what reads it.
func (a *app) pluginsEffective(p plugins.Pack) plugins.Grant {
	return a.plugins.state.Effective(p, a.privacy.offline, a.lockShut())
}

// pluginsLoadNext loads the next plugin queued, after its code said again what it is (script.Probe), with a host of its own (pluginsHost) and what it may do now, and queues the one after it on the next update, so a window never waits for more than one plugin's entry module, a second at most. A plugin that fails to load is turned off for its content (plugins.ReasonInit). One that loaded is accepted by the trust, so an older build of it is refused from now on.
func (a *app) pluginsLoadNext() {
	s := &a.plugins.script
	if len(s.loading) == 0 || a.cx == nil {
		return
	}
	id := s.loading[0]
	s.loading = s.loading[1:]
	if len(s.loading) > 0 {
		defer a.update(a.pluginsLoadNext)
	}
	i := slices.IndexFunc(a.plugins.packs, func(p plugins.Pack) bool { return p.ID == id })
	if i < 0 || !a.pluginsRunnable(a.plugins.packs[i]) || s.loaded[id] != nil {
		return
	}
	p := a.plugins.packs[i]
	v := s.verified[id]
	// Loading holds the window for a moment, which is no call's doing.
	resume := a.pluginsRunner().Pause()
	defer resume()
	if err := script.Probe(a.cx, v); err != nil {
		a.pluginsTurnOff(id, p.Hash, plugins.ReasonInit)
		return
	}
	host := ns.NewHost()
	grant := a.pluginsEffective(p)
	plugin, err := script.Load(a.cx, v, grant, host)
	if err != nil {
		a.pluginsTurnOff(id, p.Hash, plugins.ReasonInit)
		return
	}
	a.plugins.panels.runs++
	s.loaded[id] = &pluginsScript{pack: p, v: v, plugin: plugin, host: host, grant: grant, run: a.plugins.panels.runs}
	a.pluginsHost(id, host)
	if trust := s.trust; trust != nil {
		pluginsWrite(func() { trust.Accept(v.Package) })
	}
	a.Changed()
}

// pluginsCloseScript closes the plugin id if it runs; its calls end with script.ErrStopped.
func (a *app) pluginsCloseScript(id string) {
	s := &a.plugins.script
	l := s.loaded[id]
	if l == nil {
		return
	}
	delete(s.loaded, id)
	a.pluginsRunner().Close(l.plugin)
}

// pluginsCloseScripts closes every plugin, as the app quits: the app's scope, which they run under, is still there.
func (a *app) pluginsCloseScripts() {
	for id := range a.plugins.script.loaded {
		a.pluginsCloseScript(id)
	}
	a.plugins.script.loading = nil
}

// pluginsStagedVerifier is what verifies a pack just staged off the UI thread: nothing for a data pack; a script plugin's package against the trust (script.Verify), opened now if it is not yet.
func (a *app) pluginsStagedVerifier() func(p plugins.Pack) (script.Verified, error) {
	s := &a.plugins.script
	root := a.pluginsRoot()
	if s.trust == nil && s.trustErr == nil && root != "" {
		s.trust, s.trustErr = script.OpenTrust(root, a.pluginsRoots())
	}
	trust, trustErr, roots := s.trust, s.trustErr, a.pluginsRoots()
	return func(p plugins.Pack) (script.Verified, error) {
		if p.Kind != plugins.KindScript {
			return script.Verified{}, nil
		}
		if trustErr != nil {
			return script.Verified{}, trustErr
		}
		return script.Verify(trust, roots, p)
	}
}

// pluginsStagedCheck checks the script plugin p just staged and verified as v on the engine's goroutine, before its confirmation opens: it runs on this Kando (Manifest.RunsOn); its code says what its manifest says (script.Probe); and a developer plugin takes no id of a plugin installed that is not one. It sets where p comes from and who signed it. A data pack passes as it is.
func (a *app) pluginsStagedCheck(p *plugins.Pack, v script.Verified) error {
	if p.Kind != plugins.KindScript {
		return nil
	}
	if !p.RunsOn(appVersion) {
		return script.ErrRuntime
	}
	if a.cx == nil {
		return script.ErrNoManifest
	}
	resume := a.pluginsRunner().Pause()
	defer resume()
	if err := script.Probe(a.cx, v); err != nil {
		return err
	}
	p.Source, p.Signer = v.Source, v.Signer
	if i := slices.IndexFunc(a.plugins.packs, func(o plugins.Pack) bool { return o.ID == p.ID }); i >= 0 && v.Source == plugins.SourceDeveloper && a.plugins.packs[i].Source != plugins.SourceDeveloper {
		return errPluginsDevID
	}
	return nil
}

// pluginsStopped hears that the runner stopped the plugin id for reason, its content hash's (script.Runner.Stopped), and turns it off.
func (a *app) pluginsStopped(id, hash, reason string) {
	delete(a.plugins.script.loaded, id)
	a.pluginsTurnOff(id, hash, reason)
	a.Changed()
}

// pluginsTurnOff turns the plugin id off for its content hash because of reason: it is recorded in health.json, which keeps it from loading until the user turns it on again, its switch is turned off in state.json, and the audit log says why.
func (a *app) pluginsTurnOff(id, hash, reason string) {
	s := &a.plugins.script
	a.pluginsCloseScript(id)
	s.health = plugins.SetHealth(s.health, plugins.Health{ID: id, Hash: hash, Reason: reason, At: time.Now()})
	a.pluginsSaveHealth()
	if e, ok := a.plugins.state.Entry(id); ok && e.Enabled {
		e.Enabled = false
		a.pluginsSaveState(a.pluginsNextState(func(st *plugins.State) { st.Set(e) }))
	}
	a.pluginsAudit(plugins.AuditStopped, id, reason)
}

// pluginsSaveHealth writes health.json off the UI thread.
func (a *app) pluginsSaveHealth() {
	root, health := a.pluginsRoot(), slices.Clone(a.plugins.script.health)
	if root == "" {
		return
	}
	pluginsWrite(func() { plugins.SaveHealth(root, health) })
}

// pluginsSaveState takes next as the plugins' state and saves it off the UI thread, as pluginsSetEnabled does; the plugins follow at once. Saves run one at a time, and one that a later change saved over already is not written, so state.json ends with the last state whatever order the saves start in; once the last one ended, the state shown goes back when it could not be saved.
func (a *app) pluginsSaveState(next plugins.State) {
	root := a.pluginsRoot()
	if root == "" {
		return
	}
	s := &a.plugins.script
	before := a.plugins.state
	a.plugins.state, a.plugins.saving = next, true
	a.plugins.stateGen++
	s.saves++
	gen, update := s.saves, a.update
	pluginsWrite(func() {
		pluginsStateMu.Lock()
		var err error
		if gen > pluginsStateSaved[root] {
			pluginsStateSaved[root] = gen
			err = next.Save(root)
		}
		pluginsStateMu.Unlock()
		update(func() {
			a.plugins.stateGen++
			if gen != s.saves {
				return
			}
			a.plugins.saving = false
			if err != nil {
				a.plugins.state, a.plugins.err = before, err
			} else {
				a.plugins.stateErr = nil
			}
			a.pluginsScriptsSync()
		})
	})
	a.pluginsScriptsSync()
}

// pluginsStateMu serializes the script plugins' saves of state.json, and pluginsStateSaved is the save each plugins folder was last written for.
var (
	pluginsStateMu    sync.Mutex
	pluginsStateSaved = map[string]int{}
)

// pluginsAudit adds a line to the audit log off the UI thread, through the feedback's scrubber, which hides what reads as a secret or a path of the user's.
func (a *app) pluginsAudit(event, id, detail string) {
	root := a.pluginsRoot()
	if root == "" {
		return
	}
	scrub := a.feedbackScrubber().Clean
	at := time.Now()
	pluginsWrite(func() { plugins.Audit(root, scrub, at, event, id, detail) })
}

// pluginsScriptsEnabled is how many script plugins are turned on.
func (a *app) pluginsScriptsEnabled() int {
	n := 0
	for _, p := range a.pluginsScriptPacks() {
		if a.plugins.state.EnabledPack(p) {
			n++
		}
	}
	return n
}

// pluginsScriptSetEnabled turns the script plugin p on or off, for its content and signer, keeping what the user granted it: at most pluginsScriptsMax at once. Turning it on forgets why Kando turned it off (health.json).
func (a *app) pluginsScriptSetEnabled(p plugins.Pack, on bool) {
	if on && p.Signer == "" {
		return
	}
	if on && !a.plugins.state.EnabledPack(p) && a.pluginsScriptsEnabled() >= pluginsScriptsMax {
		a.plugins.err = errPluginsTooMany
		return
	}
	e, _ := a.plugins.state.Entry(p.ID)
	e.ID, e.Hash, e.Signer, e.Enabled = p.ID, p.Hash, p.Signer, on
	s := &a.plugins.script
	if on {
		s.health = plugins.SetHealth(s.health, plugins.Health{ID: p.ID})
		a.pluginsSaveHealth()
	}
	a.plugins.err = nil
	a.pluginsSaveState(a.pluginsNextState(func(st *plugins.State) { st.Set(e) }))
	event := plugins.AuditDisable
	if on {
		event = plugins.AuditEnable
	}
	a.pluginsAudit(event, p.ID, "")
}

// pluginsSetGrant grants the script plugin id the permission perm, or its network as a whole when perm is pluginsNetwork, or takes it back, and saves state.json; it also forgets that an update asked for more (OldHash), as the user has looked at the grant again.
func (a *app) pluginsSetGrant(id, perm string, on bool) {
	i := slices.IndexFunc(a.plugins.packs, func(p plugins.Pack) bool { return p.ID == id })
	if i < 0 || a.plugins.busy {
		return
	}
	p := a.plugins.packs[i]
	if p.Kind != plugins.KindScript || p.Signer == "" {
		return
	}
	declared := p.Declared()
	// What the user granted another signer's plugin is not this one's, and a content the user never turned on stays off (State.GrantEntry).
	e := a.plugins.state.GrantEntry(p)
	g := e.Grant.Intersect(declared)
	if perm == pluginsNetwork {
		g.Network = nil
		if on {
			g.Network = slices.Clone(declared.Network)
		}
	} else {
		pp := plugins.Perm(perm)
		g.Perms = slices.DeleteFunc(g.Perms, func(o plugins.Perm) bool { return o == pp })
		if on && declared.Has(pp) {
			g.Perms = append(g.Perms, pp)
		}
		// The credentials go with credentialFetch, each as it is declared now.
		if pp == plugins.PermCredFetch {
			g.Credentials = nil
			if on {
				g.Credentials = slices.Clone(declared.Credentials)
			}
		}
	}
	e.Grant, e.OldHash = g, ""
	a.pluginsSaveState(a.pluginsNextState(func(st *plugins.State) { st.Set(e) }))
	a.pluginsAudit(plugins.AuditGrant, id, g.AuditText())
}

// pluginsNetwork names a plugin's network, granted as a whole, among its permissions.
const pluginsNetwork = "network"

// pluginsForgetPartitions clears what the plugin id kept in its storage under the key signer, and the secrets of its settings Kando kept for it, as it is uninstalled or replaced by one another key signed (docs/plugin-system.md 6.3).
func (a *app) pluginsForgetPartitions(p plugins.Pack, signer string) {
	if a.cx == nil || signer == "" {
		return
	}
	script.ClearStorage(a.cx, p.ID, signer)
	keys := p.SecretKeys()
	delete(a.plugins.script.secrets, p.ID)
	if len(keys) == 0 {
		return
	}
	secrets := a.pluginsHostSecrets(p.ID, signer)
	go func() {
		for _, k := range keys {
			secrets.Delete(k)
		}
	}()
}

// pluginsHostSecrets is the secure store's partition where Kando keeps the secrets of the plugin id signed by signer (script.HostPartition), which its own code cannot reach.
func (a *app) pluginsHostSecrets(id, signer string) ns.Secrets {
	inner := a.cx.Child()
	var secrets ns.Secrets
	inner.Run(func() {
		inner.ProvideSecrets(script.HostPartition(id, signer))
		secrets = inner.Secrets()
	})
	inner.Dispose()
	return secrets
}

// pluginsLoadValues reads the values of the settings of the script plugin p off the UI thread, and which of its secrets are kept, under its signer's partition.
func (a *app) pluginsLoadValues(p plugins.Pack) {
	root := a.pluginsRoot()
	if root == "" || a.cx == nil {
		return
	}
	secretKeys := p.SecretKeys()
	id := p.ID
	secrets, update := a.pluginsHostSecrets(id, p.Signer), a.update
	go func() {
		values, _ := plugins.LoadValues(root, id)
		kept := map[string]bool{}
		missing := false
		for _, k := range secretKeys {
			_, ok, err := secrets.Get(k)
			kept[k] = ok
			missing = missing || errors.Is(err, ns.ErrSecretsUnavailable)
		}
		update(func() {
			s := &a.plugins.script
			if missing && !s.noSecrets {
				// Without a secure store a plugin's credentials could only be kept in plain files, which Kando never does: the plugins that have some stay off (pluginsRunnable).
				s.noSecrets = true
				a.pluginsScriptsSync()
			}
			if s.values == nil {
				s.values = map[string]plugins.Values{}
			}
			if s.secrets == nil {
				s.secrets = map[string]map[string]bool{}
			}
			if _, ok := s.values[id]; !ok {
				s.values[id] = values
			}
			s.secrets[id] = kept
		})
	}()
}

// pluginsSetValue sets the plugin id's setting key to value, as a value of it (plugins.Setting.Value), and saves its settings off the UI thread; a value that is not one is dropped.
func (a *app) pluginsSetValue(id, key string, value any) {
	i := slices.IndexFunc(a.plugins.packs, func(p plugins.Pack) bool { return p.ID == id })
	root := a.pluginsRoot()
	if i < 0 || root == "" {
		return
	}
	j := slices.IndexFunc(a.plugins.packs[i].Settings, func(s plugins.Setting) bool { return s.Key == key })
	if j < 0 {
		return
	}
	v, ok := a.plugins.packs[i].Settings[j].Value(value)
	if !ok {
		return
	}
	s := &a.plugins.script
	current, read := s.values[id]
	if !read {
		// Its values are not read yet: saving these alone would write over the others.
		return
	}
	values := current.Clone()
	values[key] = v
	s.values[id] = values
	pluginsWriteValues(id, func() { plugins.SaveValues(root, id, values) })
}

// pluginsWriteValues writes the plugin id's settings with write off the UI thread: writes run one at a time, and one a later change wrote over already goes unwritten, so the file ends as the last change left it whatever order the writes start in. Call it on the engine's goroutine.
func pluginsWriteValues(id string, write func()) {
	pluginsValuesGen[id]++
	gen := pluginsValuesGen[id]
	pluginsWrite(func() {
		pluginsValuesMu.Lock()
		defer pluginsValuesMu.Unlock()
		if gen < pluginsValuesSaved[id] {
			return
		}
		pluginsValuesSaved[id] = gen
		write()
	})
}

// pluginsValuesGen counts the changes of each plugin's settings, on the engine's goroutine; pluginsValuesSaved is the change each plugin's file was last written for, under pluginsValuesMu.
var (
	pluginsValuesGen   = map[string]int{}
	pluginsValuesMu    sync.Mutex
	pluginsValuesSaved = map[string]int{}
)

// pluginsSetSecret keeps value as the plugin id's secret setting key in the secure store, or removes it when value is "", off the UI thread.
func (a *app) pluginsSetSecret(id, key, value string) {
	i := slices.IndexFunc(a.plugins.packs, func(p plugins.Pack) bool { return p.ID == id })
	if i < 0 || a.cx == nil || a.plugins.packs[i].Signer == "" {
		return
	}
	p := a.plugins.packs[i]
	if !slices.Contains(p.SecretKeys(), key) {
		return
	}
	secrets, update := a.pluginsHostSecrets(id, p.Signer), a.update
	go func() {
		var err error
		if value == "" {
			err = secrets.Delete(key)
		} else {
			err = secrets.Set(key, value)
		}
		update(func() {
			if err != nil {
				a.plugins.err = err
				return
			}
			s := &a.plugins.script
			if s.secrets == nil {
				s.secrets = map[string]map[string]bool{}
			}
			kept := map[string]bool{}
			for k, v := range s.secrets[id] {
				kept[k] = v
			}
			kept[key] = value != ""
			s.secrets[id] = kept
		})
	}()
}

// pluginsFilmID is the id the plugin l knows the film of video by (plugins.FilmID).
func (a *app) pluginsFilmID(l *pluginsScript, video string) string {
	return plugins.FilmID(a.plugins.script.filmKey, l.pack.ID, l.pack.Signer, video)
}

// pluginsSend sends each plugin running that hears event (its manifest's events) the event, with what data gives it for that plugin: scalars alone (docs/plugin-system.md 5.3); and the view of its panel shown now, which runs apart from it.
func (a *app) pluginsSend(event string, data func(l *pluginsScript) map[string]any) {
	for _, id := range slices.Sorted(maps.Keys(a.plugins.script.loaded)) {
		l := a.plugins.script.loaded[id]
		if slices.Contains(l.pack.Events, event) {
			d := data(l)
			l.host.Send(event, d)
			if l.panel != nil {
				l.panel.Send(event, d)
			}
		}
	}
}

// pluginsFilmOpened sends film.opened once for each film the player opens, after the change that opened it.
func (a *app) pluginsFilmOpened() bool {
	s := &a.plugins.script
	if a.openKey == s.opened {
		return false
	}
	s.opened = a.openKey
	f, ok := a.openFilm()
	if !ok || len(s.loaded) == 0 {
		return false
	}
	a.pluginsSend("film.opened", func(l *pluginsScript) map[string]any {
		return map[string]any{"film": a.pluginsFilmID(l, cmp.Or(f.VideoPath, f.Key)), "spoken": f.Spoken}
	})
	return false
}

// pluginsJobsCompleted sends job.completed for each job that completed since the shelf last changed; the jobs complete as the app starts are only noted.
func (a *app) pluginsJobsCompleted() {
	s := &a.plugins.script
	first := s.jobs == nil
	seen := map[string]string{}
	var done []string
	for _, job := range a.library.Jobs {
		stage := string(job.Stage)
		seen[job.ID] = stage
		if !first && stage == "complete" && s.jobs[job.ID] != stage {
			done = append(done, cmp.Or(job.VideoPath, job.ID))
		}
	}
	s.jobs = seen
	for _, video := range done {
		a.pluginsSend("job.completed", func(l *pluginsScript) map[string]any {
			return map[string]any{"film": a.pluginsFilmID(l, video), "stage": "complete"}
		})
	}
}

// pluginsQuota reports whether the plugin id may make another host call now, and counts it: at most pluginsCallsMinute a minute.
func (a *app) pluginsQuota(id string) bool {
	s := &a.plugins.script
	if s.calls == nil {
		s.calls = map[string][]time.Time{}
	}
	now := time.Now()
	calls := slices.DeleteFunc(s.calls[id], func(t time.Time) bool { return now.Sub(t) > time.Minute })
	if len(calls) >= pluginsCallsMinute {
		s.calls[id] = calls
		return false
	}
	s.calls[id] = append(calls, now)
	return true
}

// pluginsToast shows text, which the plugin of the pack p gave, as a toast under its name, plain and cut short.
func (a *app) pluginsToast(p plugins.Pack, text string) {
	s := &a.plugins.script
	text = strings.Join(strings.Fields(text), " ")
	if r := []rune(text); len(r) > pluginsToastMax {
		text = string(r[:pluginsToastMax]) + "…"
	}
	if s.toaster == nil || text == "" {
		return
	}
	s.toasting = true
	s.toaster.Show(ui.ToastContent{Title: p.Name.In(i18n.Language(a.cx)), Description: text})
	a.Changed()
}

// pluginsToasts is where the plugins' toasts show, over the main window (pluginsOverlay).
func (a *app) pluginsToasts(cx *ns.Scope) ns.Node {
	s := &a.plugins.script
	if s.toaster == nil {
		s.toaster = ui.NewToaster(cx)
	}
	toaster := s.toaster
	return ns.Show{When: live.Func(a, func() bool { return a.plugins.script.toasting }), Then: func(cx *ns.Scope) ns.Node {
		return ui.Toasts{Toaster: toaster, TestID: "plugins-toasts", CloseLabel: locales.PluginsToastClose(cx)}
	}}
}

// pluginsDevPack packs the developer's plugin folder dir with the developer key (script.PackFolder) off the UI thread and opens the install confirmation of the plugin made, as for any plugin.
func (a *app) pluginsDevPack(dir string) {
	root := a.pluginsRoot()
	if root == "" || a.cx == nil || a.plugins.busy || a.plugins.pending != nil {
		return
	}
	a.plugins.busy, a.plugins.err = true, nil
	secrets, update := a.cx.Secrets(), a.update
	go func() {
		secret, ok, err := secrets.Get(pluginsDevKeyName)
		if err == nil && !ok {
			err = errPluginsNoDev
		}
		var out string
		if err == nil {
			out, err = os.MkdirTemp("", "kando-devpack-*")
		}
		if err == nil {
			var key ed25519.PrivateKey
			if key, _, err = script.DevKey(secret); err == nil {
				err = script.PackFolder(dir, out, key)
			}
		}
		update(func() {
			a.plugins.busy = false
			if err != nil {
				os.RemoveAll(out)
				a.pluginsAsk(&pluginsPending{err: err})
				return
			}
			a.pluginsStageThen(out, func() { os.RemoveAll(out) })
		})
	}()
}

// pluginsMakeDevKey makes a developer key and keeps it in the system's secure store off the UI thread; the trust takes its root at once, as developer mode is on.
func (a *app) pluginsMakeDevKey() {
	if a.cx == nil || a.plugins.script.devBusy {
		return
	}
	s := &a.plugins.script
	s.devBusy = true
	secrets, update := a.cx.Secrets(), a.update
	go func() {
		secret, public, err := script.NewDevKey()
		if err == nil {
			err = secrets.Set(pluginsDevKeyName, secret)
		}
		update(func() {
			s.devBusy = false
			if err != nil {
				a.plugins.err = err
				return
			}
			s.dev, s.devRead = public, true
			s.trust, s.trustErr = nil, nil
			a.pluginsVerify()
		})
	}()
}

// pluginsSetDeveloper turns developer mode on or off: the trust is opened again, with or without the developer root, and the plugins verified again, so a developer plugin stops once it is off.
func (a *app) pluginsSetDeveloper(on bool) {
	if a.plugins.script.developer == on {
		return
	}
	a.plugins.script.developer = on
	s := &a.plugins.script
	s.trust, s.trustErr = nil, nil
	a.pluginsVerify()
}

// pluginsGrantsFollow loads again, after a change of the app's state, the plugins running whose grant changed with it, as when Kando went offline or the library was locked: tsx holds a plugin to what it was granted as it loaded.
func (a *app) pluginsGrantsFollow() {
	for _, l := range a.plugins.script.loaded {
		if a.pluginsEffective(l.pack).Key() != l.grant.Key() {
			a.pluginsScriptsSync()
			return
		}
	}
}

// pluginsNames are the names of the plugins running, by id, for the menus their commands are under (groupTitle), which reads no app.
var pluginsNames sync.Map

// pluginsGroupTitle is the name of the plugin id in the language shown under cx, for the menu of its commands; its id while it is not known.
func pluginsGroupTitle(cx *ns.Scope, id string) string {
	if name, ok := pluginsNames.Load(id); ok {
		return name.(plugins.Text).In(i18n.Language(cx))
	}
	return id
}

// pluginKeys are the commands of the plugins running (contributes.commands), under a menu of each plugin's (plugin:<id>), in the order the plugins are listed: each runs its command (pluginsRunCommand), with the key it asks for unless a row of Kando's own, or of a plugin before it, has that key, which leaves it to the menu bar and the command palette (pluginsConflicts). A command for while a film is open applies only then.
func (a *app) pluginKeys(cx *ns.Scope) []shortcut {
	s := &a.plugins.script
	if len(s.loaded) == 0 || s.keying {
		return nil
	}
	conflicts := a.pluginsConflicts(cx)
	var out []shortcut
	for _, id := range a.pluginsLoadedOrder() {
		l := s.loaded[id]
		if l.pack.Contributes == nil {
			continue
		}
		pluginsNames.Store(id, l.pack.Name)
		for _, c := range l.pack.Contributes.Commands {
			row := shortcut{Group: pluginsCommandGroup + id, Label: c.Title.In(i18n.Language(cx)), Key: c.Key, Run: func() { a.pluginsRunCommand(id, c) }}
			if slices.Contains(conflicts[id], c.ID) {
				row.Key = ""
			}
			if c.When == plugins.WhenFilm {
				row.When = func() bool { return a.openKey != "" }
			}
			out = append(out, row)
		}
	}
	return out
}

// pluginsLoadedOrder is the ids of the plugins running in the order the plugins are listed (pluginsUI.order).
func (a *app) pluginsLoadedOrder() []string {
	var out []string
	for _, id := range a.plugins.order {
		if a.plugins.script.loaded[id] != nil {
			out = append(out, id)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(a.plugins.script.loaded)) {
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

// pluginsConflicts are the commands of each plugin running, by id, whose key a row of Kando's own (shortcuts without the plugins') or of a plugin listed before it has.
func (a *app) pluginsConflicts(cx *ns.Scope) map[string][]string {
	s := &a.plugins.script
	out := map[string][]string{}
	var taken map[string]bool
	for _, id := range a.pluginsLoadedOrder() {
		l := s.loaded[id]
		if l.pack.Contributes == nil {
			continue
		}
		for _, c := range l.pack.Contributes.Commands {
			if c.Key == "" {
				continue
			}
			if taken == nil {
				taken = map[string]bool{}
				s.keying = true
				for _, row := range a.shortcuts(cx) {
					taken[row.Key] = true
				}
				s.keying = false
			}
			if taken[c.Key] {
				out[id] = append(out[id], c.ID)
				continue
			}
			taken[c.Key] = true
		}
	}
	return out
}

// pluginsRunCommand runs the command c of the plugin id: its export is called with the command's id; a toast it returns ({toast: "…"}) shows when the plugin may show toasts, and a command that failed says so in a toast under the plugin's name.
func (a *app) pluginsRunCommand(id string, c plugins.Command) {
	l := a.plugins.script.loaded[id]
	if l == nil || a.cx == nil {
		return
	}
	a.pluginsRunner().Call(l.plugin, c.Export, []any{c.ID}, func(result any, err error) {
		defer a.Changed()
		if err != nil {
			a.pluginsToast(l.pack, a.messages().PluginsCommandFailed(c.Title.In(i18n.Language(a.cx))))
			return
		}
		if m, ok := result.(map[string]any); ok && a.pluginsEffective(l.pack).Has(plugins.PermToast) {
			if text, ok := m["toast"].(string); ok {
				a.pluginsToast(l.pack, text)
			}
		}
	})
}
