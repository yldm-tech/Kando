package session

import (
	"cmp"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"time"

	"kando/apps/mobile/study"
	"kando/packages/sync"
)

// The learning state, the settings and the subscriptions through a computer's storage relay (relay.go; yldm-tech/Kando#219). Away from the computer's network, the calls a sync of the learning state, a resolution of a conflict, a change of the computer's settings or an action on a subscription makes on the network go into the computer's storage instead: the same calls, which the computer answers there the same way, with the same read-only refusals (sync.Mutates), within a minute or two while it runs. A library's sync leaves its puts and its read in the storage at once, in that order, and ends once the computer answered them all.
//
// The phone follows what changes on the computer by the state.json the computer writes there, which tells the revision each library's learning state last changed at (sync.RelayState.Learning). At each turn of the follower, a library that changed since the phone last synced it, or that holds changes of the phone's the computer has not taken, syncs; one whose sync failed waits relayRetry before the follower tries it again, so a computer that keeps refusing does not use up the storage's slots. The language translated into goes the same way, and the computer's settings are read there once a run.

// relayRetry is how long the follower waits before it syncs again through the storage what failed there, unless the user asks (SyncLearning). It is paced by the wall clock, as the follower's turns are, not by the session's clock.
const relayRetry = 5 * time.Minute

// errAway is the error of a change that must reach the computer now, made while its storage says it has not run lately.
var errAway = errors.New("the computer has not written to its storage lately")

// relayStudy is what the phone follows of a computer's learning state and settings through its storage: what its state.json said last (hello, and by library the revision of epoch its learning state last changed at), the revisions the phone synced each library to since, and when each library's sync there last failed. Engine goroutine only.
type relayStudy struct {
	hello   sync.Hello
	epoch   string
	changed map[string]int64
	seen    map[string]int64
	failed  map[string]time.Time
	// settings is set while a change or a read of the computer's settings waits in the storage, and settingsFailed is when the last one failed.
	settings       bool
	settingsFailed time.Time
	// listed is set once the computer's subscriptions were read through the storage since the phone began following it there.
	listed bool
}

// relayed reports whether desktop is followed through its storage, away from its network: its learning state, settings and subscriptions go there too.
func (s *Session) relayed(desktop string) bool {
	return s.remote.queues.GetUntracked()[desktop].Relayed
}

// relayFollowStudy takes in what state, desktop's state.json, says of its learning state. Engine goroutine only.
func (s *Session) relayFollowStudy(desktop string, state sync.RelayState) {
	rs := s.remote.relayStudies[desktop]
	if rs == nil || rs.epoch != state.Jobs.Epoch {
		// A computer that started again counts the revisions afresh.
		rs = &relayStudy{epoch: state.Jobs.Epoch, seen: map[string]int64{}, failed: map[string]time.Time{}}
		s.remote.relayStudies[desktop] = rs
	}
	rs.hello, rs.changed = state.Hello, state.Learning
	s.learn.watchedKind[desktop] = slices.Contains(state.Hello.State, sync.KindWatched) && !state.Hello.ReadOnly
}

// relayCatchUp brings what the phone keeps of desktop up to date through its storage at a turn of the follower: its subscriptions once the phone follows it there, its learning state where it changed or the phone has changes for it, and its settings. What only reads is left there while the computer writes there, so a computer that is off does not find a pile of reads once back, while the phone's changes wait there for it. Engine goroutine only.
func (s *Session) relayCatchUp(desktop string) {
	rs := s.remote.relayStudies[desktop]
	if rs == nil {
		return
	}
	if !rs.listed && s.remote.queues.GetUntracked()[desktop].Reach == Online {
		rs.listed = true
		s.readSubscriptions(desktop)
	}
	s.relayLearn(desktop, false)
	s.relaySettings(desktop, false)
}

// relayLearn syncs through desktop's storage each library of it that changed there since the phone synced it, while the computer writes there, and each holding changes of the phone's the computer has not taken, unless the phone may not change it: but for those whose sync is under way, and those whose last failed less than relayRetry ago unless retry is set. Engine goroutine only.
func (s *Session) relayLearn(desktop string, retry bool) {
	rs := s.remote.relayStudies[desktop]
	if rs == nil || !(slices.Contains(rs.hello.State, sync.KindLine) || slices.Contains(rs.hello.State, sync.KindWord)) {
		return
	}
	q := s.remote.queues.GetUntracked()[desktop]
	canPut, fresh := q.Can(sync.OpStatePut), q.Reach == Online
	studies := s.studies.GetUntracked()
	libs := map[string]bool{}
	for lib := range rs.changed {
		libs[lib] = true
	}
	for k := range studies {
		if d, lib := splitStudyKey(k); d == desktop {
			libs[lib] = true
		}
	}
	now := time.Now()
	for _, lib := range slices.Sorted(maps.Keys(libs)) {
		k := studyKey(desktop, lib)
		if _, running := s.studyRuns[k]; running || s.studyLoading[k] {
			continue
		}
		l, kept := studies[k]
		rev, listed := rs.changed[lib]
		due := (fresh && listed && rev > rs.seen[lib]) || (kept && canPut && len(l.Pending) > 0)
		if failed, ok := rs.failed[lib]; !due || (ok && !retry && now.Sub(failed) < relayRetry) {
			continue
		}
		s.relaySyncLibrary(desktop, lib, rs, rev)
	}
}

