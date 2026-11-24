package main

import (
	"context"
	"errors"

	"kando/apps/kando/locales"
	"kando/apps/kando/mediasource"
	"kando/packages/media"
)

// The sources of a script plugin's kinds (docs/media-sources.md 11, state_plugins_sources.go) as 设置 › 媒体来源 and the 媒体来源 page treat them: what a draft needs, what the dialog says of an address that is not encrypted, why one could not be read, the question that trusts a server's certificate the system does not, as a NAS's own, being 设置 › 媒体来源's one question about a server (mediaSourceAlert, 17.5).

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
