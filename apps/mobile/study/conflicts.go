package study

import (
	"slices"
	"time"

	"kando/packages/sync"
)

// Conflict is an open conflict of a library's learning state as the phone shows it (sync.Conflict): the field changed on two devices apart, the film and the place in it, and the two versions, the kept one first.
type Conflict struct {
	ID    string
	Kind  string
	Field string
	// Film is the ID of the film the record belongs to, "" for none, and Title its title as the record has it, "" when it has none.
	Film, Title string
	// At is where in the film the line is, -1 for a word or a record of no place.
	At time.Duration
	// Word is the word a conflict of the vocabulary is of, "" for one of a line.
	Word string
	// Line is the line's original, shown over the two versions when they are not of it, "" for none.
	Line string
	// Kept is the version the sync kept, Lost the one it set aside.
	Kept, Lost Version
	// Both is whether the two can be kept together, as two notes can (sync.Joinable).
	Both bool
	// Rev is the revision at which the computer recorded it, which the conflicts are ordered by.
	Rev int64
}

// Version is one of a conflict's versions: the name of the device it was made on, "" for one the computer does not name, whether that is this phone, when it was made, and its text.
type Version struct {
	Device string
	Mine   bool
	At     time.Time
	Text   string
}

// Conflicts are l's open conflicts, the oldest first, me being this phone's ID on the computer (store.Desktop.DeviceID).
func Conflicts(l Library, me string) []Conflict {
	version := func(f sync.Field) Version {
		return Version{Device: l.Devices[f.Device], Mine: f.Device != "" && f.Device == me, At: f.At, Text: sync.Text(f.Value)}
	}
	var out []Conflict
	for _, c := range l.Conflicts {
		if c.Resolved != nil {
			continue
		}
		x := Conflict{ID: c.ID, Kind: c.Kind, Field: c.Field, Film: c.Film, At: -1, Kept: version(c.Kept), Lost: version(c.Lost), Both: sync.Joinable(c.Kind, c.Field), Rev: c.Rev}
		if r, ok := View(l, c.Kind, c.Record); ok {
			read(r, sync.FieldTitle, &x.Title)
			switch c.Kind {
			case sync.KindLine:
				var ms float64
				if f, ok := r.Fields[sync.FieldStartMS]; ok && !sync.IsNull(f.Value) {
					read(r, sync.FieldStartMS, &ms)
					x.At = time.Duration(ms) * time.Millisecond
				}
				if c.Field != sync.FieldOriginal {
					read(r, sync.FieldOriginal, &x.Line)
				}
			case sync.KindWord:
				read(r, sync.FieldWord, &x.Word)
			}
		}
		if c.Kind == sync.KindCorrection {
			if film, start, _, ok := sync.ParseCorrectionID(c.Record); ok {
				x.At = time.Duration(start) * time.Millisecond
				if x.Film == "" {
					x.Film = film
				}
			}
		}
		out = append(out, x)
	}
	slices.SortStableFunc(out, func(a, b Conflict) int { return int(a.Rev - b.Rev) })
	return out
}

// Resolved is l once the computer resolved a conflict, answering res: its records as resolved in place of l's, and the conflicts it closed gone.
func Resolved(l Library, res sync.StatePutResult) Library {
	out := l
	out.Records = slices.Clone(l.Records)
	for _, r := range res.Records {
		out.Records = upsert(out.Records, r)
	}
	out.Conflicts = slices.DeleteFunc(slices.Clone(l.Conflicts), func(x sync.Conflict) bool {
		return slices.ContainsFunc(res.Conflicts, func(c sync.Conflict) bool { return c.ID == x.ID && c.Resolved != nil })
	})
	return out
}

// Forget is l without conflict id, as one the computer no longer has open: settled on another device meanwhile.
func Forget(l Library, id string) Library {
	out := l
	out.Conflicts = slices.DeleteFunc(slices.Clone(l.Conflicts), func(x sync.Conflict) bool { return x.ID == id })
	return out
}
