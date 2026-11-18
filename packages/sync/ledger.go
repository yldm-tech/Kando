package sync

import (
	"cmp"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"time"
)

// Ledger is the desktop's copy of a library's learning state: every record as last seen or put, with the revision it last changed at, and the conflicts recorded. The desktop keeps the state itself where it kept it before (the study record, the resume positions, the corrections files), most of which carry no stamps; the ledger finds what changed there by comparing it with its copy (Observe), stamps those changes, and gives the desktop back what devices put to write there (Put). It is not safe for concurrent use: the desktop uses it on one goroutine.
type Ledger struct {
	epoch     string
	rev       int64
	records   map[recordKey]Record
	conflicts []Conflict
}

type recordKey struct{ kind, id string }

// KeepResolved is how many resolved conflicts a ledger keeps, so a device that asks after an older revision than the oldest is told them all again (StateChanges.Reset) rather than miss a resolution.
const KeepResolved = 256

// LedgerVersion is the version of the ledger file this package writes. A file of a later version is not read (ErrLedgerNewer).
const LedgerVersion = 1

// ErrLedgerNewer is ReadLedger's error for a file a later Kando wrote.
var ErrLedgerNewer = errors.New("the learning state was kept by a newer Kando")

// NewLedger is an empty ledger in a new epoch.
func NewLedger() *Ledger {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return &Ledger{epoch: hex.EncodeToString(b), records: map[recordKey]Record{}}
}

// Epoch names the ledger, which its revisions count in.
func (l *Ledger) Epoch() string { return l.epoch }

// Rev is the revision of the ledger's last change.
func (l *Ledger) Rev() int64 { return l.rev }

// Record is the record kind id as the ledger has it.
func (l *Ledger) Record(kind, id string) (Record, bool) {
	r, ok := l.records[recordKey{kind, id}]
	return r.clone(), ok
}

// Records are the ledger's records of kind, tombstones among them, in the order of their revisions.
func (l *Ledger) Records(kind string) []Record {
	var out []Record
	for k, r := range l.records {
		if k.kind == kind {
			out = append(out, r.clone())
		}
	}
	slices.SortFunc(out, byRev)
	return out
}

func byRev(a, b Record) int {
	return cmp.Or(cmp.Compare(a.Rev, b.Rev), cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.ID, b.ID))
}

// Conflicts are the conflicts still open.
func (l *Ledger) Conflicts() []Conflict {
	var out []Conflict
	for _, c := range l.conflicts {
		if c.Resolved == nil {
			out = append(out, c)
		}
	}
	return out
}

// Observe takes current, the library's records as the desktop keeps them now, into the ledger, and reports whether that changed it. A field of current with a stamp keeps it, as a line's carries the study record's; one with none whose value differs from the ledger's changed on the desktop, and is stamped now, else it keeps the ledger's stamp. A record the ledger has that current lacks, and that covered says the desktop read, was removed on the desktop: one of a Tombstoned kind takes a tombstone stamped now, any other its fields set to null; a record covered does not say of, as one of a film whose files could not be read, stays as it is.
func (l *Ledger) Observe(current []Record, covered func(Record) bool, now Stamp) bool {
	changed := false
	seen := make(map[recordKey]bool, len(current))
	for _, in := range current {
		key := recordKey{in.Kind, in.ID}
		seen[key] = true
		have, ok := l.records[key]
		if !ok {
			r := in.clone()
			r.Rev = 0
			if r.Removed == nil {
				for name, f := range r.Fields {
					if f.Stamp.IsZero() {
						f.Stamp = now
					}
					r.Fields[name] = f
				}
			}
			l.put(r)
			changed = true
			continue
		}
		next, moved := observed(have, in, now)
		if moved {
			l.put(next)
			changed = true
		}
	}
	for key, have := range l.records {
		if seen[key] || !covered(have) {
			continue
		}
		next := have.clone()
		moved := false
		switch {
		case Tombstoned(key.kind):
			if next.Removed == nil {
				next.Removed, next.Fields, moved = &Stamp{At: now.At, Device: now.Device}, nil, true
			}
		default:
			for name, f := range next.Fields {
				if !IsNull(f.Value) {
					next.Fields[name], moved = Field{Value: Null, Stamp: now}, true
				}
			}
		}
		if moved {
			l.put(next)
			changed = true
		}
	}
	return changed
}

