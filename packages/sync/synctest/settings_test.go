package synctest

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"kando/packages/sync"
)

func TestTheSettingsOverV2(t *testing.T) {
	ctx := context.Background()
	d := New(t, "Mac")
	d.Offer("123456")
	c := sync.NewClient(d.Addr, d.Fingerprint, "")
	t.Cleanup(c.Close)
	paired, err := c.Pair(ctx, sync.PairRequest{Code: "123456", Device: "Pixel", Platform: sync.PlatformAndroid})
	if err != nil {
		t.Fatal(err)
	}
	c = sync.NewClient(d.Addr, d.Fingerprint, paired.Token)
	t.Cleanup(c.Close)

	got, err := c.Settings(ctx)
	if err != nil || got.Target != "zh-Hans" {
		t.Fatalf("settings %+v, %v", got, err)
	}
	got, err = c.PutSettings(ctx, sync.SettingsPut{Target: "ja"})
	if err != nil || got.Target != "ja" || d.Target() != "ja" {
		t.Fatalf("put answered %+v, %v; the desktop translates into %q", got, err, d.Target())
	}
	var e *sync.Error
	if _, err := c.PutSettings(ctx, sync.SettingsPut{Target: "xx"}); !errors.As(err, &e) || e.Status != http.StatusBadRequest || e.Code != sync.CodeInvalid {
		t.Fatalf("a language the desktop does not translate into answered %v", err)
	}
	if d.Target() != "ja" {
		t.Fatalf("a refused change left the desktop translating into %q", d.Target())
	}
}
