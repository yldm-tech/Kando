package main

import (
	"slices"
	"strings"

	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/index"
	"kando/apps/kando/settings"
)

// The official plugins that add a kind of media source (docs/media-sources.md 11.5), offered in 设置 › 媒体来源's dialog before they are installed or turned on: choosing one there installs it from the community index through the install confirmation, as a link of the plugin's page does, then turns it on and grants it kando.source.fetch, the one permission a source plugin asks for, and opens the dialog again on its kind.

// mediaSourceOffer is an official plugin the dialog offers as a kind of source: its id and its name, a product's, which is not translated.
type mediaSourceOffer struct {
	ID, Name string
}

// mediaSourceOffers are Kando's official media-source plugins, besides WebDAV, which is built in.
var mediaSourceOffers = []mediaSourceOffer{
	{ID: "kando.alist", Name: "Alist"},
	{ID: "kando.jellyfin", Name: "Jellyfin"},
	{ID: "kando.emby", Name: "Emby"},
	{ID: "kando.plex", Name: "Plex"},
}

// mediaSourceInstallPrefix starts the ID of the kind the dialog offers for a plugin to install or turn on, followed by the plugin's id: no kind registered has it (mediasource.Registry's IDs are SMB's, SFTP's, WebDAV's and plugin:…).
const mediaSourceInstallPrefix = "install:"

// The steps of installing a plugin for the dialog (mediaSourceInstall.step): waiting for the install confirmation, which comes once the index is read and the plugin downloaded and checked; the confirmation open; installed, waiting for the plugins folder to be read again; turned on, waiting for its kind to be registered.
const (
	mediaSourceInstallAsked = iota + 1
	mediaSourceInstallConfirming
	mediaSourceInstallInstalled
	mediaSourceInstallEnabled
)

// mediaSourceInstall is the plugin being installed or turned on for the dialog: its id, the step it is at, the content the confirmation installs (hash), and whether the source then added is shown on the 媒体来源 page (thenBrowse, as the dialog had it). id is "" while none is.
type mediaSourceInstall struct {
	id         string
	step       int
	hash       string
	thenBrowse bool
}

// mediaSourceOffered are the plugins the dialog offers to install or turn on: those of offers whose kind of source is not registered, as their plugin is not installed or is off.
func (a *app) mediaSourceOffered() []mediaSourceOffer {
	var out []mediaSourceOffer
	for _, o := range a.sources.offers {
		if !a.mediaSourcePluginKindRegistered(o.ID) {
			out = append(out, o)
		}
	}
	return out
}

// mediaSourcePluginKindRegistered reports whether a kind of source of the plugin id is registered, as it is while the plugin is on.
func (a *app) mediaSourcePluginKindRegistered(id string) bool {
	return a.mediaSourcePluginKind(id) != ""
}

// mediaSourcePluginKind is the ID of the first kind of source of the plugin id registered, "" while none is.
func (a *app) mediaSourcePluginKind(id string) string {
	for _, k := range a.sources.kinds.Kinds() {
		if k.Origin.Plugin == id {
			return k.ID
		}
	}
	return ""
}

// mediaSourceOfferOf is the plugin the draft's kind offers to install or turn on, false when it is a kind of source.
func (a *app) mediaSourceOfferOf(kind string) (mediaSourceOffer, bool) {
	id, ok := strings.CutPrefix(kind, mediaSourceInstallPrefix)
	if !ok {
		return mediaSourceOffer{}, false
	}
	i := slices.IndexFunc(a.sources.offers, func(o mediaSourceOffer) bool { return o.ID == id })
	if i < 0 {
		return mediaSourceOffer{}, false
	}
	return a.sources.offers[i], true
}

// mediaSourceInstalledPack is the plugin id installed, false while it is not.
func (a *app) mediaSourceInstalledPack(id string) (plugins.Pack, bool) {
	i := slices.IndexFunc(a.plugins.packs, func(p plugins.Pack) bool { return p.ID == id && p.Kind == plugins.KindScript })
	if i < 0 {
		return plugins.Pack{}, false
	}
	return a.plugins.packs[i], true
}

