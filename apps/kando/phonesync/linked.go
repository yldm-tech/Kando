package phonesync

import (
	"encoding/json"
	"slices"
	"time"

	psync "kando/packages/sync"
)

// What the desktop shows of what the phones do (yldm-tech/Kando#218), read from a library's ledger: how much of its learning state phones were told (Counts), where a phone last left a film (Handoff, LeftOn) and what a conflict's versions say (Text).

// Counts is how many words and saved lines the ledger holds, tombstones left out, and how many films it has a place to continue from.
func Counts(l *psync.Ledger) (words, lines, positions int) {
	for _, r := range l.Records(psync.KindWord) {
		if r.Removed == nil {
			words++
		}
	}
	for _, r := range l.Records(psync.KindLine) {
		if r.Removed == nil {
			lines++
		}
	}
	for _, r := range l.Records(psync.KindPosition) {
		if PositionOf(r) > 0 {
			positions++
		}
	}
	return words, lines, positions
}

// Left is where a film was left, by which device and when.
type Left struct {
	Film  string
	At    time.Duration
	Stamp psync.Stamp
}

// Handoff is where a phone last left one of the ledger's films, the latest of the positions set on a device other than desktop (the desktop's own ID), and false when the latest position of all was set on the desktop, or none was set on a phone: a film the desktop played since is the desktop's again. A position at a film's start, as one watched to its end, is no place to continue from.
func Handoff(l *psync.Ledger, desktop string) (Left, bool) {
	var latest psync.Field
	var film string
	for _, r := range l.Records(psync.KindPosition) {
		f, ok := r.Fields[psync.FieldPositionMS]
		if !ok || PositionOf(r) <= 0 {
			continue
		}
		if film == "" || f.Stamp.After(latest.Stamp) {
			latest, film = f, r.ID
		}
	}
	if film == "" || latest.Device == desktop || latest.Device == "" {
		return Left{}, false
	}
	return Left{Film: film, At: msField(latest), Stamp: latest.Stamp}, true
}

// LeftOn is the device that last set where film was left, "" when none did or it was desktop.
func LeftOn(l *psync.Ledger, film, desktop string) string {
	r, ok := l.Record(psync.KindPosition, film)
	if !ok || PositionOf(r) <= 0 {
		return ""
	}
	if d := r.Fields[psync.FieldPositionMS].Device; d != desktop {
		return d
	}
	return ""
}

// Text is a text field's value as the page shows it, "" for one that is not a string.
func Text(v json.RawMessage) string {
	var s string
	if json.Unmarshal(v, &s) != nil {
		return ""
	}
	return s
}

// Joinable reports whether a conflict of field of a record of kind can keep both versions, one after the other: a note or a word's meaning, which say more together, unlike two translations of one line.
func Joinable(kind, field string) bool {
	return field == psync.FieldNote || (kind == psync.KindWord && field == psync.FieldMeaning)
}

// Joined is the value keeping both of a conflict's versions, the one kept first.
func Joined(c psync.Conflict) json.RawMessage {
	return psync.Value(Text(c.Kept.Value) + "\n" + Text(c.Lost.Value))
}

// OpenConflicts are the ledger's open conflicts, the oldest first.
func OpenConflicts(l *psync.Ledger) []psync.Conflict {
	out := slices.Clone(l.Conflicts())
	slices.SortFunc(out, func(a, b psync.Conflict) int { return int(a.Rev - b.Rev) })
	return out
}
