package session

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	gosync "sync"
	"time"

	"kando/apps/mobile/langs"
	"kando/apps/mobile/learning"
	"kando/apps/mobile/locales"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
)

// learn is the session's part of the learner's study (package learning): the paired computers' libraries' learning state as the phone keeps it, how long films played on the phone, whether the phone is bringing them up to date with the computers and why it last could not, and the computers that no longer take the phone.
type learn struct {
	disk learning.Disk
	// ready turns true once what the phone kept is read.
	ready   *core.Signal[bool]
	mirrors *core.Signal[[]learning.Mirror]
	watched *core.Signal[learning.Watched]
	syncing *core.Signal[bool]
	problem *core.Signal[Problem]
	// revoked are the computers that answered that they no longer take the phone, until the user saw it (DismissRevoked).
	revoked   *core.Signal[[]string]
	dismissed map[string]bool
	// again is set when a sync was asked for while one ran, which runs once it is done, so a change made meanwhile is not left until the next.
	again bool
	// saveMu orders the saves of each computer's mirrors and of the time watched, as prefsMu does the preferences': saving is the generation last given out, saved the one last written, by computer ("" for the time watched).
	saveMu        gosync.Mutex
	saving, saved map[string]uint64
}

// learnSyncFor is how long a sync waits for a computer, which may be off or out of reach.
const learnSyncFor = 20 * time.Second

// reminderID names the review reminder among the app's notifications.
const reminderID = "learn.review"

// newLearn is the session's study, read from disk off the engine's goroutine.
func newLearn(s *Session) *learn {
	l := &learn{disk: learning.Open(s.cfg.Dir),
		ready:     core.NewSignal(s.cx, false),
		mirrors:   core.NewSignalFunc[[]learning.Mirror](s.cx, nil, nil),
		watched:   core.NewSignalFunc(s.cx, learning.Watched{}, nil),
		syncing:   core.NewSignal(s.cx, false),
		problem:   core.NewSignal(s.cx, NoProblem),
		revoked:   core.NewSignalFunc[[]string](s.cx, nil, nil),
		dismissed: map[string]bool{},
		saving:    map[string]uint64{},
		saved:     map[string]uint64{},
	}
	s.work.Go(func() {
		mirrors, _ := l.disk.Mirrors()
		watched, _ := l.disk.Watched()
		s.cx.Post(func() {
			l.mirrors.Set(mirrors)
			l.watched.Set(watched)
			l.ready.Set(true)
		})
	})
	// The phone catches up with the computers once what it kept is read, each time the app shows again after it was in the background, and as a computer is paired.
	shown := core.NewMemo(s.cx, func() bool { return s.cx.Env().Lifecycle() != core.LifecycleBackground })
	s.cx.Effect(func() {
		if !l.ready.Get() || !s.loaded.Get() || !shown.Get() {
			return
		}
		_ = len(s.desktops.Get())
		s.cx.Post(func() {
			s.SyncLearning()
			s.scheduleReminder()
		})
	})
	return l
}

// Mirrors are the libraries' learning state as the phone keeps it.
func (s *Session) Mirrors() []learning.Mirror { return s.learn.mirrors.Get() }

// LearnItems are the words and saved lines of every library the phone keeps the learning state of, the latest saved first.
func (s *Session) LearnItems() []learning.Item {
	var out []learning.Item
	for _, m := range s.learn.mirrors.Get() {
		out = append(out, m.Items()...)
	}
	slices.SortStableFunc(out, func(a, b learning.Item) int { return b.SavedAt.Compare(a.SavedAt) })
	return out
}

// Now is the session's clock.
func (s *Session) Now() time.Time { return s.cfg.Now() }

// Watched is how long films played on the phone each day.
func (s *Session) Watched() learning.Watched { return s.learn.watched.Get() }

// Watch adds d, time a film played on the phone, to today's.
func (s *Session) Watch(d time.Duration) {
	if d <= 0 {
		return
	}
	w := maps.Clone(s.learn.watched.Get())
	if w == nil {
		w = learning.Watched{}
	}
	w.Add(s.cfg.Now(), d)
	s.learn.watched.Set(w)
	s.learnSave("", func() error { return s.learn.disk.SaveWatched(w) })
}

// Goal is the daily goal in minutes of films played.
func (s *Session) Goal() int {
	if g := s.prefs.Get().Goal; g > 0 {
		return g
	}
	return learning.DefaultGoal
}

// LearnSyncing reports whether the phone is bringing the learning state up to date with the computers.
func (s *Session) LearnSyncing() bool { return s.learn.syncing.Get() }

