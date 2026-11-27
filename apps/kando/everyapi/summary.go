package everyapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// DefaultQuotaPerUnit is how many of EveryAPI's quota units make one US dollar, its published peg, which /api/status says for the server at hand (quota_per_unit).
const DefaultQuotaPerUnit = 500_000

// Summary is what the account has, as GET /api/usage/account says for its key: who it is and its balance in Currency (US dollars, converted from EveryAPI's quota units at the server's own rate, which the summary's pricing says, or else /api/status), and the balance as EveryAPI counts it. The prices the summary may list per model are not read: the page shows the balance alone.
type Summary struct {
	Name     string
	Balance  float64
	Currency string
	Quota    int64
}

// Low is a balance under which the AI 服务 page says to top up soon: about what a few films cost to translate.
const Low = 1.0

// Low reports whether the balance is low enough to top up soon.
func (s Summary) Low() bool { return s.Balance < Low }

// FetchSummary asks EveryAPI what the account of key has. A key EveryAPI refuses is ErrSignedOut, and an account out of credit ErrOutOfCredit.
func FetchSummary(ctx context.Context, cfg Config, key string) (Summary, error) {
	cfg = cfg.withDefaults()
	var body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			Username    string `json:"username"`
			DisplayName string `json:"display_name"`
			Wallet      struct {
				Quota    int64  `json:"quota"`
				Currency string `json:"currency"`
			} `json:"wallet"`
			Pricing *struct {
				QuotaPerUnit float64 `json:"quota_per_unit"`
			} `json:"pricing"`
		} `json:"data"`
	}
	status, err := getJSON(ctx, cfg, "/api/usage/account", key, &body)
	switch {
	case err != nil:
		return Summary{}, err
	case status == http.StatusUnauthorized:
		return Summary{}, ErrSignedOut
	case status == http.StatusPaymentRequired:
		return Summary{}, ErrOutOfCredit
	case status/100 != 2 || !body.Success:
		if msg := strings.TrimSpace(body.Message); msg != "" {
			return Summary{}, fmt.Errorf("everyapi: the account summary failed: %s", msg)
		}
		return Summary{}, fmt.Errorf("everyapi: the account summary answered HTTP %d", status)
	}
	d := body.Data
	var perUnit float64
	if d.Pricing != nil && d.Pricing.QuotaPerUnit > 0 {
		perUnit = d.Pricing.QuotaPerUnit
	} else {
		perUnit = quotaPerUnit(ctx, cfg)
	}
	s := Summary{Name: strings.TrimSpace(d.DisplayName), Quota: d.Wallet.Quota, Currency: d.Wallet.Currency, Balance: float64(d.Wallet.Quota) / perUnit}
	if s.Name == "" {
		s.Name = strings.TrimSpace(d.Username)
	}
	if s.Currency == "" {
		s.Currency = "USD"
	}
	return s, nil
}

// quotaPerUnit is the server's quota units per dollar from its public /api/status, DefaultQuotaPerUnit when it does not say.
func quotaPerUnit(ctx context.Context, cfg Config) float64 {
	var body struct {
		Data struct {
			QuotaPerUnit float64 `json:"quota_per_unit"`
		} `json:"data"`
	}
	if status, err := getJSON(ctx, cfg, "/api/status", "", &body); err == nil && status/100 == 2 && body.Data.QuotaPerUnit > 0 {
		return body.Data.QuotaPerUnit
	}
	return DefaultQuotaPerUnit
}

// getJSON sends GET path to the API, with key as the bearer key when it is not "", and reads the JSON reply into v.
func getJSON(ctx context.Context, cfg Config, path, key string, v any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.APIBase+path, nil)
	if err != nil {
		return 0, err
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Accept", "application/json")
	res, err := cfg.Client.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	decodeReply(res, v)
	return res.StatusCode, nil
}
