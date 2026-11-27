package everyapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"kando/apps/kando/locales"

	"golang.org/x/oauth2"
)

// oauthConfig is cfg as golang.org/x/oauth2 takes it: a public client, which sends its id in the form and no secret, asking for no scope, as EveryAPI gives Kando's client its own, and EveryAPI's endpoints. redirect is the loopback address the browser comes back to, "" for the device code and a renewal.
func oauthConfig(cfg Config, redirect string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:    cfg.ClientID,
		RedirectURL: redirect,
		Endpoint: oauth2.Endpoint{
			AuthURL:       cfg.WebBase + "/authorize",
			TokenURL:      cfg.APIBase + "/api/oauth2/token",
			DeviceAuthURL: cfg.APIBase + "/api/oauth2/device",
			AuthStyle:     oauth2.AuthStyleInParams,
		},
	}
}

// withClient is ctx carrying cfg's HTTP client, which golang.org/x/oauth2 sends its requests with.
func withClient(ctx context.Context, cfg Config) context.Context {
	return context.WithValue(ctx, oauth2.HTTPClient, cfg.Client)
}

// maxDeviceName is the longest device name EveryAPI takes, in characters.
const maxDeviceName = 64

// deviceParams name this computer to EveryAPI when it gives a key, at most maxDeviceName characters of its name, so it keeps one key for Kando on each: a sign-in with the same name replaces the key of the one before. None when it has no name.
func deviceParams(cfg Config) []oauth2.AuthCodeOption {
	name := []rune(strings.TrimSpace(cfg.DeviceName))
	if len(name) == 0 {
		return nil
	}
	if len(name) > maxDeviceName {
		name = name[:maxDeviceName]
	}
	return []oauth2.AuthCodeOption{oauth2.SetAuthURLParam("device_name", strings.TrimSpace(string(name)))}
}

// tokensOf is what golang.org/x/oauth2 got, or what EveryAPI's OAuth server said instead (OAuthError).
func tokensOf(tok *oauth2.Token, err error) (Tokens, error) {
	if err != nil {
		var retrieve *oauth2.RetrieveError
		if errors.As(err, &retrieve) {
			status := 0
			if retrieve.Response != nil {
				status = retrieve.Response.StatusCode
			}
			if retrieve.ErrorCode != "" {
				return Tokens{}, &OAuthError{Status: status, Code: retrieve.ErrorCode, Description: retrieve.ErrorDescription}
			}
			if status == http.StatusNotFound {
				return Tokens{}, ErrUnavailable
			}
		}
		return Tokens{}, err
	}
	if tok.AccessToken == "" || tok.RefreshToken == "" {
		return Tokens{}, errors.New("everyapi: the sign-in server gave no key or no refresh token")
	}
	return Tokens{Key: tok.AccessToken, Refresh: tok.RefreshToken, Expires: tok.Expiry}, nil
}

// Browser is one browser sign-in: Open receives the address of EveryAPI's sign-in page, and Language is the interface's language, a tag of locales.Interface, which the page the browser comes back to says it is done in.
type Browser struct {
	Open     func(url string) error
	Language string
}