// LearnProblem is why the last sync reached no computer, NoProblem when one answered or none is paired.
func (s *Session) LearnProblem() Problem { return s.learn.problem.Get() }

// Revoked are the paired computers that no longer take the phone, which the user has not seen yet.
func (s *Session) Revoked() []string { return s.learn.revoked.Get() }

// DismissRevoked takes computer id off Revoked for this run: the user saw it.
func (s *Session) DismissRevoked(id string) {
	s.learn.dismissed[id] = true
	s.learn.revoked.Set(slices.DeleteFunc(slices.Clone(s.learn.revoked.Get()), func(x string) bool { return x == id }))
}

// PendingChanges is how many of the phone's changes of computer id's learning state it has not taken yet.
func (s *Session) PendingChanges(id string) int {
	n := 0
	for _, m := range s.learn.mirrors.Get() {
		if m.Desktop == id {
			n += len(m.Pending)
		}
	}
	return n
}

// Grade schedules it after the learner's review now: good when they knew it. The computer takes the change at the next sync.
func (s *Session) Grade(it learning.Item, good bool) {
	s.learnChange(it, func(device string) sync.Record { return learning.Grade(it, good, s.cfg.Now(), device) })
}

// RemoveItem removes it from the vocabulary or the saved lines, on every device.
func (s *Session) RemoveItem(it learning.Item) {
	s.learnChange(it, func(device string) sync.Record { return learning.Removal(it, s.cfg.Now(), device) })
	s.SyncLearning()
}

// learnChange makes the change change gives, stamped with the phone's ID on the item's computer, in the item's mirror, and keeps it.
func (s *Session) learnChange(it learning.Item, change func(device string) sync.Record) {
	d, ok := s.Desktop(it.Desktop)
	if !ok {
		return
	}
	list := slices.Clone(s.learn.mirrors.Get())
	i := slices.IndexFunc(list, func(m learning.Mirror) bool { return m.Desktop == it.Desktop && m.Library == it.Library })
	if i < 0 {
		return
	}
	m := cloneMirror(list[i])
	m.Change(change(d.DeviceID))
	list[i] = m
	s.learn.mirrors.Set(list)
	s.learnSaveDesktop(it.Desktop)
	s.scheduleReminder()
}

// learnLib is what a sync did with a library of a computer: the changes it put (sent) and the answer, then the changes it asked for after them.
type learnLib struct {
	lib     sync.Library
	sent    []sync.Record
	put     sync.StatePutResult
	putErr  error
	changes sync.StateChanges
	getErr  error
}

// learnResult is what a sync did with a computer: the libraries it lists, or why it could not be asked, and the language it translates into when it shares its settings.
type learnResult struct {
	desktop  string
	err      error
	state    bool
	libs     []learnLib
	settings *sync.Settings
	// targetPut is set when the phone's language was put to the computer.
	targetPut bool
}

// SyncLearning brings the learning state up to date with every paired computer that serves it: the phone's changes put first, then the computer's asked for; and the language translated into, the phone's put to a computer that has not taken it yet, else the computer's taken. A computer out of reach is left as it was.
func (s *Session) SyncLearning() {
	if s.learn.syncing.GetUntracked() {
		s.learn.again = true
		return
	}
	desktops := s.Desktops()
	if len(desktops) == 0 {
		s.learn.problem.Set(NoProblem)
		return
	}
	s.learn.syncing.Set(true)
	mirrors := s.learn.mirrors.Get()
	prefs := s.prefs.Get()
	target := s.Target()
	results := make([]learnResult, len(desktops))
	var wg gosync.WaitGroup
	for i, d := range desktops {
		c := s.Client(d.ID)
		mine := map[string]learning.Mirror{}
		for _, m := range mirrors {
			if m.Desktop == d.ID {
				mine[m.Library] = cloneMirror(m)
			}
		}
		wg.Add(1)
		s.work.Go(func() {
			defer wg.Done()
			results[i] = learnSync(s.closed, c, d.ID, mine, prefs.TargetPending, target)
		})
	}
	s.work.Go(func() {
		wg.Wait()
		s.cx.Post(func() { s.learnSynced(results) })
	})
}