// relaySyncLibrary syncs library lib of desktop through its storage, one the phone keeps nothing of yet starting empty, and notes in rs, once it is done, that the library is synced up to rev, or when it failed. A library the computer no longer serves, as one locked, goes as on its network (learnLibrarySynced). Engine goroutine only.
func (s *Session) relaySyncLibrary(desktop, lib string, rs *relayStudy, rev int64) {
	k := studyKey(desktop, lib)
	if _, ok := s.studies.GetUntracked()[k]; !ok {
		m := maps.Clone(s.studies.GetUntracked())
		m[k] = study.Library{}
		s.studies.Set(m)
	}
	s.syncStudyThen(desktop, lib, func(err error) {
		switch {
		case err == nil:
			delete(rs.failed, lib)
			rs.seen[lib] = max(rs.seen[lib], rev)
			return
		case !errors.Is(err, errRelayLost):
			// A call lost in the storage goes again at the next turn; one the computer refused waits.
			rs.failed[lib] = time.Now()
		}
		if ProblemOf(err) == Gone {
			if l, ok := s.studies.GetUntracked()[k]; ok && !holdsChanges(l) {
				s.dropStudy(desktop, lib)
			}
		}
	})
}

// relayStudyRun syncs library lib of desktop, l as the phone has it, through its storage: it leaves there the puts of sent, the phone's changes, in batches the computer takes, unless the computer made the phone read-only, then the read of the computer's changes since l's revision, and tells done, on the engine's goroutine once the computer answered them all, the puts it answered, its changes, nil when it did not tell them, and the first error. A batch the computer refuses (sync.CodeInvalid) goes again a change at a time, as on its network (putStudy). Engine goroutine only.
func (s *Session) relayStudyRun(desktop, lib string, l study.Library, sent []sync.Record, done func([]studyPut, *sync.StateChanges, error)) {
	var batches [][]sync.Record
	if s.remote.queues.GetUntracked()[desktop].Can(sync.OpStatePut) {
		batches = study.Batches(sent, putBudget)
	}
	calls := make([]sync.Call, 0, len(batches)+1)
	for _, b := range batches {
		calls = append(calls, relayCallOf(sync.NewCallID(), sync.OpStatePut, sync.StatePut{Library: lib, Records: b}))
	}
	get := relayCallOf(sync.NewCallID(), sync.OpStateGet, sync.StateGet{Library: lib, Epoch: l.Epoch, Since: l.Rev})
	calls = append(calls, get)
	s.relayAsk(desktop, calls, func(answers map[string]relayAnswer, err error) {
		puts, singles, perr := s.relayPuts(desktop, batches, calls, answers)
		err = cmp.Or(err, perr)
		var ch *sync.StateChanges
		if _, sent := answers[get.ID]; sent {
			got, gerr := relayResult[sync.StateChanges](answers, get.ID, nil)
			if gerr == nil {
				ch = &got
			}
			err = cmp.Or(err, gerr)
		}
		if len(singles) == 0 {
			done(puts, ch, err)
			return
		}
		again := make([]sync.Call, len(singles))
		for i, b := range singles {
			again[i] = relayCallOf(sync.NewCallID(), sync.OpStatePut, sync.StatePut{Library: lib, Records: b})
		}
		s.relayAsk(desktop, again, func(answers map[string]relayAnswer, aerr error) {
			more, _, perr := s.relayPuts(desktop, singles, again, answers)
			done(append(puts, more...), ch, cmp.Or(err, aerr, perr))
		})
	})
}