// Login signs in in the browser: it listens on a loopback port, opens EveryAPI's sign-in page with a PKCE challenge, waits for the browser to come back with the code, and trades it for the key. It ends with ctx, which is the user's cancel and the time it waits.
func Login(ctx context.Context, cfg Config, b Browser) (Tokens, error) {
	cfg = cfg.withDefaults()
	if b.Open == nil {
		return Tokens{}, errors.New("everyapi: nothing opens the browser")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Tokens{}, err
	}
	redirect := fmt.Sprintf("http://127.0.0.1:%d/callback", ln.Addr().(*net.TCPAddr).Port)
	conf := oauthConfig(cfg, redirect)
	verifier := oauth2.GenerateVerifier()
	state, err := randomState()
	if err != nil {
		ln.Close()
		return Tokens{}, err
	}
	type result struct {
		code string
		err  error
	}
	got := make(chan result, 1)
	msgs := locales.In(b.Language)
	srv := &http.Server{ReadHeaderTimeout: 10 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/callback" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		// A request without this sign-in's state is not EveryAPI's answer to it, as another page sending the browser here, and does not end the sign-in.
		if q.Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}
		res := result{code: q.Get("code")}
		switch {
		case q.Get("error") != "":
			res = result{err: &OAuthError{Code: q.Get("error"), Description: q.Get("error_description")}}
		case res.code == "":
			res = result{err: errors.New("everyapi: the sign-in page came back without a code")}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		text := msgs.EveryAPIBrowserSignedIn()
		if res.err != nil {
			w.WriteHeader(http.StatusBadRequest)
			text = msgs.EveryAPIBrowserFailed()
		}
		_, _ = io.WriteString(w, "<!doctype html><meta charset=utf-8><title>"+html.EscapeString(msgs.AppName())+"</title><p>"+html.EscapeString(text)+"</p>")
		select {
		case got <- res:
		default:
		}
	})}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		shut, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
	}()
	page := conf.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
	if err := b.Open(page); err != nil {
		return Tokens{}, err
	}
	var code string
	select {
	case <-ctx.Done():
		return Tokens{}, ctx.Err()
	case res := <-got:
		if res.err != nil {
			return Tokens{}, res.err
		}
		code = res.code
	}
	opts := append([]oauth2.AuthCodeOption{oauth2.VerifierOption(verifier)}, deviceParams(cfg)...)
	return tokensOf(conf.Exchange(withClient(ctx, cfg), code, opts...))
}

// Device is a device-code sign-in under way, for a computer whose browser cannot come back to Kando: the user opens URL on any device, signs in and confirms Code, and Wait gets the key once they did.
type Device struct {
	Code, URL string
	cfg       Config
	auth      *oauth2.DeviceAuthResponse
}

// StartDevice asks EveryAPI for a device code.
func StartDevice(ctx context.Context, cfg Config) (*Device, error) {
	cfg = cfg.withDefaults()
	auth, err := oauthConfig(cfg, "").DeviceAuth(withClient(ctx, cfg), deviceParams(cfg)...)
	if err != nil {
		_, err = tokensOf(nil, err)
		return nil, err
	}
	link := auth.VerificationURIComplete
	if link == "" {
		link = auth.VerificationURI
	}
	if u, perr := url.Parse(link); perr != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, fmt.Errorf("everyapi: the device code came with no web address")
	}
	return &Device{Code: auth.UserCode, URL: link, cfg: cfg, auth: auth}, nil
}

// Wait polls EveryAPI, as often as it allows, until the user approved the code, declined it (ErrDenied) or it expired (ErrExpired), or ctx ends.
func (d *Device) Wait(ctx context.Context) (Tokens, error) {
	return tokensOf(oauthConfig(d.cfg, "").DeviceAccessToken(withClient(ctx, d.cfg), d.auth, deviceParams(d.cfg)...))
}

// Refresh trades refresh for a new key and a new refresh token, which takes the place of refresh: EveryAPI revokes it as it answers.
func Refresh(ctx context.Context, cfg Config, refresh string) (Tokens, error) {
	cfg = cfg.withDefaults()
	src := oauthConfig(cfg, "").TokenSource(withClient(ctx, cfg), &oauth2.Token{RefreshToken: refresh})
	return tokensOf(src.Token())
}

// Revoke ends the sign-in at EveryAPI (RFC 7009): its refresh token, and with it the key it gave. EveryAPI answers 200 for a token it does not know, so only a request that did not get through fails.
func Revoke(ctx context.Context, cfg Config, refresh string) error {
	cfg = cfg.withDefaults()
	form := url.Values{"client_id": {cfg.ClientID}, "token": {refresh}, "token_type_hint": {"refresh_token"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.APIBase+"/api/oauth2/revoke", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := cfg.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<16))
	if res.StatusCode/100 != 2 {
		return fmt.Errorf("everyapi: revoking the sign-in answered HTTP %d", res.StatusCode)
	}
	return nil
}

func randomState() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
