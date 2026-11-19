package phonesync

import (
	"slices"
	"testing"
	"time"

	"kando/packages/study"
	psync "kando/packages/sync"
)

func TestWatchedRecordsRoundTrip(t *testing.T) {
	book := study.Book{Watched: []study.Watched{{Day: "2026-11-16", Device: "mac", Time: 90 * time.Second}, {Day: "2026-11-17", Device: "phone/1", Time: time.Hour}}}
	records := WatchedRecords(book)
	if len(records) != 2 || records[1].ID != "phone/1/2026-11-17" || records[1].Film != "" {
		t.Fatalf("records %+v", records)
	}
	if got := WatchedOf(records); !slices.Equal(got, book.Watched) {
		t.Errorf("read back as %+v", got)
	}
	bad := []psync.Record{{Kind: psync.KindWatched, ID: "phone", Fields: map[string]psync.Field{psync.FieldWatchedMS: {Value: psync.Value(1000)}}}}
	if got := WatchedOf(bad); len(got) != 0 {
		t.Errorf("a record that names no day read as %+v", got)
	}
}

func TestStateAcceptsADevicesOwnTimeWatched(t *testing.T) {
	known := func(string) bool { return false }
	have := func(string, string) bool { return false }
	own := psync.Record{Kind: psync.KindWatched, ID: psync.WatchedID("p1", "2026-11-17"), Fields: map[string]psync.Field{
		psync.FieldWatchedMS: {Value: psync.Value(60_000), Stamp: psync.Stamp{Device: "p1"}}}}
	if why := StateAccepts([]psync.Record{own}, known, have); why != "" {
		t.Errorf("a device's own time refused: %s", why)
	}
	other := own
	other.Fields = map[string]psync.Field{psync.FieldWatchedMS: {Value: psync.Value(60_000), Stamp: psync.Stamp{Device: "p2"}}}
	if StateAccepts([]psync.Record{other}, known, have) == "" {
		t.Error("another device's time accepted")
	}
	for _, v := range []any{-1, "long", 25 * 3_600_000} {
		r := own
		r.Fields = map[string]psync.Field{psync.FieldWatchedMS: {Value: psync.Value(v), Stamp: psync.Stamp{Device: "p1"}}}
		if StateAccepts([]psync.Record{r}, known, have) == "" {
			t.Errorf("a time of %v accepted", v)
		}
	}
	film := own
	film.Film = "f1"
	if StateAccepts([]psync.Record{film}, known, have) == "" {
		t.Error("a time watched of a film accepted")
	}
}
