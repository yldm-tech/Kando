package session

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"slices"
	gosync "sync"
	"time"

	"kando/apps/mobile/store"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
)

// The computers' queues over protocol v2 (kando/packages/sync, v2.go): while the app runs, the session follows the queue of every paired computer, listing its jobs and waiting for their events, so the Jobs page shows each step as the computer moves it along; the links the user sends wait on the phone until their computer takes them, so a link sent while the computer is off goes once it answers again.

// Reach is how a computer's queue stands as the phone follows it.
type Reach uint8

const (
	// Reaching is a computer the phone has not heard from since it began following it.
	Reaching Reach = iota
	// Online is one that answers.
	Online
	// Offline is one the phone cannot reach: off, asleep, on another network, or with Phone Sync off. The jobs it listed last stay as they were, and the links sent meanwhile wait on the phone.
	Offline
	// Older is one that speaks protocol v1 alone, an older Kando, which takes no links and lists no jobs.
	Older
	// Revoked is one that no longer takes the phone's token.
	Revoked
)

// Queue is a computer's queue as the phone follows it.
type Queue struct {
	Reach Reach
	// Ops are the operations the computer answers, as it said when it last answered (sync.Hello), and Options the job options it takes with a link.
	Ops     []string
	Options []string
	// Receive is what the computer does with a link (sync.ReceiveQueue, ReceiveAsk or ReceiveOff), and ReadOnly is set while its user made this phone read-only, as it said when it last answered.
	Receive  string
	ReadOnly bool
	// Jobs are its jobs as it last told them: the running one, the waiting ones in the order they will run, then the others, newest first.
	Jobs []sync.Job
	// Seen is when it last answered, zero before it has.
	Seen time.Time
	// Relayed is a computer the phone last heard of through the user's storage, away from its network (relay.go): Online while the state.json it writes there is fresh, Offline once it is sync.RelayStale old. Its jobs are as that file last said, and the links and actions sent to it are left in the storage, where it answers them within a minute or two while it runs.
	Relayed bool
	// Storage is why the user's storage last failed the phone while it follows the computer there: it refused the links, as once they lapsed or the computer's key changed, the bucket is gone or full, or the phone could not connect to it; sync.RelayNoFailure while it serves the phone.
	Storage sync.RelayFailure
}

// Can reports whether the computer answers op for this phone: none that changes something (sync.Mutates) while it made the phone read-only.
func (q Queue) Can(op string) bool {
	return slices.Contains(q.Ops, op) && !(q.ReadOnly && sync.Mutates(op))
}

// Offers reports whether the phone offers job option name (sync.OptionQuality, OptionSubtitles) with a link to the computer: one it takes, while it queues links at once, as one that asks its user first starts them with its own settings.
func (q Queue) Offers(name string) bool {
	return q.Reach == Online && q.Can(sync.OpJobCreate) && q.Receive == sync.ReceiveQueue && slices.Contains(q.Options, name)
}

// Outgoing is a link sent from this phone that its computer has not taken yet: waiting for the computer while Refusal is nil, else refused for Refusal's reason (its Code one of sync's, its Message in the computer's words), which is kept with the link.
type Outgoing struct {
	store.Link
}

// QueueEvents are what the app is told of the queues, on the engine's goroutine: a job that finished while the app followed it, a link a computer took, a list a computer subscribed to (Session.Subscribe), a film a computer changed as asked (Acted), and an action on a job, a link, a subscription or a film that failed.
type QueueEvents struct {
	Done       func(desktop string, job sync.Job)
	Sent       func(desktop string, created sync.JobCreated)
	Subscribed func(desktop string, got sync.Subscribed)
	// Acted is a change to the film titled title that desktop took (films.go): op is sync.OpFilmRetranslate, OpFilmDelete or OpFilmVideo.
	Acted  func(desktop, op, title string)
	Failed func(desktop string, err error)
}