// learnSync does a sync with computer desktop through c, off the engine's goroutine, given up as closed closes, as the session ends: mirrors are the phone's of its libraries, by library ID.
func learnSync(closed <-chan struct{}, c *sync.Client, desktop string, mirrors map[string]learning.Mirror, putTarget bool, target string) learnResult {
	out := learnResult{desktop: desktop}
	if c == nil {
		out.err = &sync.Error{Status: 401}
		return out
	}
	ctx, cancel := context.WithTimeout(context.Background(), learnSyncFor)
	defer cancel()
	go func() {
		select {
		case <-closed:
			cancel()
		case <-ctx.Done():
		}
	}()
	h, err := c.Hello(ctx)
	if errors.Is(err, sync.ErrV1Only) {
		return out
	}
	if err != nil {
		out.err = err
		return out
	}
	if slices.Contains(h.Ops, sync.OpSettingsPut) && putTarget {
		got, err := c.PutSettings(ctx, sync.SettingsPut{Target: target})
		if err == nil {
			out.settings, out.targetPut = &got, true
		}
	} else if slices.Contains(h.Ops, sync.OpSettingsGet) {
		if got, err := c.Settings(ctx); err == nil {
			out.settings = &got
		}
	}
	out.state = slices.Contains(h.State, sync.KindLine) || slices.Contains(h.State, sync.KindWord)
	if !out.state {
		return out
	}
	libs, err := c.Libraries(ctx)
	if err != nil {
		out.err = err
		return out
	}
	for _, lib := range libs {
		r := learnLib{lib: lib}
		m := mirrors[lib.ID]
		if len(m.Pending) > 0 {
			r.sent = m.Pending
			r.put, r.putErr = c.PutState(ctx, lib.ID, m.Pending)
		}
		r.changes, r.getErr = c.State(ctx, lib.ID, m.Epoch, m.Rev)
		out.libs = append(out.libs, r)
	}
	return out
}

// learnSynced takes the results of a sync in, on the engine's goroutine.
func (s *Session) learnSynced(results []learnResult) {
	s.learn.syncing.Set(false)
	reached, problem := false, NoProblem
	list := slices.Clone(s.learn.mirrors.Get())
	revoked := slices.Clone(s.learn.revoked.Get())
	for _, r := range results {
		if _, ok := s.Desktop(r.desktop); !ok {
			// Unpaired while the sync ran.
			continue
		}
		if r.err != nil {
			p := ProblemOf(r.err)
			if p == Unpaired && !s.learn.dismissed[r.desktop] && !slices.Contains(revoked, r.desktop) {
				revoked = append(revoked, r.desktop)
			}
			if problem == NoProblem || p == Unpaired {
				problem = p
			}
			continue
		}
		reached = true
		if r.settings != nil {
			s.learnTakeSettings(*r.settings, r.targetPut)
		}
		if !r.state {
			continue
		}
		var kept []learning.Mirror
		for _, lib := range r.libs {
			i := slices.IndexFunc(list, func(m learning.Mirror) bool { return m.Desktop == r.desktop && m.Library == lib.lib.ID })
			m := learning.Mirror{Desktop: r.desktop, Library: lib.lib.ID}
			if i >= 0 {
				m = cloneMirror(list[i])
			}
			m.Name = lib.lib.Name
			var refused *sync.Error
			reread := false
			switch {
			case lib.sent != nil && lib.putErr == nil:
				m.Settle(lib.sent, lib.put)
			case lib.sent != nil && errors.As(lib.putErr, &refused) && refused.Code == sync.CodeInvalid:
				// Changes the computer refuses it will refuse again: they go, and the next sync reads the library's state afresh, so the records show as the computer has them.
				m.Settle(lib.sent, sync.StatePutResult{})
				reread = true
			}
			if lib.getErr == nil {
				m.Take(lib.changes)
			} else if ProblemOf(lib.getErr) == Gone {
				// A library locked since it was listed tells the phone nothing of it.
				continue
			}
			if reread {
				m.Epoch, m.Rev = "", 0
			}
			kept = append(kept, m)
		}
		// A library the computer no longer lists, as one locked or removed, goes with what the phone kept of it.
		list = slices.DeleteFunc(list, func(m learning.Mirror) bool { return m.Desktop == r.desktop })
		list = append(list, kept...)
		s.learnSaveDesktopList(r.desktop, kept)
	}
	if reached {
		problem = NoProblem
	}
	slices.SortStableFunc(list, func(a, b learning.Mirror) int {
		if a.Key() < b.Key() {
			return -1
		}
		if a.Key() > b.Key() {
			return 1
		}
		return 0
	})
	s.learn.mirrors.Set(list)
	s.learn.revoked.Set(revoked)
	s.learn.problem.Set(problem)
	s.scheduleReminder()
	if s.learn.again {
		s.learn.again = false
		s.SyncLearning()
	}
}

