package cloud

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"kando/apps/kando/locales"

	"golang.org/x/oauth2"
)

const (
	defaultAuthURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	defaultTokenURL = "https://oauth2.googleapis.com/token"
	defaultUserInfo = "https://www.googleapis.com/oauth2/v2/userinfo"
)

// GoogleConfig is one login attempt. Empty URLs use Google's. Open receives the authorization URL.
type GoogleConfig struct {
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	UserInfoURL  string
	Open         func(string) error
}

// Account is what we keep after a login. The access token is not stored.
type Account struct {
	Email        string
	RefreshToken string
}

// Login runs a loopback OAuth flow with PKCE and returns the account email and refresh token.
func Login(ctx context.Context, cfg GoogleConfig) (Account, error) {
	if strings.TrimSpace(cfg.ClientID) == "" {
		return Account{}, errors.New("google client id is missing")
	}
	if cfg.Open == nil {
		return Account{}, errors.New("google opener is missing")
	}
	authURL := cfg.AuthURL
	if authURL == "" {
		authURL = defaultAuthURL
	}
	tokenURL := cfg.TokenURL
	if tokenURL == "" {
		tokenURL = defaultTokenURL
	}
	infoURL := cfg.UserInfoURL
	if infoURL == "" {
		infoURL = defaultUserInfo
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Account{}, err
	}
	redirect := fmt.Sprintf("http://127.0.0.1:%d/callback", ln.Addr().(*net.TCPAddr).Port)
	conf := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  redirect,
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     oauth2.Endpoint{AuthURL: authURL, TokenURL: tokenURL},
	}
	verifier := oauth2.GenerateVerifier()
	state, err := randomState()
	if err != nil {
		ln.Close()
		return Account{}, err
	}
	auth := conf.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("prompt", "consent"),
	)

	type result struct {
		code string
		err  error
	}
	ch := make(chan result, 1)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/callback" {
			http.NotFound(w, r)
			return
		}
		query := r.URL.Query()
		send := func(res result, status int, page string) {
			http.Error(w, page, status)
			select {
			case ch <- res:
			default:
			}
		}
		if query.Get("state") != state {
			send(result{err: errors.New("state mismatch")}, http.StatusBadRequest, "state mismatch")
			return
		}
		if query.Get("error") != "" {
			send(result{err: errors.New("google denied")}, http.StatusBadRequest, "denied")
			return
		}
		code := query.Get("code")
		if code == "" {
			send(result{err: errors.New("missing code")}, http.StatusBadRequest, "missing code")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<!doctype html><meta charset=utf-8><title>"+html.EscapeString(locales.CloudSignedInTitle())+"</title><p>"+html.EscapeString(locales.CloudSignedIn())+"</p>")
		select {
		case ch <- result{code: code}:
		default:
		}
	})}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		shut, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
	}()

	if err := cfg.Open(auth); err != nil {
		return Account{}, err
	}
	var code string
	select {
	case <-ctx.Done():
		return Account{}, ctx.Err()
	case got := <-ch:
		if got.err != nil {
			return Account{}, got.err
		}
		code = got.code
	}

	tok, err := conf.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return Account{}, err
	}
	email, err := fetchEmail(ctx, infoURL, tok.AccessToken)
	if err != nil {
		return Account{}, err
	}
	if tok.RefreshToken == "" {
		return Account{}, errors.New("google did not return a refresh token")
	}
	return Account{Email: email, RefreshToken: tok.RefreshToken}, nil
}

func fetchEmail(ctx context.Context, infoURL, access string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, infoURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+access)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if res.StatusCode != http.StatusOK {
		return "", errors.New("userinfo failed")
	}
	var payload struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	email := strings.TrimSpace(payload.Email)
	if !strings.Contains(email, "@") {
		return "", errors.New("userinfo has no email")
	}
	return email, nil
}

func randomState() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
