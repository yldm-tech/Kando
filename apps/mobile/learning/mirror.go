// Package learning is what the phone keeps of the learner's study, with nothing of the interface in it: each paired computer's libraries' learning state as the computer last told it (protocol v2's records, kando/packages/sync, state.go), with the changes made on the phone that it has not taken yet, so the Learn tab works away from the computer; the words and lines to review in it and their schedule; how long films played on the phone each day; the export of the vocabulary and the saved lines; and when to remind the learner to review. The session reads and writes it off the engine's goroutine.
package learning

import (
	"slices"

	"kando/packages/sync"
)

// Mirror is a library's learning state on the phone: the records the computer last told it, after revision Rev of Epoch, with the phone's changes laid over them, and those changes (Pending) until the computer takes them.
type Mirror struct {
	// Desktop is the paired computer's ID on the phone (store.Desktop.ID), Library the library's ID on it and Name its name.
	Desktop string `json:"desktop"`
	Library string `json:"library"`
	Name    string `json:"name"`
	Epoch   string `json:"epoch"`
	Rev     int64  `json:"rev"`
	// Records are the library's records as the phone shows them, tombstones among them.
	Records []sync.Record `json:"records"`
	// Pending are the phone's changes the computer has not taken yet, each field with the stamp of the value it was changed from (sync.Field.Base).
	Pending []sync.Record `json:"pending,omitempty"`
	// Devices names the devices the stamps name, by their IDs, as the computer last told them.
	Devices map[string]string `json:"devices,omitempty"`
}

// Key names a mirror on the phone.
func Key(desktop, library string) string { return desktop + "/" + library }

// Key is the mirror's.
func (m Mirror) Key() string { return Key(m.Desktop, m.Library) }

// index is the index of record kind id among records, or -1.
func index(records []sync.Record, kind, id string) int {
	return slices.IndexFunc(records, func(r sync.Record) bool { return r.Kind == kind && r.ID == id })
}

// Take takes the computer's changes in: with Reset they replace the records, else each takes the place of the record of its kind and ID. The phone's pending changes stay laid over them, as the computer has not taken them yet.
func (m *Mirror) Take(ch sync.StateChanges) {
	if ch.Reset || ch.Epoch != m.Epoch {
		m.Records = nil
	}
	for _, r := range ch.Records {
		m.set(r)
	}
	m.Epoch, m.Rev = ch.Epoch, ch.Rev
	if len(ch.Devices) > 0 {
		m.Devices = ch.Devices
	}
	m.overlay()
}

// Change makes r, a change made on the phone, part of the records and of what is still to be put: each field of it is stamped with its Base, the stamp of the value it changes, so the computer sees whether its own changed meanwhile.
func (m *Mirror) Change(r sync.Record) {
	if i := index(m.Pending, r.Kind, r.ID); i >= 0 {
		p := &m.Pending[i]
		if r.Removed != nil {
			p.Removed, p.Fields = r.Removed, nil
		} else if p.Removed == nil {
			for name, f := range r.Fields {
				// A field changed twice before the computer took it was changed from what the computer had, not from the phone's own first change.
				if had, ok := p.Fields[name]; ok {
					f.Base = had.Base
				} else {
					f.Base = m.base(r.Kind, r.ID, name)
				}
				p.Fields[name] = f
			}
		}
	} else {
		p := sync.Record{Kind: r.Kind, ID: r.ID, Film: r.Film, Removed: r.Removed}
		if r.Removed == nil {
			p.Fields = map[string]sync.Field{}
			for name, f := range r.Fields {
				f.Base = m.base(r.Kind, r.ID, name)
				p.Fields[name] = f
			}
		}
		m.Pending = append(m.Pending, p)
	}
	m.overlay()
}

// base is the stamp of field name of record kind id as the computer last told it, nil for none.
func (m *Mirror) base(kind, id, name string) *sync.Stamp {
	i := index(m.Records, kind, id)
	if i < 0 {
		return nil
	}
	f, ok := m.Records[i].Fields[name]
	if !ok || f.Stamp.IsZero() {
		return nil
	}
	s := f.Stamp
	return &s
}

// Settle takes the computer's answer to a put of sent, the pending changes as they were sent: the records as merged take the place of the phone's, and the changes sent leave Pending, but for those changed again since.
func (m *Mirror) Settle(sent []sync.Record, res sync.StatePutResult) {
	for _, s := range sent {
		i := index(m.Pending, s.Kind, s.ID)
		if i >= 0 && samePending(m.Pending[i], s) {
			m.Pending = slices.Delete(m.Pending, i, i+1)
		}
	}
	for _, r := range res.Records {
		m.set(r)
	}
	// Rev is not moved: changes other devices made before res.Rev are still to be asked for.
	m.overlay()
}

// set puts r in place of the record of its kind and ID, or adds it.
func (m *Mirror) set(r sync.Record) {
	if i := index(m.Records, r.Kind, r.ID); i >= 0 {
		m.Records[i] = r
		return
	}
	m.Records = append(m.Records, r)
}

// overlay lays the pending changes over the records, as the computer will merge them.
func (m *Mirror) overlay() {
	for _, p := range m.Pending {
		var have sync.Record
		i := index(m.Records, p.Kind, p.ID)
		if i >= 0 {
			have = m.Records[i]
		}
		out, changed, _ := sync.MergeRecord(have, p)
		if !changed {
			continue
		}
		if i >= 0 {
			m.Records[i] = out
		} else {
			m.Records = append(m.Records, out)
		}
	}
}

// samePending reports whether a and b are the same change.
func samePending(a, b sync.Record) bool {
	if (a.Removed == nil) != (b.Removed == nil) || len(a.Fields) != len(b.Fields) {
		return false
	}
	if a.Removed != nil {
		return a.Removed.Equal(*b.Removed)
	}
	for name, f := range a.Fields {
		g, ok := b.Fields[name]
		if !ok || !f.Stamp.Equal(g.Stamp) || !sync.SameValue(f.Value, g.Value) {
			return false
		}
	}
	return true
}
