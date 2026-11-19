package session

import (
	"cmp"
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	gosync "sync"
	"time"

	"kando/apps/mobile/langs"
	"kando/apps/mobile/learning"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/store"
	"kando/apps/mobile/study"
	kstudy "kando/packages/study"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
)

// learn is the session's part of the learner's study (package learning) besides the libraries' learning state, which study.go keeps and syncs (Session.studies): how long films played on the phone, whether the phone is bringing the learning state up to date with the computers and why it last could not, and the computers that no longer take the phone.
type learn struct {
	disk learning.Disk
	// ready turns true once what the phone kept is read: the time watched and every library's learning state.
	ready   *core.Signal[bool]
	watched *core.Signal[learning.Watched]
	syncing *core.Signal[bool]
	problem *core.Signal[Problem]
	// revoked are the computers that answered that they no longer take the phone, until the user saw it (DismissRevoked).
	revoked   *core.Signal[[]string]
	dismissed map[string]bool
	// settings are the paired computers' settings as each last answered them this run (computer.go).
	settings *core.Signal[map[string]DesktopSettings]
	// watchedKind are the paired computers that sync the time watched (sync.KindWatched), as each said at the last sync this run.
	watchedKind map[string]bool
	// again is set when a sync was asked for while one ran, which runs once it is done, so a change made meanwhile is not left until the next.
	again bool
	// waiting is how many libraries' syncs the sync running waits for (syncStudy).
	waiting int
	// saveMu orders the saves of the time watched, as prefsMu does the preferences': saving is the generation last given out, saved the one last written.
	saveMu        gosync.Mutex
	saving, saved uint64
}

// learnSyncFor is how long a sync waits for a computer, which may be off or out of reach.
const learnSyncFor = 20 * time.Second

// reminderID names the review reminder among the app's notifications.
const reminderID = "learn.review"