// remote is the session's part that follows the queues. Its fields belong to the engine's goroutine, but for linksMu's and ctx.
type remote struct {
	// ctx ends as the session does (end), which stops the followers and the calls still under way, so the session does not wait for a computer that does not answer.
	ctx    context.Context
	end    context.CancelFunc
	queues *core.Signal[map[string]Queue]
	links  *core.Signal[[]Outgoing]
	// busy are the jobs an action is under way for, by Key(desktop, job), and the computers updating their downloader, by Key(desktop, ytdlpKey).
	busy    *core.Signal[map[string]bool]
	follows map[string]*follower
	// sending are the links going out now, by ID, so a link is not sent twice at once.
	sending map[string]bool
	events  QueueEvents
	// linksMu orders the saves of the links as prefsMu does the preferences'.
	linksMu                 gosync.Mutex
	linksSaving, linksSaved uint64
	// relays are the computers' storage relays, by ID, and relayCalls the calls left in them whose replies the session waits for, by call ID (relay.go); relayStudies what the phone follows there of each computer's learning state and settings (relay_study.go).
	relays       map[string]*relayLink
	relayCalls   map[string]relayCall
	relayStudies map[string]*relayStudy
	// shelf is the films the computers put in their storage, by ID (relay_films.go), which the bindings read; under shelfMu, shelfCopy is the same for the posters' fetchers off the engine's goroutine, and shelfAway the computers the phone reaches through the storage now.
	shelf     *core.Signal[map[string]relayFilms]
	shelfMu   gosync.Mutex
	shelfCopy map[string]relayFilms
	shelfAway map[string]bool
}

// follower is the goroutine following a computer's queue.
type follower struct{ cancel context.CancelFunc }

// ytdlpKey stands for the computer's downloader among the busy jobs.
const ytdlpKey = "\x00ytdlp"

// The waits between the tries of a computer the phone cannot reach, the first doubled each time up to the last, and the wait before asking again a computer that speaks v1 alone, which may be updated meanwhile.
const (
	reachFirst  = time.Second
	reachLast   = 15 * time.Second
	olderWait   = time.Minute
	eventsSlack = 15 * time.Second
	// actFor is how long an action on a job waits for its computer.
	actFor = 20 * time.Second
	// resendAfter is the wait before a link is sent again that did not reach a computer that answers otherwise, as when one call dropped or timed out while its events kept coming.
	resendAfter = 5 * time.Second
)

func newRemote(cx *core.Scope) *remote {
	ctx, end := context.WithCancel(context.Background())
	return &remote{
		ctx:     ctx,
		end:     end,
		queues:  core.NewSignalFunc(cx, map[string]Queue{}, nil),
		links:   core.NewSignalFunc[[]Outgoing](cx, nil, nil),
		busy:    core.NewSignalFunc(cx, map[string]bool{}, nil),
		follows: map[string]*follower{},
		sending: map[string]bool{},
		relays:  map[string]*relayLink{},
		// The calls left in the computers' storage (relay.go).
		relayCalls:   map[string]relayCall{},
		relayStudies: map[string]*relayStudy{},
		shelf:        core.NewSignalFunc(cx, map[string]relayFilms{}, nil),
		shelfAway:    map[string]bool{},
	}
}

// OnQueue has events told what happens to the queues.
func (s *Session) OnQueue(events QueueEvents) { s.remote.events = events }

// Queue is the queue of desktop id as the phone follows it.
func (s *Session) Queue(id string) Queue { return s.remote.queues.Get()[id] }

// Outgoing are the links sent from this phone that their computers have not taken yet, oldest first.
func (s *Session) Outgoing() []Outgoing { return s.remote.links.Get() }

// Unfinished counts what the Jobs page holds that is not done, over every paired computer, as the badge on the way to it shows: active are the jobs running, waiting or paused and the links waiting on the phone, failed the jobs that failed and the links refused, which need the user. Read it in a binding.
func (s *Session) Unfinished() (active, failed int) {
	for _, q := range s.remote.queues.Get() {
		for _, j := range q.Jobs {
			switch j.State {
			case sync.JobRunning, sync.JobWaiting, sync.JobPaused:
				active++
			case sync.JobFailed:
				failed++
			}
		}
	}
	for _, o := range s.remote.links.Get() {
		if o.Refusal != nil {
			failed++
		} else {
			active++
		}
	}
	return active, failed
}

// Busy reports whether an action on job id of desktop is under way.
func (s *Session) Busy(desktop, id string) bool { return s.remote.busy.Get()[store.Key(desktop, id)] }

// Updating reports whether desktop was asked to update its downloader and has not answered yet.
func (s *Session) Updating(desktop string) bool { return s.Busy(desktop, ytdlpKey) }

