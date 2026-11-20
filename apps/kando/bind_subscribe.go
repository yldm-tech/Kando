package main

import (
	"slices"

	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/apps/kando/subscribe"
	kcore "kando/packages/core"
)

// The sidebar's list of subscriptions and the subscription's page read and change these (subscribe.App). They read them in their bindings and call the actions from their handlers, which tell the revision (package live), so none of them calls Changed itself. The page's are of the subscription it shows.

// Subscriptions is the open library's subscriptions, none while it is locked.
func (a *app) Subscriptions() []subscribe.Item {
	if a.lockShut() {
		return nil
	}
	var items []subscribe.Item
	for _, sub := range a.subs.store.In(a.snap().LibraryDir) {
		items = append(items, subscribe.Item{ID: sub.ID, Name: sub.Name, Platform: sub.Link().Platform, Waiting: len(sub.Pending), Warn: subscribe.WarnOf(sub),
			Checking: sub.ID == a.subs.checking, Paused: !sub.Checks()})
	}
	return items
}

func (a *app) SubscriptionShown() string {
	if a.page != pageSubscription {
		return ""
	}
	return a.subs.shown
}

func (a *app) ShowSubscription(id string) { a.subscribeShow(id) }

// AskSubscription opens the dialog a link to subscribe to is pasted in, as the sidebar's + and File › 添加订阅… do.
func (a *app) AskSubscription() {
	a.subscribeAskClose()
	a.subs.ask = subscribeAsk{open: true, release: a.holdModal(), run: a.subs.ask.run}
}

// subscribeShown is the subscription whose page shows.
func (a *app) subscribeShown() (*subscribe.Sub, bool) {
	return a.subscribeFind(a.SubscriptionShown())
}

func (a *app) SubscriptionHead() subscribe.Head {
	sub, ok := a.subscribeShown()
	if !ok {
		return subscribe.Head{}
	}
	link := sub.Link()
	return subscribe.Head{ID: sub.ID, Name: sub.Name, Platform: link.Platform, Kind: subscribe.KindOf(link), Paused: sub.Paused, Checking: sub.ID == a.subs.checking,
		Gone: sub.Gone, Stopped: sub.Stopped, Renamed: sub.Renamed, TimedOut: sub.TimedOut, Err: sub.Err, Checked: sub.Checked, Cadence: sub.Cadence, Added: len(sub.Added), Earlier: a.subscribeEarlier(*sub), Note: a.subscribeNoteOf(sub.ID)}
}

// subscribeEarlier is how many videos, at most, ListEarlier would offer on sub: those its list held when it was subscribed that it left out (subscribe.Sub.Earlier), but for those its library has, which the check records as added instead.
func (a *app) subscribeEarlier(sub subscribe.Sub) int {
	n := 0
	for _, link := range sub.Earlier() {
		if !a.subscribeHas(sub.Library, link) {
			n++
		}
	}
	return n
}

// subscribeNoteOf is the note of the subscription with id as its page shows it: why something did not work is worded now, in the interface's language, which the page follows as it changes.
func (a *app) subscribeNoteOf(id string) subscribe.Note {
	n := a.subs.notes[id]
	if n.why != nil {
		n.Text = n.why(a.messages())
	}
	return n.Note
}

// SubscriptionWaiting is the new videos waiting, each with whether it fits the rules as they are now.
func (a *app) SubscriptionWaiting() []subscribe.Waiting {
	sub, ok := a.subscribeShown()
	if !ok {
		return nil
	}
	out := make([]subscribe.Waiting, len(sub.Pending))
	for i, v := range sub.Pending {
		out[i] = subscribe.Waiting{Video: v, Fits: sub.Fits(v)}
	}
	return out
}

// SubscriptionAdded is the videos the subscription added, oldest first, each with the title its job learned, or the list's, and where its job is now: one the library no longer has a job for was removed.
func (a *app) SubscriptionAdded() []subscribe.Added {
	sub, ok := a.subscribeShown()
	if !ok {
		return nil
	}
	jobs := map[string]kcore.Job{}
	for _, job := range a.library.Jobs {
		if films.InLibrary(sub.Library, job) {
			key := queueSourceKey(job.Source)
			// A job that did not fail tells more than one of the same video that did.
			if old, seen := jobs[key]; !seen || old.Stage == kcore.StageFailed {
				jobs[key] = job
			}
		}
	}
	out := make([]subscribe.Added, len(sub.Added))
	for i, v := range sub.Added {
		added := subscribe.Added{URL: v.URL, Title: v.Title, Stage: subscribe.StageGone}
		if job, ok := jobs[queueSourceKey(v.URL)]; ok {
			added.Stage = subscribeStage(job.Stage)
			if job.Title != "" {
				added.Title = job.Title
			}
		}
		if added.Title == "" {
			added.Title = v.URL
		}
		out[i] = added
	}
	return out
}

// subscribeStage is where a job at stage is, as the page says it.
func subscribeStage(stage kcore.Stage) subscribe.Stage {
	switch stage {
	case kcore.StageQueued:
		return subscribe.StageQueued
	case kcore.StageComplete:
		return subscribe.StageDone
	case kcore.StageFailed:
		return subscribe.StageFailed
	}
	return subscribe.StageRunning
}

func (a *app) SubscriptionRules() subscribe.Rules {
	if sub, ok := a.subscribeShown(); ok {
		return sub.Rules
	}
	return subscribe.DefaultRules()
}

func (a *app) SetSubscriptionRules(r subscribe.Rules) {
	sub, ok := a.subscribeShown()
	if !ok || sub.Rules == r {
		return
	}
	series := sub.Series != r.Series
	sub.Rules = r
	a.subscribeSave()
	if series {
		a.glossarySubscriptionSeries(*sub)
	}
}

