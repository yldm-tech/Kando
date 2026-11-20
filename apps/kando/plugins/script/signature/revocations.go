package signature

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

// RevocationsMax is the most bytes read of a revocation list.
const RevocationsMax = 1 << 20

// ErrRevocationsTooLarge is a revocation list over RevocationsMax bytes.
var ErrRevocationsTooLarge = errors.New("the revocation list is too large")

// ParseRevocations parses the revocation list data, at most RevocationsMax bytes (pkg.ParseRevocations). Whose root signed it is checked as a trust takes it (pkg.Trust.UpdateRevocations).
func ParseRevocations(data []byte) (*pkg.Revocations, error) {
	if len(data) > RevocationsMax {
		return nil, ErrRevocationsTooLarge
	}
	return pkg.ParseRevocations(data)
}

// FetchRevocations fetches and parses the revocation list at target with client (ParseRevocations): nil and no error for a 404, which is no list published. It waits 30 seconds at most. The app gives what it fetches to the plugins folder's trust (script.FetchRevocations), the plugin directory's CI tool to the trust it checks packs with (cmd/kando-plugin --revocations).
func FetchRevocations(client *http.Client, target string) (*pkg.Revocations, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("the revocation list answered " + resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, RevocationsMax+1))
	if err != nil {
		return nil, err
	}
	return ParseRevocations(data)
}