// Send sends link to desktop for it to process with opts: kept on the phone until the computer takes or refuses it, and sent again under the same call ID while it cannot be reached.
func (s *Session) Send(desktop, link string, opts sync.JobOptions) {
	l := store.Link{ID: sync.NewCallID(), Desktop: desktop, URL: link, Options: opts, Added: s.cfg.Now()}
	s.setLinks(append(slices.Clone(s.remote.links.Get()), Outgoing{Link: l}))
	s.sendLink(l)
}

// Resend sends a refused link again, as a new call, so the computer looks at it afresh. One refused as the computer made the phone read-only has the computer asked afresh too (follow), so its page shows what it lets the phone do now.
func (s *Session) Resend(id string) {
	list := slices.Clone(s.remote.links.Get())
	i := slices.IndexFunc(list, func(o Outgoing) bool { return o.ID == id })
	if i < 0 || s.remote.sending[id] {
		return
	}
	if r := list[i].Refusal; r != nil && r.Code == sync.CodeReadOnly {
		s.follow(list[i].Desktop)
	}
	list[i] = Outgoing{Link: store.Link{ID: sync.NewCallID(), Desktop: list[i].Desktop, URL: list[i].URL, Options: list[i].Options, Added: list[i].Added}}
	s.setLinks(list)
	s.sendLink(list[i].Link)
}

// Forget drops a link not taken yet.
func (s *Session) Forget(id string) {
	s.setLinks(slices.DeleteFunc(slices.Clone(s.remote.links.Get()), func(o Outgoing) bool { return o.ID == id }))
}

// Act runs op, one of sync.OpJobCancel, OpJobRetry, OpJobPause and OpJobResume, on job id of desktop: away from its network, through its storage, where the action waits for the computer.
func (s *Session) Act(desktop, op, id string) {
	c := s.Client(desktop)
	key := store.Key(desktop, id)
	if s.remote.queues.GetUntracked()[desktop].Relayed {
		s.relayAct(desktop, key, op, sync.JobRef{ID: id})
		return
	}
	if c == nil || s.remote.busy.Get()[key] {
		return
	}
	s.setBusy(key, true)
	s.work.Go(func() {
		ctx, cancel := context.WithTimeout(s.remote.ctx, actFor)
		defer cancel()
		job, err := c.ActOnJob(ctx, op, id)
		s.cx.Post(func() {
			s.setBusy(key, false)
			if err != nil {
				s.queueFailed(desktop, err)
				return
			}
			s.upsertJobs(desktop, []sync.Job{job})
		})
	})
}

// UpdateYTDLP has desktop update its downloader; the downloads that failed for it run again once it is updated, as their events tell.
func (s *Session) UpdateYTDLP(desktop string) {
	c := s.Client(desktop)
	key := store.Key(desktop, ytdlpKey)
	if s.remote.queues.GetUntracked()[desktop].Relayed {
		s.relayAct(desktop, key, sync.OpUpdateYTDLP, nil)
		return
	}
	if c == nil || s.remote.busy.Get()[key] {
		return
	}
	s.setBusy(key, true)
	s.work.Go(func() {
		ctx, cancel := context.WithTimeout(s.remote.ctx, actFor)
		defer cancel()
		_, err := c.UpdateYTDLP(ctx)
		s.cx.Post(func() {
			s.setBusy(key, false)
			if err != nil {
				s.queueFailed(desktop, err)
			}
		})
	})
}

// ErrFilmGone is the error Failed is told of when a done job's film is not on its computer: it was removed, or its library is locked.
var ErrFilmGone = errors.New("the computer does not serve the job's film")

// JobFilm finds the film job made on desktop, and its library, for its page: done gets them on the engine's goroutine once found. While it looks, the job is busy (Busy) and another look for it is dropped, so a second press does not open the page twice; one that fails tells Failed why, ErrFilmGone for a film the computer does not serve.
func (s *Session) JobFilm(desktop string, job sync.Job, done func(lib sync.Library, f sync.Film)) {
	key := store.Key(desktop, job.ID)
	if job.Film == "" || s.remote.busy.GetUntracked()[key] {
		return
	}
	c := s.Client(desktop)
	if c == nil {
		s.queueFailed(desktop, ErrFilmGone)
		return
	}
	s.setBusy(key, true)
	s.work.Go(func() {
		ctx, cancel := context.WithTimeout(s.remote.ctx, actFor)
		defer cancel()
		lib, f, err := findFilm(ctx, c, job)
		s.cx.Post(func() {
			s.setBusy(key, false)
			if err != nil {
				s.queueFailed(desktop, err)
				return
			}
			done(lib, f)
		})
	})
}

