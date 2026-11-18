package media

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

// ErrOffline is what every step that would use the network returns, before it sends anything, while offline mode is on (Tools.Offline, ChatTranslator.Offline): a download from a link, a list of a playlist's videos, and a request to an AI service that is not on this computer. A failed job keeps its text, so the app tells it from other failures by its words.
var ErrOffline = errors.New("offline mode is on")

// LocalEndpoint reports whether baseURL names a server on this computer: its host is localhost or a loopback address, as Ollama's and LM Studio's are. Requests to it never leave the computer, so offline mode lets them through.
func LocalEndpoint(baseURL string) bool {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Local reports whether t translates on this computer: its endpoint is a local one (LocalEndpoint) and it signs in through no program (Credential), as the everyapi command-line program, which asks EveryAPI's servers for its key.
func (t ChatTranslator) Local() bool {
	return t.Credential == nil && LocalEndpoint(t.BaseURL)
}

// offline is ErrOffline when t may not send anything, as offline mode is on and its endpoint is not on this computer, and nil otherwise.
func (t ChatTranslator) offline() error {
	if t.Offline && !t.Local() {
		return ErrOffline
	}
	return nil
}

// offlineNow reports whether offline mode is on for t now: Offline, or the switch the app shares with the copies of t (OfflineSwitch).
func (t Tools) offlineNow() bool {
	return t.Offline || (t.OfflineSwitch != nil && t.OfflineSwitch.Load())
}

// withOffline is t's translator, which also refuses to reach a service off this computer while t is offline.
func (t Tools) withOffline() ChatTranslator {
	translator := t.Translator
	translator.Offline = translator.Offline || t.offlineNow()
	return translator
}
