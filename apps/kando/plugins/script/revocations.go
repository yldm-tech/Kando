package script

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

// revocationsMax is the most bytes read of a revocation list; revocationsChecked the file in the plugins folder that says when the list was last fetched, so a restart does not fetch it again the same day.
const (
	revocationsMax     = 1 << 20
	revocationsChecked = ".revocations-checked"
)

// FetchRevocations fetches the revocation list at target with client and gives it to trust, and reports whether trust took it: a list it holds already, or an older one, is not taken (pkg.ErrStale), and a 404 is no list published, which is no error. It waits 30 seconds at most.
func FetchRevocations(client *http.Client, target string, trust *pkg.Trust) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return false, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, errors.New("the revocation list answered " + resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, revocationsMax+1))
	if err != nil {
		return false, err
	}
	if len(data) > revocationsMax {
		return false, errors.New("the revocation list is too large")
	}
	list, err := pkg.ParseRevocations(data)
	if err != nil {
		return false, err
	}
	if err := trust.UpdateRevocations(list); err != nil {
		if errors.Is(err, pkg.ErrStale) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// RevocationsChecked is when the revocation list was last fetched, as the plugins folder root records it; the zero time when it never was.
func RevocationsChecked(root string) time.Time {
	data, err := os.ReadFile(filepath.Join(root, revocationsChecked))
	if err != nil {
		return time.Time{}
	}
	sec, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}

// SetRevocationsChecked records in the plugins folder root that the revocation list was fetched at, which a later RevocationsChecked reads.
func SetRevocationsChecked(root string, at time.Time) {
	_ = os.WriteFile(filepath.Join(root, revocationsChecked), []byte(strconv.FormatInt(at.Unix(), 10)+"\n"), 0o644)
}