func findFilm(ctx context.Context, c *sync.Client, job sync.Job) (sync.Library, sync.Film, error) {
	libs, err := c.Libraries(ctx)
	if err != nil {
		return sync.Library{}, sync.Film{}, err
	}
	i := slices.IndexFunc(libs, func(l sync.Library) bool { return l.ID == job.Library })
	if i < 0 {
		return sync.Library{}, sync.Film{}, ErrFilmGone
	}
	films, err := c.Films(ctx, libs[i].ID)
	if ProblemOf(err) == Gone {
		return sync.Library{}, sync.Film{}, ErrFilmGone
	}
	if err != nil {
		return sync.Library{}, sync.Film{}, err
	}
	j := slices.IndexFunc(films, func(f sync.Film) bool { return f.ID == job.Film })
	if j < 0 {
		return sync.Library{}, sync.Film{}, ErrFilmGone
	}
	return libs[i], films[j], nil
}

// loadLinks takes the links kept on the phone, read as the store is, and sends them on as their computers answer.
func (s *Session) loadLinks(links []store.Link) {
	out := make([]Outgoing, len(links))
	for i, l := range links {
		out[i] = Outgoing{Link: l}
	}
	s.remote.links.Set(out)
}

// follow starts following the queue of desktop id, again from the start for one followed already, as when it was paired again.
func (s *Session) follow(id string) {
	s.unfollow(id)
	c := s.Client(id)
	if c == nil {
		return
	}
	ctx, cancel := context.WithCancel(s.remote.ctx)
	f := &follower{cancel: cancel}
	s.remote.follows[id] = f
	s.setQueue(id, func(q Queue) Queue { q.Reach = Reaching; return q })
	d, _ := s.Desktop(id)
	rl := s.relayOf(id)
	s.work.Go(func() { s.followQueue(ctx, id, d.DeviceID, c, rl, f) })
}

// unfollow stops following the queue of desktop id.
func (s *Session) unfollow(id string) {
	if f, ok := s.remote.follows[id]; ok {
		f.cancel()
		delete(s.remote.follows, id)
	}
}

// forgetQueue drops what the phone knew of desktop id's queue, the links waiting for it and its relay, as it is unpaired.
func (s *Session) forgetQueue(id string) {
	s.unfollow(id)
	s.forgetRelay(id)
	q := maps.Clone(s.remote.queues.GetUntracked())
	delete(q, id)
	s.remote.queues.Set(q)
	links := s.remote.links.GetUntracked()
	if slices.ContainsFunc(links, func(o Outgoing) bool { return o.Desktop == id }) {
		s.setLinks(slices.DeleteFunc(slices.Clone(links), func(o Outgoing) bool { return o.Desktop == id }))
	}
}

// stopFollowing ends every follower and the calls under way, as the session ends.
func (s *Session) stopFollowing() {
	s.remote.end()
}

