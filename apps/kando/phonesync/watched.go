package phonesync

import (
	"time"

	"kando/packages/study"
	psync "kando/packages/sync"
)

// WatchedRecords are book's time watched as records, a device's day each (psync.KindWatched). Their fields carry no stamp: the ledger stamps one whose time changed on this computer as it observes them, and keeps a phone's own stamp on the time it put.
func WatchedRecords(book study.Book) []psync.Record {
	out := make([]psync.Record, 0, len(book.Watched))
	for _, w := range book.Watched {
		if w.Device == "" || w.Day == "" || w.Time <= 0 {
			continue
		}
		out = append(out, psync.Record{Kind: psync.KindWatched, ID: psync.WatchedID(w.Device, w.Day), Fields: map[string]psync.Field{
			psync.FieldWatchedMS: {Value: psync.Value(w.Time.Milliseconds())},
		}})
	}
	return out
}

// WatchedOf are watched records, as the ledger has them once merged, as book takes them in (study.Book.MergeWatched). A record whose ID names no device and day, or with no time, is left out.
func WatchedOf(records []psync.Record) []study.Watched {
	var out []study.Watched
	for _, r := range records {
		if r.Kind != psync.KindWatched {
			continue
		}
		device, day, ok := psync.ParseWatchedID(r.ID)
		d := msField(r.Fields[psync.FieldWatchedMS])
		if !ok || d <= 0 {
			continue
		}
		out = append(out, study.Watched{Day: day, Device: device, Time: d})
	}
	return out
}

// watchedAccepts reports why a watched record a device put cannot be kept, "" when it can: its ID names no device and day, it is of a film, or it changes the time of another device than the one that put it, which the desk stamps every change with.
func watchedAccepts(r psync.Record) string {
	device, _, ok := psync.ParseWatchedID(r.ID)
	if !ok || r.Film != "" {
		return "watched " + r.ID + " names no device and day"
	}
	for _, f := range r.Fields {
		if f.Device != device {
			return "watched " + r.ID + " is another device's time"
		}
	}
	return ""
}

// watchedFieldValid reports whether x is a value field name of a watched record may hold: its time, a number of milliseconds from 0 to a day's.
func watchedFieldValid(name string, x any) bool {
	if name != psync.FieldWatchedMS {
		return true
	}
	ms, ok := x.(float64)
	return ok && ms >= 0 && ms <= float64(24*time.Hour/time.Millisecond)
}
