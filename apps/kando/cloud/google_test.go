package cloud

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestLoginExchangesTheLoopbackCode(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		if r.Form.Get("code") == "" || r.Form.Get("code_verifier") == "" {
			http.Error(w, "missing verifier", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"at","refresh_token":"rt","token_type":"Bearer","expires_in":3600}`)
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer at" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"email":"a@b.c"}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	acc, err := Login(context.Background(), GoogleConfig{
		ClientID:    "client",
		AuthURL:     srv.URL + "/auth",
		TokenURL:    srv.URL + "/token",
		UserInfoURL: srv.URL + "/userinfo",
		Open: func(raw string) error {
			u, err := url.Parse(raw)
			if err != nil {
				return err
			}
			q := u.Query()
			if q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" {
				return fmt.Errorf("pkce missing")
			}
			if q.Get("access_type") != "offline" || q.Get("prompt") != "consent" {
				return fmt.Errorf("consent missing")
			}
			if !strings.Contains(q.Get("scope"), "email") {
				return fmt.Errorf("scope missing")
			}
			return hitCallback(q.Get("redirect_uri"), "abc", q.Get("state"))
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if acc.Email != "a@b.c" {
		t.Fatalf("email %q", acc.Email)
	}
	if acc.RefreshToken != "rt" {
		t.Fatal("refresh token was not returned")
	}
}

func TestLoginRejectsABadState(t *testing.T) {
	_, err := Login(context.Background(), GoogleConfig{
		ClientID: "client",
		Open: func(raw string) error {
			u, err := url.Parse(raw)
			if err != nil {
				return err
			}
			return hitCallback(u.Query().Get("redirect_uri"), "abc", "nope")
		},
	})
	if err == nil || !strings.Contains(err.Error(), "state") {
		t.Fatalf("err = %v", err)
	}
}

func hitCallback(redirect, code, state string) error {
	cb, err := url.Parse(redirect)
	if err != nil {
		return err
	}
	q := url.Values{}
	q.Set("code", code)
	q.Set("state", state)
	cb.RawQuery = q.Encode()
	var last error
	for i := 0; i < 50; i++ {
		res, err := http.Get(cb.String())
		if err != nil {
			last = err
			time.Sleep(10 * time.Millisecond)
			continue
		}
		res.Body.Close()
		return nil
	}
	return last
}

// The page the browser lands on once Google signed the user in speaks the interface's language the sign-in started in, as the server answers on a goroutine of its own, where there is no scope to read it from.
func TestLoginLandingPageSpeaksTheLanguage(t *testing.T) {
	for language, want := range map[string]string{"en": "Signed in. You can close this page.", "ja": "ログインしました。このページは閉じてかまいません。"} {
		var page string
		_, _ = Login(context.Background(), GoogleConfig{
			ClientID: "client",
			TokenURL: "http://127.0.0.1:1/token",
			Language: language,
			Open: func(raw string) error {
				u, err := url.Parse(raw)
				if err != nil {
					return err
				}
				cb, err := url.Parse(u.Query().Get("redirect_uri"))
				if err != nil {
					return err
				}
				cb.RawQuery = url.Values{"code": {"abc"}, "state": {u.Query().Get("state")}}.Encode()
				res, err := http.Get(cb.String())
				if err != nil {
					return err
				}
				defer res.Body.Close()
				body, err := io.ReadAll(res.Body)
				page = string(body)
				return err
			},
		})
		if !strings.Contains(page, want) {
			t.Errorf("%s: landing page %q, want %q", language, page, want)
		}
	}
}