// followQueue follows desktop id's queue on a goroutine of its own until ctx ends: it says hello, joins the computer's storage relay when it offers one (relay.go), lists the jobs and waits for their events, listing again when the computer says to or after it could not be reached, and posts what it learns while f is still the desktop's follower. While the computer does not answer on its network, the phone, the paired device, follows it through the relay rl when it joined one, every relayEvery, trying its network first each time.
func (s *Session) followQueue(ctx context.Context, id, device string, c *sync.Client, rl *relayLink, f *follower) {
	post := func(fn func()) {
		s.cx.Post(func() {
			if s.remote.follows[id] == f {
				fn()
			}
		})
	}
	pause := func(d time.Duration) bool {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(d):
			return true
		}
	}
	unreachable := func(err error) {
		reach := Offline
		if ProblemOf(err) == Unpaired {
			reach = Revoked
		}
		post(func() { s.setQueue(id, func(q Queue) Queue { q.Reach = reach; return q }) })
	}
	wait := reachFirst
	backOff := func() bool {
		ok := pause(wait)
		wait = min(2*wait, reachLast)
		return ok
	}
	for ctx.Err() == nil {
		hctx, hcancel := ctx, context.CancelFunc(func() {})
		if rl.joined(s, id, device) {
			hctx, hcancel = context.WithTimeout(ctx, lanTry)
		}
		hello, err := c.Hello(hctx)
		hcancel()
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, sync.ErrV1Only) || (err == nil && !slices.Contains(hello.Ops, sync.OpJobs)) {
			post(func() {
				s.setQueue(id, func(q Queue) Queue { return Queue{Reach: Older, Seen: s.cfg.Now()} })
				s.refuseWaiting(id)
			})
			if !pause(olderWait) {
				return
			}
			continue
		}
		if err != nil {
			if ProblemOf(err) != Unpaired && s.followRelay(ctx, id, device, rl, post) {
				wait = reachFirst
				if !pause(s.relayEvery()) {
					return
				}
				continue
			}
			unreachable(err)
			if !backOff() {
				return
			}
			continue
		}
		s.relayJoin(ctx, id, device, c, hello, rl)
		if ctx.Err() != nil {
			return
		}
		list, err := c.Jobs(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			unreachable(err)
			if !backOff() {
				return
			}
			continue
		}
		post(func() {
			was := s.remote.queues.GetUntracked()[id].Relayed
			s.setQueue(id, func(Queue) Queue {
				return Queue{Reach: Online, Ops: hello.Ops, Options: hello.Options, Receive: hello.Receive, ReadOnly: hello.ReadOnly, Jobs: list.Jobs, Seen: s.cfg.Now()}
			})
			s.relayLanded(id)
			s.sendWaiting(id)
			if was {
				// Back on the computer's network, the learning state and the settings left for the storage go directly.
				s.SyncLearning()
			}
		})
		if !slices.Contains(hello.Ops, sync.OpEvents) {
			// A computer that tells no events is listed again from time to time.
			if !pause(reachLast) {
				return
			}
			continue
		}
		epoch, rev := list.Epoch, list.Rev
		for {
			wctx, cancel := context.WithTimeout(ctx, sync.MaxEventWait+eventsSlack)
			ev, err := c.Events(wctx, epoch, rev, sync.MaxEventWait)
			cancel()
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				// Asked again from hello, which tells whether the computer is still there.
				if !backOff() {
					return
				}
				break
			}
			wait = reachFirst
			if ev.Reset {
				break
			}
			epoch, rev = ev.Epoch, ev.Rev
			if len(ev.Events) > 0 {
				events := ev.Events
				post(func() { s.applyEvents(id, events) })
			}
		}
	}
}

// applyEvents takes events of desktop id's queue into it.
func (s *Session) applyEvents(id string, events []sync.Event) {
	var changed []sync.Job
	var gone []string
	for _, e := range events {
		switch {
		case e.Kind == sync.EventJob && e.Job != nil:
			changed = append(changed, *e.Job)
		case e.Kind == sync.EventJobGone:
			gone = append(gone, e.ID)
		}
	}
	if len(gone) > 0 {
		s.setQueue(id, func(q Queue) Queue {
			q.Jobs = slices.DeleteFunc(slices.Clone(q.Jobs), func(j sync.Job) bool { return slices.Contains(gone, j.ID) })
			return q
		})
	}
	s.upsertJobs(id, changed)
}

// upsertJobs puts jobs in desktop's queue in place of those with their IDs, or where a list would show them, tells Done of each that finished, and then reads desktop's films again.
func (s *Session) upsertJobs(desktop string, jobs []sync.Job) {
	if len(jobs) == 0 {
		return
	}
	var finished []sync.Job
	s.setQueue(desktop, func(q Queue) Queue {
		list := slices.Clone(q.Jobs)
		for _, job := range jobs {
			list, finished = upsertJob(list, job, finished)
		}
		q.Jobs = list
		return q
	})
	if s.remote.events.Done != nil {
		for _, job := range finished {
			s.remote.events.Done(desktop, job)
		}
	}
	// A job that finished may have added a film, or changed one, which the Library lists only from the computer's own listing.
	if len(finished) > 0 {
		s.RefreshDesktop(desktop)
	}
}