// observed is have with in, the record as the desktop keeps it now, taken in (Observe), and whether that changed it.
func observed(have, in Record, now Stamp) (Record, bool) {
	out := have.clone()
	changed := false
	if in.Film != out.Film {
		out.Film, changed = in.Film, true
	}
	if in.Removed != nil || have.Removed != nil {
		if Tombstoned(have.Kind) {
			if have.Removed == nil {
				out.Removed, out.Fields = cloneStamp(in.Removed), nil
				return out, true
			}
			return out, changed
		}
	}
	if out.Fields == nil {
		out.Fields = map[string]Field{}
	}
	for name, f := range in.Fields {
		h, ok := out.Fields[name]
		switch {
		case !f.Stamp.IsZero():
			if !ok || !SameValue(h.Value, f.Value) || !h.Stamp.Equal(f.Stamp) {
				out.Fields[name], changed = Field{Value: f.Value, Stamp: f.Stamp}, true
			}
		case !ok || !SameValue(h.Value, f.Value):
			out.Fields[name], changed = Field{Value: f.Value, Stamp: now}, true
		}
	}
	return out, changed
}

// put keeps r at the next revision.
func (l *Ledger) put(r Record) {
	l.rev++
	r.Rev = l.rev
	l.records[recordKey{r.Kind, r.ID}] = r
}

// Put merges records, a device's changes (MergeRecord), into the ledger, and records a conflict for each field where two changes made apart clashed, each conflict named by newID. It returns the records that changed, as they are now, for the desktop to keep where it keeps them, and the conflicts it recorded.
func (l *Ledger) Put(records []Record, newID func() string) (changed []Record, conflicts []Conflict) {
	for _, in := range records {
		key := recordKey{in.Kind, in.ID}
		have := l.records[key]
		out, moved, clashes := MergeRecord(have, in)
		if moved {
			l.put(out)
			changed = append(changed, l.records[key].clone())
		}
		for _, c := range clashes {
			conflict := Conflict{ID: newID(), Kind: in.Kind, Record: in.ID, Film: out.Film, Field: c.Field, Kept: c.Kept, Lost: c.Lost}
			conflicts = append(conflicts, l.addConflict(conflict))
		}
	}
	return changed, conflicts
}

// addConflict records c, open, at the next revision.
func (l *Ledger) addConflict(c Conflict) Conflict {
	l.rev++
	c.Rev = l.rev
	l.conflicts = append(l.conflicts, c)
	return c
}

// ErrNoConflict is Resolve's error for a conflict the ledger does not have open.
var ErrNoConflict = errors.New("no such open conflict")

// Resolve resolves the open conflict id with value, stamped at, which becomes the value of its record's field, and returns the record and the conflict as they are now.
func (l *Ledger) Resolve(id string, value []byte, at Stamp) (Record, Conflict, error) {
	i := slices.IndexFunc(l.conflicts, func(c Conflict) bool { return c.ID == id && c.Resolved == nil })
	if i < 0 {
		return Record{}, Conflict{}, ErrNoConflict
	}
	c := l.conflicts[i]
	key := recordKey{c.Kind, c.Record}
	r, ok := l.records[key]
	if ok && r.Removed == nil {
		// The resolution is a change of the field like any other, made after both of the values it chose between.
		r = r.clone()
		if r.Fields == nil {
			r.Fields = map[string]Field{}
		}
		latest := c.Kept.Stamp
		if cur, ok := r.Fields[c.Field]; ok && cur.Stamp.After(latest) {
			latest = cur.Stamp
		}
		if !at.At.After(latest.At) {
			at.At = latest.At.Add(time.Millisecond)
		}
		r.Fields[c.Field] = Field{Value: json.RawMessage(value), Stamp: at}
		l.put(r)
	}
	l.rev++
	c.Resolved, c.Rev = &Stamp{At: at.At, Device: at.Device}, l.rev
	l.conflicts[i] = c
	// Other conflicts of the same field are settled by the same choice.
	for j, o := range l.conflicts {
		if j != i && o.Resolved == nil && o.Kind == c.Kind && o.Record == c.Record && o.Field == c.Field {
			l.rev++
			o.Resolved, o.Rev = &Stamp{At: at.At, Device: at.Device}, l.rev
			l.conflicts[j] = o
		}
	}
	l.forgetResolved()
	rec, _ := l.Record(c.Kind, c.Record)
	return rec, c, nil
}