// relayPuts reads what desktop answered the puts of batches, left in its storage as calls: the puts it took, or refused for good when they held a change alone; the batches of several changes it refused, cut into one change each, to put again; and the first error of a put it did not answer. A put refused as the phone is read-only takes the phone as such (noteReadOnly), and its changes stay for later. Engine goroutine only.
func (s *Session) relayPuts(desktop string, batches [][]sync.Record, calls []sync.Call, answers map[string]relayAnswer) (puts []studyPut, singles [][]sync.Record, err error) {
	for i, b := range batches {
		if _, sent := answers[calls[i].ID]; !sent {
			continue
		}
		res, perr := relayResult[sync.StatePutResult](answers, calls[i].ID, nil)
		var e *sync.Error
		switch {
		case perr == nil:
			puts = append(puts, studyPut{sent: b, res: res})
		case errors.As(perr, &e) && e.Code == sync.CodeInvalid && len(b) == 1:
			puts = append(puts, studyPut{sent: b, rejected: true})
		case errors.As(perr, &e) && e.Code == sync.CodeInvalid:
			for _, r := range b {
				singles = append(singles, []sync.Record{r})
			}
		case errors.As(perr, &e) && e.Code == sync.CodeReadOnly:
			s.noteReadOnly(desktop, perr)
		default:
			err = cmp.Or(err, perr)
		}
	}
	return puts, singles, err
}

// relaySettings leaves in desktop's storage the phone's language to translate into when the computer has not taken it yet and lets the phone change it, else a read of the computer's settings when the phone has not had them this run and the computer writes there, and takes what the computer answers in as a sync on its network does (learnAsked): one at a time, and after one failed, not before relayRetry unless retry is set. Engine goroutine only.
func (s *Session) relaySettings(desktop string, retry bool) {
	rs := s.remote.relayStudies[desktop]
	if rs == nil || rs.settings || (!retry && time.Since(rs.settingsFailed) < relayRetry) {
		return
	}
	h := rs.hello
	_, known := s.learn.settings.GetUntracked()[desktop]
	fresh := s.remote.queues.GetUntracked()[desktop].Reach == Online
	put := slices.Contains(s.prefs.GetUntracked().TargetPending, desktop) && !h.ReadOnly && slices.Contains(h.Ops, sync.OpSettingsPut)
	var call sync.Call
	switch {
	case put:
		call = relayCallOf(sync.NewCallID(), sync.OpSettingsPut, sync.SettingsPut{Target: s.Target()})
	case !known && fresh && slices.Contains(h.Ops, sync.OpSettingsGet):
		call = relayCallOf(sync.NewCallID(), sync.OpSettingsGet, nil)
	default:
		return
	}
	rs.settings = true
	asked := s.learn.settingsPuts[desktop]
	s.relayAsk(desktop, []sync.Call{call}, func(answers map[string]relayAnswer, err error) {
		rs.settings = false
		got, err := relayResult[sync.Settings](answers, call.ID, err)
		if _, paired := s.Desktop(desktop); !paired {
			return
		}
		if err != nil {
			if !errors.Is(err, errRelayLost) {
				rs.settingsFailed = time.Now()
			}
			return
		}
		rs.settingsFailed = time.Time{}
		targetWas := s.prefs.Get().Target
		s.learnTakeSettings(desktop, got, put)
		// A change of the computer's settings begun since this was asked keeps what it answered: these may be from before it, as learnAsked takes them.
		if s.learn.settingsPuts[desktop] == asked {
			s.setDesktopSettings(desktop, DesktopSettings{Settings: got, ReadOnly: h.ReadOnly})
		}
		if put && slices.Contains(s.prefs.Get().TargetPending, desktop) {
			// The computer answered with another language than the phone put: it is put again after a while, not at every turn.
			rs.settingsFailed = time.Now()
		}
		// A language taken from the computer goes to the others at once.
		if p := s.prefs.Get(); p.Target != targetWas && len(p.TargetPending) > 0 {
			s.SyncLearning()
		}
	})
}

// relayResult is the result of call id among answers, decoded into a T, or why there is none: the computer's error, errRelayLost for a reply that will not come, or err, why the call did not go.
func relayResult[T any](answers map[string]relayAnswer, id string, err error) (T, error) {
	var out T
	a, ok := answers[id]
	switch {
	case !ok:
		return out, cmp.Or(err, errRelayLost)
	case a.err != nil:
		return out, a.err
	}
	if err := a.reply.Err(); err != nil {
		return out, err
	}
	if len(a.reply.Result) > 0 {
		if err := json.Unmarshal(a.reply.Result, &out); err != nil {
			return out, err
		}
	}
	return out, nil
}

// relayCallResult leaves a call of op with args in desktop's storage and tells done, on the engine's goroutine, its result, or why there is none, as relayResult reads it. Engine goroutine only.
func relayCallResult[T any](s *Session, desktop, op string, args any, done func(T, error)) {
	call := relayCallOf(sync.NewCallID(), op, args)
	s.relayAsk(desktop, []sync.Call{call}, func(answers map[string]relayAnswer, err error) {
		done(relayResult[T](answers, call.ID, err))
	})
}