// upsertJob puts job in list, adding it to finished when it finished since list told it. A job whose place in the order is unchanged keeps it; else it goes last among the running and waiting ones, or first among the others, which are newest first.
func upsertJob(list []sync.Job, job sync.Job, finished []sync.Job) ([]sync.Job, []sync.Job) {
	i := slices.IndexFunc(list, func(j sync.Job) bool { return j.ID == job.ID })
	if i >= 0 {
		before := list[i]
		if before.State != sync.JobDone && job.State == sync.JobDone {
			finished = append(finished, job)
		}
		if rank(before) == rank(job) {
			list[i] = job
			return list, finished
		}
		list = slices.Delete(list, i, i+1)
	}
	r := rank(job)
	at := slices.IndexFunc(list, func(j sync.Job) bool { return rank(j) > r || (r == 2 && rank(j) == 2) })
	if at < 0 {
		at = len(list)
	}
	return slices.Insert(list, at, job), finished
}

// rank is where a job stands in a list: running first, then waiting and paused, then the others.
func rank(j sync.Job) int {
	switch j.State {
	case sync.JobRunning:
		return 0
	case sync.JobWaiting, sync.JobPaused:
		return 1
	}
	return 2
}

// sendWaiting sends the links waiting for desktop, as it answers again, or leaves them in its storage as the phone follows it there; a computer that answers without taking links refuses them on the phone, unless the phone, away from it, has not heard what it takes yet.
func (s *Session) sendWaiting(desktop string) {
	q := s.remote.queues.GetUntracked()[desktop]
	if q.Relayed && len(q.Ops) == 0 {
		return
	}
	if !q.Can(sync.OpJobCreate) {
		s.refuseWaiting(desktop)
		return
	}
	for _, o := range s.remote.links.GetUntracked() {
		if o.Desktop == desktop && o.Refusal == nil {
			s.sendLink(o.Link)
		}
	}
}

// refuseWaiting refuses the links waiting for desktop, a computer that takes no links, as it would: one too old to take any, or one that made the phone read-only. They say why until the user sends them again or drops them, rather than waiting for a computer that will not take them.
func (s *Session) refuseWaiting(desktop string) {
	links := s.remote.links.GetUntracked()
	if !slices.ContainsFunc(links, func(o Outgoing) bool { return o.Desktop == desktop && o.Refusal == nil }) {
		return
	}
	code := sync.CodeUnsupported
	if q := s.remote.queues.GetUntracked()[desktop]; q.ReadOnly && slices.Contains(q.Ops, sync.OpJobCreate) {
		code = sync.CodeReadOnly
	}
	s.setLinks(slices.Clone(links), func(o *Outgoing) {
		if o.Desktop == desktop && o.Refusal == nil {
			o.Refusal = &sync.ErrorBody{Code: code}
		}
	})
}

// sendLink sends l to its computer unless it is going already: taken, it leaves the phone; refused, it stays with why; while the computer cannot be reached, it waits, and is sent again after resendAfter if the computer answers otherwise meanwhile, since its follower sends the waiting links only as the computer comes back. Away from the computer's network, it is left in the computer's storage instead (relayLeave).
func (s *Session) sendLink(l store.Link) {
	if s.remote.queues.GetUntracked()[l.Desktop].Relayed {
		s.relayLeave(l)
		return
	}
	c := s.Client(l.Desktop)
	if c == nil || s.remote.sending[l.ID] {
		return
	}
	s.remote.sending[l.ID] = true
	opts := sendOptions(s.remote.queues.GetUntracked()[l.Desktop], l.Options)
	s.work.Go(func() {
		ctx, cancel := context.WithTimeout(s.remote.ctx, actFor)
		defer cancel()
		var created sync.JobCreated
		err := c.CallID(ctx, l.ID, sync.OpJobCreate, sync.JobCreate{Link: l.URL, Options: opts}, &created)
		s.cx.Post(func() {
			delete(s.remote.sending, l.ID)
			s.linkAnswered(l, created, err, false)
		})
	})
}

// sendOptions are the options l was chosen with that its computer q takes as it last said; all of them while it has not answered, which then refuses those it does not take. A computer that stopped taking an option since, or asks its user first now, processes the link with its own settings rather than refusing it.
func sendOptions(q Queue, opts sync.JobOptions) sync.JobOptions {
	if q.Reach != Online {
		return opts
	}
	if !q.Offers(sync.OptionQuality) {
		opts.Quality = ""
	}
	if !q.Offers(sync.OptionSubtitles) {
		opts.Subtitles = ""
	}
	return opts
}

