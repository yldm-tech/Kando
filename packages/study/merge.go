package study

import (
	"slices"
	"strings"
)

// Merge takes into the book another device's lines and the lines it removed, as two devices' books sync, and reports whether the book changed. It is the same whichever book merges into which, and merging again changes nothing.
//
// A line removed on either device is removed on both, whatever was changed in it since: the removal names the line's id, so a line saved again after it, which has an id of its own, stays. Of a line both have, each field is the one changed later (Stamps); its box and its due date go together. A line both devices saved on their own, at the same start of the same video, becomes one: the one with the smaller id, which takes the other's fields where they changed later and the earlier of the two times it was saved, and the other is removed.
func (b *Book) Merge(lines []Line, removed []Removed) bool {
	changed := false
	for _, r := range removed {
		changed = b.forget(r) || changed
	}
	for _, in := range lines {
		if in.ID == "" || slices.ContainsFunc(b.Removed, func(r Removed) bool { return r.ID == in.ID }) {
			continue
		}
		i := b.Index(in.ID)
		if i < 0 {
			i = b.Find(in.Video, in.Start)
		}
		if i < 0 {
			b.insert(in)
			changed = true
			continue
		}
		have := b.Lines[i]
		merged := mergeLine(have, in)
		if have.ID != in.ID {
			// The line was saved on both devices: the larger id goes.
			lost := have
			if strings.Compare(in.ID, have.ID) > 0 {
				lost = in
			}
			gone := Removed{ID: lost.ID, Video: lost.Video, Start: lost.Start, At: Stamp{At: lost.SavedAt, Device: lost.Device}}
			b.Lines = slices.Delete(b.Lines, i, i+1)
			b.forget(gone)
			b.Lines = slices.Insert(b.Lines, min(i, len(b.Lines)), merged)
			changed = true
			continue
		}
		if !sameRecord(have, merged) {
			b.Lines[i] = merged
			changed = true
		}
	}
	return changed
}

// forget records r, the later of two removals of one line, and takes its line out of the book; it reports whether that changed the book.
func (b *Book) forget(r Removed) bool {
	if r.ID == "" {
		return false
	}
	changed := false
	if k := slices.IndexFunc(b.Removed, func(have Removed) bool { return have.ID == r.ID }); k < 0 {
		b.Removed = append(b.Removed, r)
		changed = true
	} else if r.At.After(b.Removed[k].At) {
		b.Removed[k] = r
		changed = true
	}
	if i := b.Index(r.ID); i >= 0 {
		b.Lines = slices.Delete(b.Lines, i, i+1)
		changed = true
	}
	return changed
}

// mergeLine is a and b, two copies of one line, field by field: each field the one changed later. The fields that do not change after a line is saved are the copy with the smaller id's, and it was saved at the earlier of the two times.
func mergeLine(a, b Line) Line {
	out, other := a, b
	if strings.Compare(b.ID, a.ID) < 0 {
		out, other = b, a
	}
	if out.SavedAt.IsZero() || (!other.SavedAt.IsZero() && other.SavedAt.Before(out.SavedAt)) {
		out.SavedAt = other.SavedAt
	}
	if other.Stamps.Video.After(out.Stamps.Video) {
		out.Video, out.Stamps.Video = other.Video, other.Stamps.Video
	}
	if other.Stamps.Original.After(out.Stamps.Original) {
		out.Original, out.Stamps.Original = other.Original, other.Stamps.Original
	}
	if other.Stamps.Translation.After(out.Stamps.Translation) {
		out.Translation, out.Stamps.Translation = other.Translation, other.Stamps.Translation
	}
	if other.Stamps.Note.After(out.Stamps.Note) {
		out.Note, out.Stamps.Note = other.Note, other.Stamps.Note
	}
	if other.Stamps.Review.After(out.Stamps.Review) {
		out.Box, out.Due, out.Stamps.Review = other.Box, other.Due, other.Stamps.Review
	}
	return out
}

// sameRecord reports whether a and b hold the same of every field Merge changes.
func sameRecord(a, b Line) bool {
	return a.ID == b.ID && a.Video == b.Video && a.Original == b.Original && a.Translation == b.Translation && a.Note == b.Note &&
		a.Box == b.Box && a.Due.Equal(b.Due) && a.SavedAt.Equal(b.SavedAt) && sameStamps(a.Stamps, b.Stamps)
}

// sameStamps reports whether a and b are the same stamps, their times compared by Equal, as a time read from the file has no monotonic reading.
func sameStamps(a, b Stamps) bool {
	same := func(x, y Stamp) bool { return x.At.Equal(y.At) && x.Device == y.Device }
	return same(a.Video, b.Video) && same(a.Original, b.Original) && same(a.Translation, b.Translation) && same(a.Note, b.Note) && same(a.Review, b.Review)
}