// learnTakeSettings takes a computer's settings in: the language it translates into becomes the phone's, unless the phone's own choice is still to be put, or the computer's is none the phone offers.
func (s *Session) learnTakeSettings(got sync.Settings, put bool) {
	p := s.prefs.Get()
	switch {
	case put && got.Target == p.Target:
		p.TargetPending = false
	case p.TargetPending || got.Target == "" || langs.TargetName(got.Target) == "" || got.Target == p.Target:
		return
	default:
		p.Target = got.Target
	}
	s.SetPrefs(p)
}

// SetTarget makes tag the language subtitles are translated into, on the phone and on the paired computers, which take it at once or at the next sync; a computer paired later takes it too.
func (s *Session) SetTarget(tag string) {
	p := s.prefs.Get()
	if p.Target == tag && !p.TargetPending {
		return
	}
	p.Target, p.TargetPending = tag, true
	s.SetPrefs(p)
	s.SyncLearning()
}

// SetRemind turns the daily review reminder on or off: on, it asks for the permission to show notifications first, and stays off without it.
func (s *Session) SetRemind(on bool) {
	set := func(on bool) {
		p := s.prefs.Get()
		p.Remind = on
		s.SetPrefs(p)
		s.scheduleReminder()
	}
	if !on {
		set(false)
		return
	}
	s.cx.RequestNotificationPermission(func(p core.NotificationPermission) { set(p == core.NotificationsGranted) })
}

// scheduleReminder schedules the review reminder for the next evening something is due, or takes it away when the reminder is off or nothing is to be reviewed. It shows whether the app runs or not, where the platform schedules notifications.
func (s *Session) scheduleReminder() {
	if !s.prefs.Get().Remind {
		s.cx.CloseNotification(reminderID)
		return
	}
	now := s.cx.WallTime()
	items := s.LearnItems()
	at, ok := learning.NextReminder(now, items)
	if !ok {
		s.cx.CloseNotification(reminderID)
		return
	}
	n := len(learning.Due(items, at))
	_ = s.cx.ShowNotification(core.Notification{ID: reminderID, Title: locales.LearnReminderTitle(s.cx), Body: locales.LearnReminderBody(s.cx, n), At: at}, nil)
}

// learnForget forgets what the phone kept of computer id's learning state, as it is unpaired.
func (s *Session) learnForget(id string) {
	s.learn.mirrors.Set(slices.DeleteFunc(slices.Clone(s.learn.mirrors.Get()), func(m learning.Mirror) bool { return m.Desktop == id }))
	s.DismissRevoked(id)
	delete(s.learn.dismissed, id)
	s.learnSave(id, func() error { return s.learn.disk.DeleteDesktop(id) })
	s.scheduleReminder()
}

// ExportLearning writes the words and saved lines to a folder of the app's cache and answers the files, off the engine's goroutine.
func (s *Session) ExportLearning(cols learning.Columns, done func(files []string, err error)) {
	items := s.LearnItems()
	dir := filepath.Join(s.cx.CacheDir(), "export")
	s.work.Go(func() {
		_ = os.RemoveAll(dir)
		files, err := learning.Export(dir, items, cols)
		s.cx.Post(func() { done(files, err) })
	})
}

// learnSaveDesktop keeps computer id's mirrors as they are now.
func (s *Session) learnSaveDesktop(id string) {
	var mine []learning.Mirror
	for _, m := range s.learn.mirrors.Get() {
		if m.Desktop == id {
			mine = append(mine, m)
		}
	}
	s.learnSaveDesktopList(id, mine)
}

func (s *Session) learnSaveDesktopList(id string, mirrors []learning.Mirror) {
	mirrors = slices.Clone(mirrors)
	s.learnSave(id, func() error { return s.learn.disk.SaveDesktop(id, mirrors) })
}

// learnSave runs write, a save of what key names, on a goroutine of its own, unless a later save of it was written already.
func (s *Session) learnSave(key string, write func() error) {
	l := s.learn
	l.saveMu.Lock()
	l.saving[key]++
	gen := l.saving[key]
	l.saveMu.Unlock()
	s.work.Go(func() {
		l.saveMu.Lock()
		defer l.saveMu.Unlock()
		if gen > l.saved[key] && write() == nil {
			l.saved[key] = gen
		}
	})
}

// cloneMirror is m with records of its own, so a change of it leaves m as it was.
func cloneMirror(m learning.Mirror) learning.Mirror {
	m.Records = slices.Clone(m.Records)
	m.Pending = slices.Clone(m.Pending)
	for i, r := range m.Records {
		m.Records[i].Fields = maps.Clone(r.Fields)
	}
	for i, r := range m.Pending {
		m.Pending[i].Fields = maps.Clone(r.Fields)
	}
	return m
}