// linkAnswered takes what l's computer answered, on its network or through its storage (relayed): taken, the link leaves the phone; refused, it stays with why; not answered, it goes again.
func (s *Session) linkAnswered(l store.Link, created sync.JobCreated, err error, relayed bool) {
	var e *sync.Error
	switch {
	case err == nil:
		s.Forget(l.ID)
		// A computer that took the link answers: one the follower last found out of reach is followed again at once rather than after its wait.
		if !relayed && s.remote.queues.GetUntracked()[l.Desktop].Reach != Online {
			s.follow(l.Desktop)
		}
		if created.Job != nil {
			s.upsertJobs(l.Desktop, []sync.Job{*created.Job})
		}
		if s.remote.events.Sent != nil {
			s.remote.events.Sent(l.Desktop, created)
		}
	case !relayed && errors.As(err, &e) && e.Status == http.StatusNotFound:
		// A computer without protocol v2's call speaks v1 alone, which takes no links, whatever its answer says.
		s.refuseWaiting(l.Desktop)
	case errors.As(err, &e) && e.Code != "" && e.Code != sync.CodeUnauthorized:
		s.noteReadOnly(l.Desktop, err)
		body := e.ErrorBody
		s.setLinks(slices.Clone(s.remote.links.GetUntracked()), func(o *Outgoing) {
			if o.ID == l.ID {
				o.Refusal, o.Relayed = &body, false
			}
		})
	case relayed:
		// A reply that does not read: the link goes again, under the same ID.
		s.setLinks(slices.Clone(s.remote.links.GetUntracked()), func(o *Outgoing) {
			if o.ID == l.ID {
				o.Relayed = false
			}
		})
	case s.remote.ctx.Err() == nil:
		s.resendLater(l)
	}
}

// resendLater sends l again after resendAfter, if it still waits and its computer answers then; one that does not is sent its waiting links by its follower as it comes back.
func (s *Session) resendLater(l store.Link) {
	s.work.Go(func() {
		select {
		case <-s.remote.ctx.Done():
			return
		case <-time.After(resendAfter):
		}
		s.cx.Post(func() {
			waiting := slices.ContainsFunc(s.remote.links.GetUntracked(), func(o Outgoing) bool { return o.ID == l.ID && o.Refusal == nil })
			q := s.remote.queues.GetUntracked()[l.Desktop]
			if waiting && q.Reach == Online && q.Can(sync.OpJobCreate) {
				s.sendLink(l)
			}
		})
	})
}

// queueFailed tells Failed of err, from an action on desktop's queue. A computer that answers the phone is read-only, as its user made it since it last said hello, is taken as read-only from then on, so the page drops the buttons it refuses.
func (s *Session) queueFailed(desktop string, err error) {
	s.noteReadOnly(desktop, err)
	if s.remote.events.Failed != nil {
		s.remote.events.Failed(desktop, err)
	}
}

// noteReadOnly takes desktop as having made the phone read-only when err is its refusal for it.
func (s *Session) noteReadOnly(desktop string, err error) {
	var e *sync.Error
	if errors.As(err, &e) && e.Code == sync.CodeReadOnly && !s.remote.queues.GetUntracked()[desktop].ReadOnly {
		s.setQueue(desktop, func(q Queue) Queue { q.ReadOnly = true; return q })
	}
}

// setQueue changes desktop's queue by change.
func (s *Session) setQueue(desktop string, change func(Queue) Queue) {
	m := maps.Clone(s.remote.queues.GetUntracked())
	m[desktop] = change(m[desktop])
	s.remote.queues.Set(m)
}

// setBusy marks key busy or not.
func (s *Session) setBusy(key string, on bool) {
	m := maps.Clone(s.remote.busy.GetUntracked())
	if on {
		m[key] = true
	} else {
		delete(m, key)
	}
	s.remote.busy.Set(m)
}

// setLinks makes list, after edit of each, the links not taken yet, and keeps them on disk as the latest last.
func (s *Session) setLinks(list []Outgoing, edit ...func(*Outgoing)) {
	for i := range list {
		for _, fn := range edit {
			fn(&list[i])
		}
	}
	s.remote.links.Set(list)
	kept := make([]store.Link, len(list))
	for i, o := range list {
		kept[i] = o.Link
	}
	r := s.remote
	r.linksMu.Lock()
	r.linksSaving++
	gen := r.linksSaving
	r.linksMu.Unlock()
	s.work.Go(func() {
		r.linksMu.Lock()
		defer r.linksMu.Unlock()
		if gen > r.linksSaved && s.st.SaveLinks(kept) == nil {
			r.linksSaved = gen
		}
	})
}