// forgetResolved keeps the last KeepResolved resolved conflicts.
func (l *Ledger) forgetResolved() {
	n := 0
	for _, c := range l.conflicts {
		if c.Resolved != nil {
			n++
		}
	}
	for i := 0; n > KeepResolved && i < len(l.conflicts); {
		if l.conflicts[i].Resolved != nil {
			l.conflicts = slices.Delete(l.conflicts, i, i+1)
			n--
			continue
		}
		i++
	}
}

// Since is the changes after revision since of epoch (StateGet), for library: all of them with Reset when epoch is not the ledger's, since is past its revision, or older than a resolved conflict it no longer keeps.
func (l *Ledger) Since(library, epoch string, since int64) StateChanges {
	out := StateChanges{Library: library, Epoch: l.epoch, Rev: l.rev, Records: []Record{}, Conflicts: []Conflict{}}
	if epoch != l.epoch || since < 0 || since > l.rev || since < l.forgotten() {
		out.Reset, since = true, 0
	}
	for _, r := range l.records {
		if r.Rev > since {
			out.Records = append(out.Records, r.clone())
		}
	}
	slices.SortFunc(out.Records, byRev)
	for _, c := range l.conflicts {
		if c.Rev > since && (c.Resolved == nil || !out.Reset) {
			out.Conflicts = append(out.Conflicts, c)
		}
	}
	slices.SortFunc(out.Conflicts, func(a, b Conflict) int { return cmp.Compare(a.Rev, b.Rev) })
	return out
}

// forgotten is the revision up to which resolved conflicts may have been forgotten: that of the oldest resolved one kept, once KeepResolved of them are kept, else 0.
func (l *Ledger) forgotten() int64 {
	n, oldest := 0, int64(0)
	for _, c := range l.conflicts {
		if c.Resolved != nil {
			if n == 0 || c.Rev < oldest {
				oldest = c.Rev
			}
			n++
		}
	}
	if n < KeepResolved {
		return 0
	}
	return oldest - 1
}

// ledgerFile is the ledger as JSON.
type ledgerFile struct {
	Version   int        `json:"version"`
	Epoch     string     `json:"epoch"`
	Rev       int64      `json:"rev"`
	Records   []Record   `json:"records"`
	Conflicts []Conflict `json:"conflicts,omitempty"`
}

// MarshalJSON writes the ledger as its file holds it.
func (l *Ledger) MarshalJSON() ([]byte, error) {
	f := ledgerFile{Version: LedgerVersion, Epoch: l.epoch, Rev: l.rev, Records: make([]Record, 0, len(l.records)), Conflicts: l.conflicts}
	for _, r := range l.records {
		f.Records = append(f.Records, r)
	}
	slices.SortFunc(f.Records, byRev)
	return json.Marshal(f)
}

// UnmarshalJSON reads a ledger MarshalJSON wrote; one of a later version is ErrLedgerNewer.
func (l *Ledger) UnmarshalJSON(data []byte) error {
	var f ledgerFile
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}
	if f.Version > LedgerVersion {
		return ErrLedgerNewer
	}
	if f.Epoch == "" {
		return errors.New("the learning state names no epoch")
	}
	*l = Ledger{epoch: f.Epoch, rev: f.Rev, records: make(map[recordKey]Record, len(f.Records)), conflicts: f.Conflicts}
	for _, r := range f.Records {
		if r.Kind == "" || r.ID == "" {
			continue
		}
		l.records[recordKey{r.Kind, r.ID}] = r
		l.rev = max(l.rev, r.Rev)
	}
	for _, c := range l.conflicts {
		l.rev = max(l.rev, c.Rev)
	}
	return nil
}

// ReadLedger reads the ledger kept at path; a missing file is a new ledger. A file that cannot be read is an error, so the caller does not write a new one over it.
func ReadLedger(path string) (*Ledger, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return NewLedger(), nil
	}
	if err != nil {
		return nil, err
	}
	l := &Ledger{}
	if err := json.Unmarshal(data, l); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return l, nil
}

// WriteLedger keeps l at path, all of it or nothing.
func WriteLedger(path string, l *Ledger) error {
	data, err := json.Marshal(l)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
