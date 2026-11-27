// Package everyapi is Kando's own sign-in to EveryAPI, the AI service run by YLDM, which makes Kando too: the browser sign-in through EveryAPI's OAuth 2.0 server (authorization code with PKCE and a loopback redirect, and the device code when no browser can come back), the relay key it gives and its renewal, signing out, what the account has left and where it is topped up. It imports no interface package; the app keeps the tokens in the system's secure store and shows the account in Settings › AI 服务 (state_everyapi.go).
//
// The access token EveryAPI's OAuth server gives is itself the relay key (sk-…), which translation requests carry as their bearer key; the refresh token renews it and is rotated each time: one used twice makes EveryAPI revoke the whole sign-in (refresh reuse detection), so a Session renews once for every request refused at the same time and keeps the new refresh token before it hands out the new key.
package everyapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ClientID is Kando's public OAuth client at EveryAPI. It is not a secret and proves nothing: EveryAPI grants nothing because a request names it.
const ClientID = "kando"

// The production addresses: the API, where the OAuth server, the account summary and the relay are, and the dashboard, where the user signs in and tops up.
const (
	DefaultAPIBase = "https://api.everyapi.ai"
	DefaultWebBase = "https://app.everyapi.ai"
)

// Config is where a sign-in goes. Empty fields take the defaults (withDefaults).
type Config struct {
	ClientID string
	// APIBase is EveryAPI's API, without a path: /api/oauth2/… and /api/usage/account are under it.
	APIBase string
	// WebBase is EveryAPI's dashboard: /authorize, where the browser signs in, and /wallet, where the account is topped up.
	WebBase string
	// DeviceName names this computer to EveryAPI, which keeps one key for Kando on each.
	DeviceName string
	// Client sends the requests; nil uses one with a 30-second timeout.
	Client *http.Client
}

func (c Config) withDefaults() Config {
	if c.ClientID == "" {
		c.ClientID = ClientID
	}
	if c.APIBase == "" {
		c.APIBase = DefaultAPIBase
	}
	if c.WebBase == "" {
		c.WebBase = WebBaseFor(c.APIBase)
	}
	c.APIBase = strings.TrimRight(c.APIBase, "/")
	c.WebBase = strings.TrimRight(c.WebBase, "/")
	if c.Client == nil {
		c.Client = &http.Client{Timeout: 30 * time.Second}
	}
	return c
}

// WebBaseFor is the dashboard of the API at apiBase, as EveryAPI's own command-line program finds it: api.<domain> has its dashboard at app.<domain>, and api-cn.everyapi.ai at app.everyapi.ai; any other address is its own.
func WebBaseFor(apiBase string) string {
	apiBase = strings.TrimRight(apiBase, "/")
	if apiBase == "https://api-cn.everyapi.ai" {
		return DefaultWebBase
	}
	for _, scheme := range []string{"https://", "http://"} {
		if rest, ok := strings.CutPrefix(apiBase, scheme+"api."); ok {
			return scheme + "app." + rest
		}
	}
	return apiBase
}

// TopUpURL is the dashboard's page where the account is topped up.
func TopUpURL(cfg Config) string { return cfg.withDefaults().WebBase + "/wallet" }

// Tokens is a sign-in: the relay key, when EveryAPI says it expires, and the refresh token that renews it.
type Tokens struct {
	Key     string    `json:"key"`
	Refresh string    `json:"refresh"`
	Expires time.Time `json:"expires,omitzero"`
}

// Valid reports whether t is a sign-in at all: a key and a refresh token.
func (t Tokens) Valid() bool { return t.Key != "" && t.Refresh != "" }

// What a sign-in fails with, for the app to say in its words (errors.Is):
var (
	// ErrSignedOut is a sign-in EveryAPI no longer takes: the refresh token is unknown, expired or revoked, as after signing out on the website, or the key was refused and could not be renewed.
	ErrSignedOut = errors.New("everyapi: the sign-in has ended")
	// ErrPaidAccount is EveryAPI refusing a key because the account may not hold one, as one that was never topped up.
	ErrPaidAccount = errors.New("everyapi: the account cannot hold an API key yet")
	// ErrKeySlotsFull is EveryAPI refusing a key because the account holds as many as it may.
	ErrKeySlotsFull = errors.New("everyapi: the account has no room for another API key")
	// ErrDenied is the user declining the sign-in on EveryAPI's page.
	ErrDenied = errors.New("everyapi: the sign-in was declined")
	// ErrExpired is a sign-in code that ran out before the user approved it.
	ErrExpired = errors.New("everyapi: the sign-in code expired")
	// ErrUnavailable is an EveryAPI that offers Kando no sign-in now: it does not know Kando's client (invalid_client), as an older server, or has nothing set up for it (unauthorized_client).
	ErrUnavailable = errors.New("everyapi: EveryAPI offers no sign-in for Kando right now")
	// ErrOutOfCredit is an account out of credit, as the account summary answered 402.
	ErrOutOfCredit = errors.New("everyapi: the account is out of credit")
)

// OAuthError is an error EveryAPI's OAuth server answered (RFC 6749 5.2), which Is maps to the errors above.
type OAuthError struct {
	Status      int
	Code        string
	Description string
}

func (e *OAuthError) Error() string {
	if e.Description != "" {
		return fmt.Sprintf("everyapi: %s: %s", e.Code, e.Description)
	}
	return fmt.Sprintf("everyapi: %s (HTTP %d)", e.Code, e.Status)
}

// Is maps EveryAPI's codes to what they mean for Kando: access_denied is the account (a paid account needed, or no room for another key) or the user declining, invalid_grant a sign-in that ended, expired_token a code that ran out, and invalid_client or unauthorized_client a server that does not know Kando.
func (e *OAuthError) Is(target error) bool {
	desc := strings.ToLower(e.Description)
	switch target {
	case ErrSignedOut:
		return e.Code == "invalid_grant"
	case ErrExpired:
		return e.Code == "expired_token"
	case ErrUnavailable:
		return e.Code == "invalid_client" || e.Code == "unauthorized_client"
	case ErrPaidAccount:
		return e.Code == "access_denied" && strings.Contains(desc, "paid account")
	case ErrKeySlotsFull:
		return e.Code == "access_denied" && strings.Contains(desc, "token limit")
	case ErrDenied:
		return e.Code == "access_denied" && !strings.Contains(desc, "paid account") && !strings.Contains(desc, "token limit")
	}
	return false
}

// decodeReply reads a JSON reply of at most 1 MiB into v; a body that is not JSON, as a proxy's error page, leaves v as it is.
func decodeReply(res *http.Response, v any) {
	_ = json.NewDecoder(http.MaxBytesReader(nil, res.Body, 1<<20)).Decode(v)
}
