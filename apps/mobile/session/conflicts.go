package session

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"

	"kando/apps/mobile/store"
	"kando/apps/mobile/study"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
)

// The sync conflicts of the learning state (yldm-tech/Kando#217): a translation, note or meaning changed on two devices apart, of which the computer kept the later and set the other aside (sync.Conflict). The phone lists the open ones of every library it keeps the learning state of, as the computer last answered them, and resolves one through the computer with the version the learner keeps, one of the two or both, which every device then takes.

// SyncConflict is an open conflict as Settings › Sync Conflicts shows it: the computer and library it is of, by their IDs, and the computer's name, besides the conflict, whose Title is its film's title wherever the phone knows it.
type SyncConflict struct {
	Desktop, Library string
	Computer         string
	study.Conflict
}

// Key names c among the conflicts of every library.
func (c SyncConflict) Key() string { return studyKey(c.Desktop, c.Library) + "/" + c.ID }

// Keep is which version of a conflict ResolveConflict keeps.
type Keep uint8

const (
	// KeepKept keeps the version the sync kept.
	KeepKept Keep = iota
	// KeepLost keeps the version it set aside.
	KeepLost
	// KeepBoth keeps both, the kept one first (sync.Joined), where they can be (study.Conflict.Both).
	KeepBoth
)

// ResolveFailure is why the last resolution failed: the computer's name, and whether it lets the phone only read, else Problem; the zero value for none.
type ResolveFailure struct {
	Computer string
	ReadOnly bool
	Problem  Problem
}

// settle is the session's part of the conflicts: those being resolved, by SyncConflict.Key, and why the last resolution failed.
type settle struct {
	resolving *core.Signal[map[string]bool]
	failure   *core.Signal[ResolveFailure]
}

// newSettle is the session's conflicts, none being resolved.
func newSettle(cx *core.Scope) *settle {
	return &settle{resolving: core.NewSignalFunc(cx, map[string]bool{}, nil), failure: core.NewSignal(cx, ResolveFailure{})}
}

// Conflicts are the open conflicts of every library of the paired computers, the oldest first, by computer in the order they were paired. Read it in a binding.
func (s *Session) Conflicts() []SyncConflict {
	studies := s.studies.Get()
	var out []SyncConflict
	for _, d := range s.Desktops() {
		var mine []SyncConflict
		for k, l := range studies {
			desktop, lib := splitStudyKey(k)
			if desktop != d.ID {
				continue
			}
			for _, c := range study.Conflicts(l, d.DeviceID) {
				if c.Title == "" && c.Film != "" {
					c.Title = s.filmTitle(d.ID, lib, c.Film)
				}
				mine = append(mine, SyncConflict{Desktop: d.ID, Library: lib, Computer: d.Name, Conflict: c})
			}
		}
		slices.SortStableFunc(mine, func(a, b SyncConflict) int {
			return cmp.Or(cmp.Compare(a.Rev, b.Rev), strings.Compare(a.Key(), b.Key()))
		})
		out = append(out, mine...)
	}
	return out
}

// filmTitle is the title of film id of library lib of desktop, as the phone keeps it or the computer last listed it, "" when it knows neither.
func (s *Session) filmTitle(desktop, lib, id string) string {
	if f, ok := s.Film(store.Key(desktop, id)); ok {
		return f.Film.Title
	}
	for _, l := range s.Shelf(desktop).Listing.Libraries {
		if l.ID != lib {
			continue
		}
		if i := slices.IndexFunc(l.Films, func(f sync.Film) bool { return f.ID == id }); i >= 0 {
			return l.Films[i].Title
		}
	}
	return ""
}

// Resolving reports whether c is being resolved. Read it in a binding.
func (s *Session) Resolving(c SyncConflict) bool { return s.settle.resolving.Get()[c.Key()] }

// ResolveFailed is why the last resolution failed, the zero value when it did not. Read it in a binding.
func (s *Session) ResolveFailed() ResolveFailure { return s.settle.failure.Get() }

// ResolveConflict resolves c through its computer with the version keep names, which then becomes the value on every device; c goes from the list once the computer took it. One the computer no longer has open, settled on another device meanwhile, goes too, and the library syncs to take the value kept in. A computer out of reach, or that lets the phone only read, leaves c open, and says why (ResolveFailed). Away from the computer's network the resolution goes through its storage (relay_study.go), and c shows as being resolved until the computer answered there.
func (s *Session) ResolveConflict(c SyncConflict, keep Keep) {
	k := studyKey(c.Desktop, c.Library)
	if s.settle.resolving.GetUntracked()[c.Key()] {
		return
	}
	l, ok := s.studies.GetUntracked()[k]
	if !ok {
		return
	}
	i := slices.IndexFunc(l.Conflicts, func(x sync.Conflict) bool { return x.ID == c.ID })
	if i < 0 {
		return
	}
	value := resolution(l.Conflicts[i], keep)
	cl := s.Client(c.Desktop)
	if cl == nil {
		s.settle.failure.Set(ResolveFailure{Computer: c.Computer, Problem: Unpaired})
		return
	}
	s.settle.failure.Set(ResolveFailure{})
	s.setResolving(c.Key(), true)
	answered := func(res sync.StatePutResult, err error) {
		s.setResolving(c.Key(), false)
		cur, kept := s.studies.GetUntracked()[k]
		var e *sync.Error
		switch {
		case err == nil:
			if kept {
				s.setStudy(c.Desktop, c.Library, study.Resolved(cur, res))
			}
		case errors.As(err, &e) && e.Code == sync.CodeNotFound:
			if kept {
				s.setStudy(c.Desktop, c.Library, study.Forget(cur, c.ID))
			}
		case errors.As(err, &e) && e.Code == sync.CodeReadOnly:
			s.settle.failure.Set(ResolveFailure{Computer: c.Computer, ReadOnly: true})
			return
		default:
			s.settle.failure.Set(ResolveFailure{Computer: c.Computer, Problem: ProblemOf(err)})
			return
		}
		if kept {
			s.syncStudy(c.Desktop, c.Library)
		}
	}
	req := sync.ConflictResolve{Library: c.Library, ID: c.ID, Value: value}
	if s.relayed(c.Desktop) {
		// Away from the computer's network, through its storage: the conflict shows as being resolved until the computer answered there.
		relayCallResult(s, c.Desktop, sync.OpStateResolve, req, answered)
		return
	}
	s.work.Go(func() {
		ctx, cancel := context.WithTimeout(s.ctx, studyTimeout)
		defer cancel()
		res, err := cl.ResolveConflict(ctx, req.Library, req.ID, req.Value)
		s.cx.Post(func() { answered(res, err) })
	})
}

// resolution is the value that keeps the version of c keep names.
func resolution(c sync.Conflict, keep Keep) json.RawMessage {
	switch keep {
	case KeepLost:
		return c.Lost.Value
	case KeepBoth:
		if sync.Joinable(c.Kind, c.Field) {
			return sync.Joined(c)
		}
	}
	return c.Kept.Value
}

// setResolving marks the conflict under key as being resolved, or no longer.
func (s *Session) setResolving(key string, on bool) {
	m := maps.Clone(s.settle.resolving.GetUntracked())
	if on {
		m[key] = true
	} else {
		delete(m, key)
	}
	s.settle.resolving.Set(m)
}

// OpenConflicts is told Settings › Sync Conflicts opens: the failure of a resolution before goes, and the learning state syncs with every computer, so the page lists the conflicts as they are now.
func (s *Session) OpenConflicts() {
	s.settle.failure.Set(ResolveFailure{})
	s.SyncLearning()
}