// newLearn is the session's study, read from disk off the engine's goroutine: the time watched, and every library's learning state, which a film opened meanwhile may have read already (loadStudy).
func newLearn(s *Session) *learn {
	l := &learn{disk: learning.Open(s.cfg.Dir),
		ready:       core.NewSignal(s.cx, false),
		watched:     core.NewSignalFunc(s.cx, learning.Watched{}, nil),
		syncing:     core.NewSignal(s.cx, false),
		problem:     core.NewSignal(s.cx, NoProblem),
		revoked:     core.NewSignalFunc[[]string](s.cx, nil, nil),
		dismissed:   map[string]bool{},
		settings:    core.NewSignalFunc[map[string]DesktopSettings](s.cx, nil, nil),
		watchedKind: map[string]bool{},
	}
	s.work.Go(func() {
		libs, _ := s.st.Learnings()
		watched, _ := l.disk.Watched()
		s.cx.Post(func() {
			m := maps.Clone(s.studies.GetUntracked())
			for k, lib := range libs {
				if key := studyKey(k.Desktop, k.Library); !s.studyLoading[key] {
					if _, ok := m[key]; !ok {
						m[key] = lib
					}
				}
			}
			s.studies.Set(m)
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

// splitStudyKey is the desktop and library studyKey names; a desktop's ID has no slash in it (store.DesktopID).
func splitStudyKey(k string) (desktop, lib string) {
	desktop, lib, _ = strings.Cut(k, "/")
	return desktop, lib
}

// LearnItems are the words and saved lines of every library the phone keeps the learning state of, the phone's changes not yet sent among them, the latest saved first.
func (s *Session) LearnItems() []learning.Item {
	var out []learning.Item
	for k, l := range s.studies.Get() {
		desktop, lib := splitStudyKey(k)
		out = append(out, learning.Items(desktop, lib, l)...)
	}
	slices.SortStableFunc(out, func(a, b learning.Item) int {
		return cmp.Or(b.SavedAt.Compare(a.SavedAt), strings.Compare(a.Key(), b.Key()))
	})
	return out
}

// Now is the session's clock.
func (s *Session) Now() time.Time { return s.cfg.Now() }

// Watched is how long films played each day on every device: the phone's own, as it keeps it, and every other device's in the libraries the phone keeps the learning state of (learning.Watched.AddOthers), the computers' among them, so a learner's streak and minutes are theirs, not a device's. A locked library is not among them, as the computer does not list it.
func (s *Session) Watched() learning.Watched {
	w := maps.Clone(s.learn.watched.Get())
	if w == nil {
		w = learning.Watched{}
	}
	mine := map[string]bool{}
	for _, d := range s.Desktops() {
		mine[d.DeviceID] = true
	}
	for _, l := range s.studies.Get() {
		w.AddOthers(l, func(device string) bool { return mine[device] })
	}
	return w
}

// Watch adds d, time the film under key played on the phone, to today's: the phone's own, and its time in the learning state of the film's library, which the computer takes at the next sync and adds to its stats, when the computer syncs the time watched (sync.KindWatched) and the phone keeps that library's state.
func (s *Session) Watch(key string, d time.Duration) {
	if d <= 0 {
		return
	}
	now := s.cfg.Now()
	w := maps.Clone(s.learn.watched.Get())
	if w == nil {
		w = learning.Watched{}
	}
	w.Add(now, d)
	s.learn.watched.Set(w)
	s.learnSave(func() error { return s.learn.disk.SaveWatched(w) })
	f, ok := s.Film(key)
	if !ok || f.LibraryID == "" {
		return
	}
	desktop, ok := s.Desktop(f.Desktop)
	l, kept := s.studies.GetUntracked()[studyKey(f.Desktop, f.LibraryID)]
	if !ok || !kept || desktop.DeviceID == "" {
		return
	}
	// Before a sync reached the computer this run, as for a film watched with the computer out of reach, the library's state says whether it syncs the time watched: one that does serves its own time there. A sync that finds it does not after all, as for a read-only phone, takes the change back (learnAsked).
	syncs, known := s.learn.watchedKind[f.Desktop]
	if !known {
		syncs = slices.ContainsFunc(l.Records, func(r sync.Record) bool { return r.Kind == sync.KindWatched })
	}
	if !syncs {
		return
	}
	s.setStudy(f.Desktop, f.LibraryID, study.Change(l, learning.WatchedChange(l, desktop.DeviceID, now, d)))
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
	for k, l := range s.studies.Get() {
		if desktop, _ := splitStudyKey(k); desktop == id {
			n += len(l.Pending)
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

// learnChange makes the change change gives, stamped with the phone's ID on the item's computer and made from the values the computer has (study.Based), in the item's library's learning state, and keeps it.
func (s *Session) learnChange(it learning.Item, change func(device string) sync.Record) {
	d, ok := s.Desktop(it.Desktop)
	if !ok {
		return
	}
	l, ok := s.studies.GetUntracked()[studyKey(it.Desktop, it.Library)]
	if !ok {
		return
	}
	s.setStudy(it.Desktop, it.Library, study.Change(l, study.Based(l, change(d.DeviceID))))
	s.scheduleReminder()
}

// learnResult is what a sync found of a computer: the libraries it lists, or why it could not be asked, whether it serves the learning state, and the language it translates into when it shares its settings.
type learnResult struct {
	desktop  string
	err      error
	state    bool
	libs     []sync.Library
	settings *sync.Settings
	// targetPut is set when the phone's language was put to the computer, readOnly when the computer takes no change of the phone's, and watched when it syncs the time watched.
	targetPut, readOnly, watched bool
}

// SyncLearning brings the learning state up to date with every paired computer that serves it, each library the computer lists synced as a film's is (syncStudy): the phone's changes put first, then the computer's asked for; and the language translated into, the phone's put to a computer that has not taken it yet, else the computer's taken. A computer out of reach is left as it was. LearnSyncing is true until every library's sync is done.
func (s *Session) SyncLearning() {
	if !s.learn.ready.GetUntracked() {
		// The libraries kept are not read yet: the sync runs once they are (newLearn).
		return
	}
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
	prefs := s.prefs.Get()
	target := s.Target()
	results := make([]learnResult, len(desktops))
	var wg gosync.WaitGroup
	for i, d := range desktops {
		c := s.Client(d.ID)
		putTarget := slices.Contains(prefs.TargetPending, d.ID)
		wg.Add(1)
		s.work.Go(func() {
			defer wg.Done()
			results[i] = learnAsk(s.ctx, c, d.ID, putTarget, target)
		})
	}
	s.work.Go(func() {
		wg.Wait()
		s.cx.Post(func() { s.learnAsked(results) })
	})
}

// learnAsk asks computer desktop through c, off the engine's goroutine, what a sync needs of it: whether it serves the learning state and the libraries it lists, and its settings, once the phone's language is put to it when putTarget is set. A read-only phone (sync.Hello.ReadOnly) puts nothing.
func learnAsk(ctx context.Context, c *sync.Client, desktop string, putTarget bool, target string) learnResult {
	out := learnResult{desktop: desktop}
	if c == nil {
		out.err = &sync.Error{Status: 401}
		return out
	}
	ctx, cancel := context.WithTimeout(ctx, learnSyncFor)
	defer cancel()
	h, err := c.Hello(ctx)
	if errors.Is(err, sync.ErrV1Only) {
		return out
	}
	if err != nil {
		out.err = err
		return out
	}
	out.readOnly = h.ReadOnly
	if slices.Contains(h.Ops, sync.OpSettingsPut) && putTarget && !h.ReadOnly {
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
	out.watched = slices.Contains(h.State, sync.KindWatched) && !h.ReadOnly
	if !out.state {
		return out
	}
	out.libs, out.err = c.Libraries(ctx)
	return out
}

// learnAsked takes what the computers answered in, on the engine's goroutine, and syncs each library they list. A library a computer no longer lists, as one locked or removed, goes with what the phone kept of it, unless the phone has changes of it the computer has not taken yet: those stay until it lists the library again, as once it is unlocked, so a review made meanwhile is not lost. One removed for good keeps them until the computer is unpaired.
func (s *Session) learnAsked(results []learnResult) {
	reached, problem := false, NoProblem
	targetWas := s.prefs.Get().Target
	revoked := slices.Clone(s.learn.revoked.Get())
	type lib struct{ desktop, id string }
	var libs []lib
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
		s.learn.watchedKind[r.desktop] = r.watched
		if !r.watched {
			// The time watched kept for a computer that does not take it, as Watch keeps before a sync reached it, goes rather than wait for good.
			for k, l := range s.studies.GetUntracked() {
				if desktop, id := splitStudyKey(k); desktop == r.desktop && slices.ContainsFunc(l.Pending, pendingWatched) {
					s.setStudy(desktop, id, study.Unpend(l, sync.KindWatched))
				}
			}
		}
		if r.settings != nil {
			s.learnTakeSettings(r.desktop, *r.settings, r.targetPut)
			s.setDesktopSettings(r.desktop, DesktopSettings{Settings: *r.settings, ReadOnly: r.readOnly})
		}
		if !r.state {
			continue
		}
		for k, l := range s.studies.GetUntracked() {
			desktop, id := splitStudyKey(k)
			if desktop == r.desktop && !holdsChanges(l) && !slices.ContainsFunc(r.libs, func(x sync.Library) bool { return x.ID == id }) {
				s.dropStudy(desktop, id)
			}
		}
		for _, l := range r.libs {
			libs = append(libs, lib{r.desktop, l.ID})
		}
	}
	if reached {
		problem = NoProblem
	}
	s.learn.revoked.Set(revoked)
	s.learn.problem.Set(problem)
	// A language taken from a computer goes to the others at once.
	if p := s.prefs.Get(); p.Target != targetWas && len(p.TargetPending) > 0 {
		s.learn.again = true
	}
	s.learn.waiting += len(libs)
	for _, l := range libs {
		s.syncLibrary(l.desktop, l.id, s.learnLibrarySynced)
	}
	if s.learn.waiting == 0 {
		s.learnDone()
	}
}

// learnLibrarySynced is told a library's sync is done, with what it ended with: a library locked since it was listed, which tells the phone nothing of it, goes as one not listed does (learnAsked).
func (s *Session) learnLibrarySynced(desktop, lib string, err error) {
	if err != nil && ProblemOf(err) == Gone {
		if l, ok := s.studies.GetUntracked()[studyKey(desktop, lib)]; ok && !holdsChanges(l) {
			s.dropStudy(desktop, lib)
		}
	}
	s.learn.waiting--
	if s.learn.waiting == 0 {
		s.learnDone()
	}
}

// learnDone ends a sync once every library's is done, and runs the one asked for meanwhile.
func (s *Session) learnDone() {
	s.learn.syncing.Set(false)
	s.scheduleReminder()
	if s.learn.again {
		s.learn.again = false
		s.SyncLearning()
	}
}

// learnTakeSettings takes computer desktop's settings in. Put, the computer took the phone's language when it is the one it answers, and is no longer to take it. Asked, the language it translates into becomes the phone's when it changed it, and the other paired computers are to take it, so the phone and the computers name one language: unless the computer is still to take the phone's own, or the computer's is none the phone offers.
func (s *Session) learnTakeSettings(desktop string, got sync.Settings, put bool) {
	p := s.prefs.Get()
	pending := slices.Contains(p.TargetPending, desktop)
	switch {
	case put && pending && got.Target == p.Target:
		p.TargetPending = slices.DeleteFunc(slices.Clone(p.TargetPending), func(id string) bool { return id == desktop })
	case put || pending || got.Target == "" || langs.TargetName(got.Target) == "" || got.Target == p.Target:
		return
	default:
		p.Target, p.TargetPending = got.Target, nil
		for _, d := range s.Desktops() {
			if d.ID != desktop {
				p.TargetPending = append(p.TargetPending, d.ID)
			}
		}
	}
	s.SetPrefs(p)
}

// SetTarget makes tag the language subtitles are translated into, on the phone and on the paired computers, which take it at once or at the next sync; a computer paired later takes it too (AddDesktop).
func (s *Session) SetTarget(tag string) {
	p := s.prefs.Get()
	if p.Target == tag {
		return
	}
	p.Target, p.TargetPending = tag, nil
	for _, d := range s.Desktops() {
		p.TargetPending = append(p.TargetPending, d.ID)
	}
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
	for k := range s.studies.GetUntracked() {
		if desktop, lib := splitStudyKey(k); desktop == id {
			s.dropStudy(desktop, lib)
		}
	}
	s.DismissRevoked(id)
	delete(s.learn.dismissed, id)
	s.forgetDesktopSettings(id)
	delete(s.learn.watchedKind, id)
	if p := s.prefs.Get(); slices.Contains(p.TargetPending, id) {
		p.TargetPending = slices.DeleteFunc(slices.Clone(p.TargetPending), func(x string) bool { return x == id })
		s.SetPrefs(p)
	}
	s.scheduleReminder()
}

// ExportOptions are what the Learn tab's export writes, as its page last chose it: Anki's notes with the line each word is from, its translation and the saved lines, until the learner changes them.
func (s *Session) ExportOptions() kstudy.VocabOptions {
	e := s.prefs.Get().Export
	format := e.Format
	if format != kstudy.VocabCSV && format != kstudy.VocabTSV {
		format = kstudy.VocabAnki
	}
	return kstudy.VocabOptions{Format: format, Context: !e.NoSentence, Translation: !e.NoTranslation, Lines: !e.NoLines}
}

// SetExportOptions keeps o as what the export writes.
func (s *Session) SetExportOptions(o kstudy.VocabOptions) {
	p := s.prefs.Get()
	p.Export = store.Export{Format: o.Format, NoSentence: !o.Context, NoTranslation: !o.Translation, NoLines: !o.Lines}
	if p.Export.Format == kstudy.VocabAnki {
		p.Export.Format = ""
	}
	s.SetPrefs(p)
}

// ExportLearning writes the words, and the saved lines when ExportOptions ask for them, to a folder of the app's cache as ExportOptions say (learning.Export), and answers the file, off the engine's goroutine.
func (s *Session) ExportLearning(done func(file string, err error)) {
	items, o := s.LearnItems(), s.ExportOptions()
	dir := filepath.Join(s.cx.CacheDir(), "export")
	s.work.Go(func() {
		_ = os.RemoveAll(dir)
		file, err := learning.Export(dir, items, o)
		s.cx.Post(func() { done(file, err) })
	})
}

// learnSave runs write, a save of the time watched, on a goroutine of its own, unless a later save of it was written already.
func (s *Session) learnSave(write func() error) {
	l := s.learn
	l.saveMu.Lock()
	l.saving++
	gen := l.saving
	l.saveMu.Unlock()
	s.work.Go(func() {
		l.saveMu.Lock()
		defer l.saveMu.Unlock()
		if gen > l.saved && write() == nil {
			l.saved = gen
		}
	})
}

// LineClips are the clips of items' lines to dictate (learning.ClipOf), by their keys, read off the engine's goroutine and answered on it: an item whose film is not kept whole on the phone has none, as its line cannot be heard.
func (s *Session) LineClips(items []learning.Item, done func(map[string]learning.Clip)) {
	type kept struct {
		video string
		film  store.Film
	}
	films := map[string]kept{}
	for _, it := range items {
		key := store.Key(it.Desktop, it.Film)
		if f, ok := s.Film(key); ok && f.Done() && it.Film != "" {
			films[key] = kept{video: filepath.Join(s.st.FilmDir(f.Desktop, f.Film.ID), f.Video), film: f}
		}
	}
	s.work.Go(func() {
		out := map[string]learning.Clip{}
		cues := map[string][]sync.Cue{}
		for _, it := range items {
			key := store.Key(it.Desktop, it.Film)
			k, ok := films[key]
			if !ok {
				continue
			}
			if _, read := cues[key]; !read {
				cues[key], _, _ = s.st.Cues(k.film.Desktop, k.film.Film.ID)
			}
			if c, ok := learning.ClipOf(it, k.video, time.Duration(k.film.DelayMS)*time.Millisecond, cues[key]); ok {
				out[it.Key()] = c
			}
		}
		s.cx.Post(func() { done(out) })
	})
}

// ReviewMode is how the Learn tab reviews, as the learner last picked it: ReviewCards or ReviewDictation.
func (s *Session) ReviewMode() string {
	if m := s.prefs.Get().Review; m == ReviewDictation {
		return m
	}
	return ReviewCards
}

// SetReviewMode keeps m as how the Learn tab reviews.
func (s *Session) SetReviewMode(m string) {
	p := s.prefs.Get()
	p.Review = ""
	if m == ReviewDictation {
		p.Review = m
	}
	s.SetPrefs(p)
}

// The ways the Learn tab reviews: a card that shows its answer on a press, or the line heard and typed in, then checked.
const (
	ReviewCards     = "cards"
	ReviewDictation = "dictation"
)

// pendingWatched reports whether r is a change of the phone's time watched (sync.KindWatched).
func pendingWatched(r sync.Record) bool { return r.Kind == sync.KindWatched }

// holdsChanges reports whether l has changes of the learner's the computer has not taken yet, which keep a library it no longer lists, as one locked: the phone's time watched is not among them, so a locked library's time does not stay in Learn's stats (Watched) for the minutes played in it before it was locked.
func holdsChanges(l study.Library) bool {
	return slices.ContainsFunc(l.Pending, func(r sync.Record) bool { return !pendingWatched(r) })
}
