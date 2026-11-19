package plugins

import (
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

// Grant is what a script plugin may do, in full: the permissions and the HTTP rules. A plugin's manifest declares one (Manifest.Declared), the user grants one, kept in state.json (PluginState.Grant), and the plugin runs with what both hold and the app's state allows (State.Effective). The user grants the network as a whole: every rule the plugin declares, or none.
type Grant struct {
	Perms   []Perm         `json:"perms,omitempty"`
	Network []pkg.HTTPRule `json:"network,omitempty"`
}

// Declared is what the script plugin m asks for: its permissions, those it needs (capabilities.host) and those it can do without (optional), and its HTTP rules. A data pack declares nothing.
func (m Manifest) Declared() Grant {
	if m.Capabilities == nil {
		return Grant{}
	}
	g := Grant{Perms: slices.Concat(perms(m.Capabilities.Host), m.Optional)}
	if m.Capabilities.Network != nil {
		g.Network = slices.Clone(m.Capabilities.Network.HTTP)
	}
	return g
}

// perms is names as permissions.
func perms(names []string) []Perm {
	out := make([]Perm, len(names))
	for i, n := range names {
		out[i] = Perm(n)
	}
	return out
}

// Has reports whether g holds the permission p.
func (g Grant) Has(p Perm) bool { return slices.Contains(g.Perms, p) }

// Within reports whether every part of g is in d: each permission, and each HTTP rule as a whole.
func (g Grant) Within(d Grant) bool {
	for _, p := range g.Perms {
		if !d.Has(p) {
			return false
		}
	}
	for _, r := range g.Network {
		if !hasRule(d.Network, r) {
			return false
		}
	}
	return true
}

// Intersect is what both g and d hold, in g's order.
func (g Grant) Intersect(d Grant) Grant {
	var out Grant
	for _, p := range g.Perms {
		if d.Has(p) && !out.Has(p) {
			out.Perms = append(out.Perms, p)
		}
	}
	for _, r := range g.Network {
		if hasRule(d.Network, r) && !hasRule(out.Network, r) {
			out.Network = append(out.Network, r)
		}
	}
	return out
}

// hasRule reports whether rules holds a rule equal to r in every field.
func hasRule(rules []pkg.HTTPRule, r pkg.HTTPRule) bool {
	return slices.ContainsFunc(rules, func(o pkg.HTTPRule) bool { return reflect.DeepEqual(o, r) })
}

// Regrant is what an update keeps of the grant old the user gave the version that declared before, now that the same signer's new version declares now (docs/plugin-system.md 6.2): when now asks for nothing before did not, the grant is kept as it was, narrowed to what now still asks for; when it asks for more, it runs with old ∩ now too, and widened reports that the update asks for more, which the details flag until the user looks at the grant again. An update by another signer is a new install, whose grant is empty: Regrant is not for it.
func Regrant(old, before, now Grant) (g Grant, widened bool) {
	return old.Intersect(now), !now.Within(before)
}

// Effective is what the script plugin p may do now under s: what the user granted it, for its content and signer, intersected with what it declares, with the permissions that need no grant (Perm.Free) it declares, and without what the gates take: no network while Kando is offline, no permission that reads the library while the library is locked. A plugin turned off, or whose entry is for another content or signer, may do nothing.
func (s State) Effective(p Pack, offline, locked bool) Grant {
	e, ok := s.entry(p.ID)
	if !ok || s.off || !e.Enabled || e.Hash != p.Hash || e.Signer != p.Signer {
		return Grant{}
	}
	declared := p.Declared()
	g := e.Grant.Intersect(declared)
	for _, perm := range declared.Perms {
		if perm.Free() && !g.Has(perm) {
			g.Perms = append(g.Perms, perm)
		}
	}
	if offline {
		g.Network = nil
	}
	if locked {
		g.Perms = slices.DeleteFunc(g.Perms, Perm.Library)
	}
	return g
}

// Key writes g down whole, so two grants compare as strings.
func (g Grant) Key() string {
	var b strings.Builder
	for _, p := range g.Perms {
		b.WriteString(string(p) + ",")
	}
	for _, r := range g.Network {
		b.WriteString(r.Host + "|" + strings.Join(r.Methods, ",") + "|" + strings.Join(r.Paths, ",") + "|" + strings.Join(r.PathPrefixes, ",") + "|" + strings.Join(r.Schemes, ",") + "|" + strconv.Itoa(r.Port) + ";")
	}
	return b.String()
}

// AuditText writes g down for the audit log: the permissions and each HTTP rule's host, "none" for nothing.
func (g Grant) AuditText() string {
	var parts []string
	for _, p := range g.Perms {
		parts = append(parts, string(p))
	}
	for _, r := range g.Network {
		parts = append(parts, "network:"+r.Host)
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, " ")
}

// RulesText writes the HTTP rules down as requests: methods, scheme, host, port and paths, a path prefix ending in "…", one rule after the other.
func RulesText(rules []pkg.HTTPRule) string {
	var out []string
	for _, r := range rules {
		scheme := "https"
		if len(r.Schemes) > 0 {
			scheme = strings.Join(r.Schemes, "/")
		}
		host := r.Host
		if r.Port != 0 {
			host += ":" + strconv.Itoa(r.Port)
		}
		paths := slices.Clone(r.Paths)
		for _, p := range r.PathPrefixes {
			paths = append(paths, p+"…")
		}
		out = append(out, strings.Join(r.Methods, "/")+" "+scheme+"://"+host+" "+strings.Join(paths, ", "))
	}
	return strings.Join(out, "; ")
}

// GrantEntry is the entry of the script plugin p a change of its grant starts from: its entry when the same signer's, with p's content hash, kept on only while that is the content the user turned on; else a new one, off and granted nothing, as what the user granted another signer's plugin is not this one's.
func (s State) GrantEntry(p Pack) PluginState {
	e, ok := s.entry(p.ID)
	if !ok || e.Signer != p.Signer {
		return PluginState{ID: p.ID, Hash: p.Hash, Signer: p.Signer}
	}
	e.Enabled = e.Enabled && e.Hash == p.Hash
	e.Hash = p.Hash
	return e
}

// InstallEntry is the entry the script plugin p gets as it is installed, replacing old, the pack of its id installed now if any, whose entry is entry (had says it has one): a plugin installed for the first time, or now signed by another key than the one the user turned on, is off and granted nothing; an update by the same key keeps its switch and its grant as far as it still asks for it, and when it asks for more it is marked as updated (OldHash), until the user looks at the grant again (Regrant).
func InstallEntry(p Pack, old *Pack, entry PluginState, had bool) PluginState {
	fresh := PluginState{ID: p.ID, Hash: p.Hash, Signer: p.Signer}
	if old == nil || !had || entry.Signer != p.Signer || old.Kind != KindScript {
		return fresh
	}
	g, widened := Regrant(entry.Grant, old.Declared(), p.Declared())
	fresh.Enabled, fresh.Grant = entry.Enabled, g
	if widened {
		fresh.OldHash = old.Hash
	}
	return fresh
}