// mediaSourceInstallOffered installs or turns on the plugin the dialog's draft offers: one installed is turned on and granted at once; another goes through the install confirmation in Settings › 插件, as a link of the plugin's page does (pluginsIndexOpenLinks), the dialog closed meanwhile; mediaSourceInstallFollow takes it from there and opens the dialog again on its kind.
func (a *app) mediaSourceInstallOffered() {
	m := &a.sources
	o, ok := a.mediaSourceOfferOf(m.draft.Kind)
	if !ok || m.install.id != "" {
		return
	}
	thenBrowse := m.thenBrowse
	a.closeMediaSourceDialog()
	m.install = mediaSourceInstall{id: o.ID, step: mediaSourceInstallAsked, thenBrowse: thenBrowse}
	if p, ok := a.mediaSourceInstalledPack(o.ID); ok {
		m.install.step, m.install.hash = mediaSourceInstallInstalled, p.Hash
		a.mediaSourceInstallFollow()
		return
	}
	a.pluginsIndexOpenLinks([]string{index.InstallLink(o.ID)})
}

// mediaSourceInstallFollow moves the plugin being installed for the dialog on after each change of the app's state (mediaSourceHooks): its confirmation opened; then closed, installed or not; installed, read from the plugins folder and its signature checked, which turns it on and grants it kando.source.fetch; turned on and its kind registered, which opens 设置 › 媒体来源's dialog again on that kind. A confirmation closed without installing, as on 取消 or a download that failed, goes back to 设置 › 媒体来源; a plugin the index does not list, or one that cannot be turned on, stays on Settings › 插件, which says why; closing the settings drops it. It reports whether it changed anything.
func (a *app) mediaSourceInstallFollow() bool {
	in := &a.sources.install
	if in.id == "" {
		return false
	}
	if !a.prefs.open {
		*in = mediaSourceInstall{}
		return true
	}
	pending := a.plugins.pending
	switch in.step {
	case mediaSourceInstallAsked:
		switch {
		case pending != nil && pending.pack.ID == in.id, pending != nil && pending.err != nil:
			in.step, in.hash = mediaSourceInstallConfirming, ""
			if pending.err == nil {
				in.hash = pending.pack.Hash
			}
			return true
		case a.plugins.index.linkMissing == in.id || a.plugins.index.linkBad:
			*in = mediaSourceInstall{}
			return true
		}
	case mediaSourceInstallConfirming:
		if pending != nil || a.plugins.busy {
			return false
		}
		if in.hash != "" && a.plugins.installed == (pluginsInstalled{in.id, in.hash}) {
			in.step = mediaSourceInstallInstalled
			return true
		}
		*in = mediaSourceInstall{}
		a.SetSettingsSection(settings.SectionMediaSources)
		return true
	case mediaSourceInstallInstalled:
		// A plugin is turned on once its signature was checked, which names its signer.
		p, ok := a.mediaSourceInstalledPack(in.id)
		if !ok || p.Hash != in.hash || p.Signer == "" || a.plugins.busy || a.plugins.saving {
			return false
		}
		if !a.plugins.state.EnabledPack(p) {
			a.pluginsScriptSetEnabled(p, true)
		}
		if a.plugins.err == nil && !slices.Contains(a.plugins.state.Effective(p, false, false).Perms, plugins.PermSourceFetch) {
			a.pluginsSetGrant(p.ID, string(plugins.PermSourceFetch), true)
		}
		if a.plugins.err != nil {
			*in = mediaSourceInstall{}
			a.SetSettingsSection(settings.SectionPlugins)
			return true
		}
		in.step = mediaSourceInstallEnabled
		return true
	case mediaSourceInstallEnabled:
		// The plugins register the kinds of those turned on after a change too, which may come after this one in the same round, with no change after it to bring this back.
		a.pluginsSourcesFollow()
		kind := a.mediaSourcePluginKind(in.id)
		if kind == "" {
			return false
		}
		thenBrowse := in.thenBrowse
		*in = mediaSourceInstall{}
		a.SetSettingsSection(settings.SectionMediaSources)
		a.AddMediaSource()
		a.SetMediaSourceKind(kind)
		a.sources.thenBrowse = thenBrowse
		return true
	}
	return false
}
