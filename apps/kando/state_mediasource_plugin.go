package main

import (
	"context"
	"errors"
	"net"
	"strings"

	"kando/apps/kando/locales"
	"kando/apps/kando/mediasource"
	"kando/apps/kando/settings"
	"kando/packages/media"
)

// The sources of a script plugin's kinds (docs/media-sources.md 11, state_plugins_sources.go) as 设置 › 媒体来源 and the 媒体来源 page treat them: what a draft needs, what the dialog says of an address that is not encrypted, why one could not be read, and the question that trusts a server's certificate the system does not, as a NAS's own (17.5).

// mediaSourceTrust is the question open over 设置 › 媒体来源 about the certificate of the endpoint of target, a source's ID or "" for the dialog's draft: the server's host and the fingerprint it offered, and the one trusted before when it offered another (Pinned).
type mediaSourceTrust struct {
	open        bool
	target      string
	host        string
	fingerprint string
	pinned      string
}

// mediaSourceOriginKey is the key, beside a plugin's source's secrets in the secure store, of the origin of the endpoint they were kept for (docs/media-sources.md 3.1): secrets kept for another origin are read as none, so a source whose address was changed to another server never sends that server the password of the old one, and the user enters it again. No field of a kind can have it, as a field's key has no dot.
const mediaSourceOriginKey = "endpoint.origin"

// mediaSourceOrigin is the origin of the endpoint of s, a source of the kind k, as its secrets are bound to it: scheme, host in lower case and port, the scheme's own when left out; "" for a built-in kind's source or an endpoint that is not one.
func mediaSourceOrigin(k mediasource.Kind, s mediasource.Source) string {
	if k.Origin.Builtin() {
		return ""
	}
	u, err := mediasource.ParseEndpoint(s.Config[mediasource.EndpointKey])
	if err != nil {
		return ""
	}
	port := u.Port()
	if port == "" {
		port = map[string]string{"https": "443", "http": "80"}[u.Scheme]
	}
	return u.Scheme + "://" + net.JoinHostPort(strings.ToLower(u.Hostname()), port)
}

// mediaSourcePluginProblem is what keeps s, a source of a plugin's kind, from being tried or saved, nil when nothing does: an address Kando sends its requests to (mediasource.ParseEndpoint).
func mediaSourcePluginProblem(s mediasource.Source) func(locales.Messages) string {
	if _, err := mediasource.ParseEndpoint(s.Config[mediasource.EndpointKey]); err != nil {
		return locales.Messages.SettingsSourcesNeedsEndpoint
	}
	return nil
}

// mediaSourcePluginWarnings are what the dialog says under the fields of a draft of a plugin's kind: that an http address sends the password unencrypted.
func mediaSourcePluginWarnings(m locales.Messages, values map[string]string) []string {
	if u, err := mediasource.ParseEndpoint(values[mediasource.EndpointKey]); err == nil && u.Scheme == "http" {
		return []string{m.SettingsSourcesPlainHTTP()}
	}
	return nil
}

// mediaSourcePluginFailure says why a source of a plugin's kind could not be read, in m's language: offline, an address that is not one, a certificate not trusted, the plugin not running, its server refusing the source's secrets or not answering, or anything else, which the plugin is named for.
func (a *app) mediaSourcePluginFailure(m locales.Messages, k mediasource.Kind, err error) string {
	var cert *mediasource.CertError
	var failed *mediasource.PluginError
	switch {
	case errors.Is(err, media.ErrOffline):
		return m.MediaSourceOffline()
	case errors.As(err, &cert):
		return m.SettingsSourcesFailCert()
	case errors.As(err, &failed) && failed.Code == pluginsNotRunningCode:
		return m.SettingsSourcesFailPluginOff(a.mediaSourcePluginName(k))
	case errors.As(err, &failed) && failed.Code == mediasource.PluginAuth:
		return m.SettingsSourcesFailAuth()
	case errors.As(err, &failed) && (failed.Code == mediasource.PluginUnavailable || failed.Code == pluginsTimedOut), errors.Is(err, context.DeadlineExceeded):
		return m.SettingsSourcesFailUnreachable()
	}
	return m.SettingsSourcesFailPlugin(a.mediaSourcePluginName(k))
}

// mediaSourcePluginName is the name of the plugin of the kind k in the interface's language, its ID while it is not installed.
func (a *app) mediaSourcePluginName(k mediasource.Kind) string {
	if p, ok := a.pluginsPack(k.Origin.Plugin); ok {
		return p.Name.In(a.prefs.interfaceLanguage)
	}
	return k.Origin.Plugin
}

// mediaSourceKindTitle is the name of the kind k as the dialog offers it: its title, and for a plugin's, the plugin's name after it.
func (a *app) mediaSourceKindTitle(k mediasource.Kind) string {
	title := k.Title(a.prefs.interfaceLanguage)
	if k.Origin.Builtin() {
		return title
	}
	return a.messages().SettingsSourcesKindPlugin(title, a.mediaSourcePluginName(k))
}

// mediaSourceAskTrust opens the question about the certificate a Test of target was refused for, holding the window's modal flag until it is answered: whether to trust it, or, when it is not the one trusted before, whether to replace that one.
func (a *app) mediaSourceAskTrust(target string, s mediasource.Source, cert *mediasource.CertError) {
	m := &a.sources
	host := mediasource.Host(s)
	m.trust = mediaSourceTrust{open: true, target: target, host: host, fingerprint: cert.Fingerprint, pinned: cert.Pinned}
	if m.trustRelease == nil {
		m.trustRelease = a.holdModal()
	}
}

// mediaSourceAnswerTrust answers the question and gives back its modal flag: yes keeps the fingerprint offered as the one trusted for the source's endpoint, in the dialog's draft or in the source saved, which settings.json then holds, and tests it again.
func (a *app) mediaSourceAnswerTrust(yes bool) {
	m := &a.sources
	t := m.trust
	m.trust = mediaSourceTrust{}
	if m.trustRelease != nil {
		m.trustRelease()
		m.trustRelease = nil
	}
	if !yes || !t.open {
		return
	}
	if t.target == "" {
		if !m.editing {
			return
		}
		m.draft.Values[mediasource.PinKey] = t.fingerprint
		a.testMediaSource("")
		return
	}
	i := a.mediaSourceIndex(t.target)
	if i < 0 {
		return
	}
	before := m.list[i]
	s := before
	s.Config = map[string]string{}
	for k, v := range before.Config {
		s.Config[k] = v
	}
	s.Config[mediasource.PinKey] = t.fingerprint
	m.list[i] = s
	if err := a.saveSettings(); err != nil {
		m.list[i] = before
		a.saveErr = a.messages().SettingsSourcesSaveFailed()
		return
	}
	a.mediaSourcePoolSync()
	a.testMediaSource(t.target)
}

// mediaSourceTrustShown is the question as the settings draw it.
func (a *app) mediaSourceTrustShown() settings.MediaSourceTrust {
	t := a.sources.trust
	return settings.MediaSourceTrust{Open: t.open, Host: t.host, Fingerprint: t.fingerprint, Trusted: t.pinned}
}
