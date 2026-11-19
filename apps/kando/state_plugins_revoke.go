package main

import (
	"net/http"
	"time"

	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/script"
)

// The revocation lists of the plugins' roots (docs/plugin-system.md 6.4): once a day while Kando is online and a script plugin is installed, the list published beside the release's appcasts (script.RevocationsURL) is fetched and given to the trust, which keeps the newest of each root's lists in plugins/trust/trust.json; the plugins are then verified again, so one it revokes is closed and shows 已被撤回. No list published yet (404) is no list, and the plugins go on as they were. A list that has passed its NotAfter pauses only the plugins that are not Kando's own and reach the network or send a credential (pluginsStale), as a plugin of this computer alone can do no harm a newer list would stop; Kando does not refuse every plugin then (pkg.Trust.RequireFreshRevocations), so a user long offline keeps the others.

// pluginsRevocationsEvery is how often the list is fetched.
const pluginsRevocationsEvery = 24 * time.Hour

// pluginsRevocationsURL is where the list is fetched from: script.RevocationsURL, or the tests' server.
func (a *app) pluginsRevocationsURL() string {
	if u := a.plugins.script.revocationsURL; u != "" {
		return u
	}
	return script.RevocationsURL
}

// pluginsCheckRevocations fetches the revocation list off the UI thread when it is due: Kando is online, a script plugin is installed, the trust is open, no fetch runs, and the last one, as the plugins folder records it, was pluginsRevocationsEvery ago or more. Once it is fetched, and taken when it is newer than the one held, the plugins are verified again.
func (a *app) pluginsCheckRevocations() {
	s := &a.plugins.script
	root := a.pluginsRoot()
	// The list is signed by Kando's official root: a trust whose official root a test stands in for (pluginsScripts.official) could not take it, and fetches only the test's own list.
	if root == "" || s.revoking || s.trust == nil || a.privacy.offline || len(a.pluginsScriptPacks()) == 0 || s.official != nil && s.revocationsURL == "" {
		return
	}
	now := time.Now()
	if !s.revokedAt.IsZero() && now.Sub(s.revokedAt) < pluginsRevocationsEvery {
		return
	}
	s.revoking = true
	trust, client, target, update := s.trust, a.pluginsRevocationsClient(), a.pluginsRevocationsURL(), a.update
	go func() {
		last := script.RevocationsChecked(root)
		taken, err := false, error(nil)
		if now.Sub(last) >= pluginsRevocationsEvery {
			taken, err = script.FetchRevocations(client, target, trust)
			if err == nil {
				script.SetRevocationsChecked(root, now)
				last = now
			}
		}
		update(func() {
			s.revoking = false
			// A failed fetch is tried again at the next check after an hour, not every time the app comes to the front.
			s.revokedAt = last
			if err != nil {
				s.revokedAt = now.Add(time.Hour - pluginsRevocationsEvery)
			}
			if taken {
				a.pluginsAudit(plugins.AuditRevocations, "", "updated")
				a.pluginsVerify()
			}
		})
	}()
}

// pluginsRevocationsClient is the client the list is fetched with: the tests' when they set one.
func (a *app) pluginsRevocationsClient() *http.Client {
	if c := a.plugins.script.client; c != nil {
		return c
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// pluginsStale reports whether the script plugin p is paused because the revocation list of the root it chains to has passed its NotAfter (pkg.Package.ExpiredRevocations) while it is not Kando's own and reaches the network or sends a credential.
func (a *app) pluginsStale(p plugins.Pack) bool {
	v, ok := a.plugins.script.verified[p.ID]
	if !ok || v.Package == nil || len(v.Package.ExpiredRevocations) == 0 || p.Source == plugins.SourceOfficial {
		return false
	}
	d := p.Declared()
	return len(d.Network) > 0 || d.Has(plugins.PermCredFetch)
}
