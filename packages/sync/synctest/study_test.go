package synctest

import (
	"context"
	"errors"
	"testing"

	"kando/packages/sync"
)

func TestLookupsAndExplanationsOverV2(t *testing.T) {
	d := New(t, "Studio")
	d.AddLibrary("lib", "Class")
	d.Offer("123456")
	c := sync.NewClient(d.Addr, d.Fingerprint, "")
	defer c.Close()
	ctx := context.Background()
	if _, err := c.Pair(ctx, sync.PairRequest{Code: "123456", Device: "Phone", Platform: sync.PlatformIOS}); err != nil {
		t.Fatal(err)
	}
	d.SetEntry("River", sync.WordEntry{Word: "river", Reading: "ˈrɪvər", Meaning: "河", Language: "简体中文"})
	e, err := c.LookUp(ctx, sync.WordLookup{Library: "lib", Film: "f1", Word: "river", Line: "Every river begins."})
	if err != nil || e.Meaning != "河" || e.Reading != "ˈrɪvər" {
		t.Fatalf("lookup: %+v, %v", e, err)
	}
	x, err := c.Explain(ctx, sync.LineExplain{Library: "lib", Film: "f1", Lines: []sync.Cue{{Original: "Every river begins."}}})
	if err != nil || x.Text != "Explained: Every river begins." {
		t.Fatalf("explain: %+v, %v", x, err)
	}
	if got := d.Lookups(); len(got) != 1 || got[0].Line != "Every river begins." || got[0].Film != "f1" {
		t.Fatalf("the desktop was asked %+v", got)
	}
	d.SetNoAI(true)
	var e2 *sync.Error
	if _, err := c.LookUp(ctx, sync.WordLookup{Word: "river"}); !errors.As(err, &e2) || e2.Code != sync.CodeNoAI {
		t.Fatalf("without an AI service: %v", err)
	}
}