// CheckSubscription checks the subscription before any other lined up, once the check that runs ends; one that is not checked, or is being checked, is left as it is. Without yt-dlp the page says it cannot be.
func (a *app) CheckSubscription() {
	sub, ok := a.subscribeShown()
	if !ok || !sub.Checks() || sub.ID == a.subs.checking {
		return
	}
	if a.subscribeLister() == nil {
		a.subs.notes[sub.ID] = subscribeNote{Note: subscribe.Note{Kind: subscribe.NoteFailed}, why: func(m locales.Messages) string { return m.SubscribeAddCannotList() }}
		return
	}
	due := []string{sub.ID}
	for _, id := range a.subs.due {
		if id != sub.ID {
			due = append(due, id)
		}
	}
	a.subs.due = due
	a.subscribeNext()
}

// ListEarlier checks the subscription shown now, as CheckSubscription does, and has that check offer the videos its list held when it was subscribed that it left out (subscribeUI.offer).
func (a *app) ListEarlier() {
	sub, ok := a.subscribeShown()
	if !ok || !sub.Checks() || sub.ID == a.subs.checking {
		return
	}
	if a.subscribeLister() != nil {
		a.subs.offer = sub.ID
	}
	a.CheckSubscription()
}

// DismissSubscriptionNote puts away the message the page of the subscription shown says about the last thing done to it.
func (a *app) DismissSubscriptionNote() {
	if sub, ok := a.subscribeShown(); ok {
		delete(a.subs.notes, sub.ID)
	}
}

// PauseSubscription stops or starts the checks; pausing stops the one that runs.
func (a *app) PauseSubscription(paused bool) {
	if sub, ok := a.subscribeShown(); ok {
		a.subscribePause(sub, paused)
	}
}

// subscribePause stops or starts the checks of sub, as its page's switch and a phone do; pausing stops the one that runs.
func (a *app) subscribePause(sub *subscribe.Sub, paused bool) {
	if sub.Paused == paused {
		return
	}
	sub.Paused = paused
	if paused && a.subs.offer == sub.ID {
		a.subs.offer = ""
	}
	if paused && sub.ID == a.subs.checking {
		a.subscribeStopCheck()
		a.subscribeNext()
	}
	a.subscribeSave()
}

// Unsubscribe removes the subscription shown and goes back to the library, whose message bar says that the videos it added stay.
func (a *app) Unsubscribe() {
	sub, ok := a.subscribeShown()
	if !ok {
		return
	}
	name := sub.Name
	a.subscribeRemove(sub.ID)
	a.Select(pageFilms)
	a.subs.shown = ""
	a.tell(a.messages().SubscribeUnsubscribed(name))
}

// subscribeRemove removes the subscription with id, stopping its check, and starts the next check lined up; the videos it added stay. A page that shows it goes back to the library after the change (subscribeFollowPage).
func (a *app) subscribeRemove(id string) {
	if id == a.subs.checking {
		a.subscribeStopCheck()
	}
	a.subs.due = slices.DeleteFunc(a.subs.due, func(x string) bool { return x == id })
	a.subs.store.Remove(id)
	delete(a.subs.notes, id)
	if a.subs.offer == id {
		a.subs.offer = ""
	}
	a.subscribeSave()
	a.subscribeNext()
}

func (a *app) KeepSubscriptionName(useNew bool) {
	sub, ok := a.subscribeShown()
	if !ok || sub.Renamed == "" {
		return
	}
	sub.KeepName(useNew)
	note := subscribe.Note{Kind: subscribe.NoteKeptName}
	if useNew {
		note = subscribe.Note{Kind: subscribe.NoteRenamed, Text: sub.Name}
	}
	a.subs.notes[sub.ID] = subscribeNote{Note: note}
	a.subscribeSave()
}

func (a *app) StopSubscription() {
	sub, ok := a.subscribeShown()
	if !ok || sub.Stopped {
		return
	}
	sub.Stopped = true
	a.subs.due = slices.DeleteFunc(a.subs.due, func(x string) bool { return x == sub.ID })
	a.subs.notes[sub.ID] = subscribeNote{Note: subscribe.Note{Kind: subscribe.NoteStopped, Text: sub.Name}}
	a.subscribeSave()
}

// AddWaiting queues the waiting videos at links, all of them for nil; those that could not be queued stay, and the page says why.
func (a *app) AddWaiting(links []string) {
	if sub, ok := a.subscribeShown(); ok {
		a.subscribeAddWaiting(sub, links)
	}
}

// subscribeAddWaiting queues the videos waiting on sub at links, all of them for nil, as its page and a phone do: those that could not be queued stay, and the page says why, which is returned, nil when every one was.
func (a *app) subscribeAddWaiting(sub *subscribe.Sub, links []string) subscribeReason {
	videos := sub.Waiting(links)
	if len(videos) == 0 {
		return nil
	}
	failed, why := a.subscribeEnqueue(sub, videos, nil)
	note := subscribeNote{Note: subscribe.Note{Kind: subscribe.NoteAdded, Count: len(videos) - len(failed)}}
	if len(failed) > 0 {
		note = subscribeNote{Note: subscribe.Note{Kind: subscribe.NoteNotQueued}, why: why}
	}
	a.subs.notes[sub.ID] = note
	a.subscribeSave()
	return note.why
}

// SkipWaiting drops the waiting videos at links, all of them for nil.
func (a *app) SkipWaiting(links []string) {
	sub, ok := a.subscribeShown()
	if !ok {
		return
	}
	if links == nil {
		for _, v := range sub.Pending {
			links = append(links, v.URL)
		}
	}
	sub.Skip(links)
	a.subscribeSave()
}
