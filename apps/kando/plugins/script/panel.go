package script

import (
	"kando/apps/kando/plugins"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/tsx"
)

// The host parameters a panel's view reads (core.Scope.HostParams, host.params in TSX): the id of the panel it is to show and the export that draws it (plugins.Panel). The plugin's entry module mounts that export when they are set, which they are only in a panel's view, never as the plugin is loaded for its functions or read by Probe:
//
//	import { host, mount } from "nsgo"
//	export function Deck() { return <Text>…</Text> }
//	if (host.params.panel === "deck") mount(Deck)
const (
	PanelParam  = "panel"
	ExportParam = "export"
)

// Panel is the view of the panel of the plugin p, whose package v loaded it with grant: its package run anew in a runtime of its own, in the same sandbox its functions run in (Sandbox: Limits, the grant made from nothing, the partition of its id and signer), with host, whose parameters it sets to name the panel (PanelParam, ExportParam). host must be the panel's own: a core.Host serves one app at a time, so the host p was loaded with would stop hearing its events. The view is under a core.Catch: what the module throws as it loads, as it renders or as it handles an event later, a piece of it cut at Limits' budget, or a quota it runs past, shows fallback in its place, retry building the view again, and counts for p's breaker as a failed call does, so a panel that keeps failing turns its plugin off (Stopped); it never reaches what holds the view. The entry module runs marked in flight (plugins.MarkInFlight) unless a call of p's is under way, which is marked already, so a panel that crashed Kando as it loaded turns its plugin off at the next start. Build it on the engine's goroutine.
//
// TODO(nsgo): tsx.View shows what the entry module mounts and tsx.Plugin draws nothing, so the host cannot render a named export of a package; the module mounts the panel itself from host.params. Render Panel.Export directly once tsx can, and drop the parameters
func (r *Runner) Panel(p *Plugin, v Verified, grant plugins.Grant, host *core.Host, panel plugins.Panel, fallback func(cx *core.Scope, err error, retry func()) core.Node) core.Node {
	view := tsx.View{Package: v.Package, Sandbox: Sandbox(v.ID, v.Signer, grant, host)}
	return core.Catch{
		Try: func(cx *core.Scope) core.Node {
			// Set at each build, as a retry builds a view that reads them again.
			_ = host.SetParams(map[string]any{PanelParam: panel.ID, ExportParam: panel.Export})
			// A call of p's under way keeps its own mark, which clearing this one would take.
			if r.Root != "" && (r.running == nil || r.running.p != p) {
				plugins.MarkInFlight(r.Root, p.ID, p.Hash)
				defer plugins.ClearInFlight(r.Root, p.ID)
			}
			return view.Render(cx)
		},
		Fallback: func(cx *core.Scope, err error, retry func()) core.Node {
			// Counted after the build: stopping the plugin changes the app's state, which a build does not.
			r.Post(func() { r.Failed(p, err) })
			return fallback(cx, err, retry)
		},
	}
}

// Failed counts err, a failure of the plugin p outside its calls, as its panel's view failing, for its breaker as a failed call is counted (a core.Error with a code is not a failure), and stops it when the breaker trips. Call it on the engine's goroutine.
func (r *Runner) Failed(p *Plugin, err error) {
	if p.closed {
		return
	}
	if p.count(err) {
		r.stop(p, plugins.ReasonFailed)
	}
}
