package subscribe

import (
	"time"

	"kando/apps/kando/live"
	"kando/packages/core"
)

// App is the subscriptions the sidebar's list and the subscription's page read and change. The views read it inside their bindings through live.Func and call its actions through live.Do and live.On, so what they change shows at once (package live). The page's reads and actions are of the subscription it shows (SubscriptionShown).
type App interface {
	live.State
	// Subscriptions is the open library's subscriptions, as the sidebar lists them.
	Subscriptions() []Item
	// SubscriptionShown is the subscription whose page the window shows, "" while it shows none; ShowSubscription shows the page of the one with id.
	SubscriptionShown() string
	ShowSubscription(id string)
	// AskSubscription opens the dialog a link to subscribe to is pasted in.
	AskSubscription()
	// SubscriptionHead is what the top of the page says; SubscriptionWaiting and SubscriptionAdded are its new videos waiting for the user and the videos it added, oldest first, and SubscriptionRules its rules.
	SubscriptionHead() Head
	SubscriptionWaiting() []Waiting
	SubscriptionAdded() []Added
	SubscriptionRules() Rules
	// SetSubscriptionRules changes the rules and saves them.
	SetSubscriptionRules(r Rules)
	// CheckSubscription lists the videos now; PauseSubscription stops or starts the checks; Unsubscribe removes the subscription, keeping the videos it added.
	CheckSubscription()
	PauseSubscription(paused bool)
	Unsubscribe()
	// KeepSubscriptionName answers a rename (Sub.KeepName); StopSubscription keeps one whose list was deleted, never to check it again.
	KeepSubscriptionName(useNew bool)
	StopSubscription()
	// AddWaiting queues the waiting videos at links, SkipWaiting drops them; nil is all of them.
	AddWaiting(links []string)
	SkipWaiting(links []string)
}

// Kind is what a subscription follows, as its link says (KindOf).
type Kind string

const (
	KindChannel    Kind = "channel"
	KindSpace      Kind = "space"
	KindPlaylist   Kind = "playlist"
	KindCollection Kind = "collection"
)

// KindOf is what link follows: a channel, a bilibili uploader's space, a bilibili collection, or a playlist of any other site.
func KindOf(link core.Link) Kind {
	bilibili := link.Platform == core.PlatformBilibili
	switch {
	case link.Kind == core.LinkChannel && bilibili:
		return KindSpace
	case link.Kind == core.LinkChannel:
		return KindChannel
	case bilibili:
		return KindCollection
	}
	return KindPlaylist
}

// Warn is what is wrong with a subscription, which its sidebar row marks: its site renamed it, or deleted it.
type Warn string

const (
	WarnNone    Warn = ""
	WarnRenamed Warn = "renamed"
	WarnGone    Warn = "gone"
)

// WarnOf is s's warning: deleted before renamed, as a list that is gone has no new name to keep; one kept after it was deleted (Stopped) has none.
func WarnOf(s Sub) Warn {
	switch {
	case s.Stopped:
		return WarnNone
	case s.Gone:
		return WarnGone
	case s.Renamed != "":
		return WarnRenamed
	}
	return WarnNone
}

// Item is one row of the sidebar's list, comparable so the list is built again only when a row changed.
type Item struct {
	ID, Name string
	Platform core.Platform
	// Waiting is how many new videos wait for the user.
	Waiting int
	Warn    Warn
	// Checking is set while its videos are listed; Paused while it is not checked (Sub.Checks).
	Checking, Paused bool
}

// Head is what the top of a subscription's page says, comparable so it is read again only when it changed.
type Head struct {
	ID, Name string
	Platform core.Platform
	Kind     Kind
	Paused   bool
	Checking bool
	// Gone and Stopped are Sub's; Renamed the site's new name until the user answers; Err why the last check failed.
	Gone, Stopped bool
	Renamed       string
	Err           string
	Checked       time.Time
	Cadence       Cadence
	// Added is how many videos it added.
	Added int
	// Note is the message about the last thing done to it.
	Note Note
}

// NoteKind is which message a subscription's page shows about the last thing done to it.
type NoteKind string

const (
	NoteNone       NoteKind = ""
	NoteSubscribed NoteKind = "subscribed"
	NoteAlready    NoteKind = "already"
	// NoteChecked is a check that worked: Count new videos, queued or held as Held says, and Skipped that the rules skipped.
	NoteChecked NoteKind = "checked"
	// NoteNotQueued is new videos that could not be queued and are held instead, Text saying why.
	NoteNotQueued NoteKind = "notQueued"
	NoteAdded     NoteKind = "added"
	NoteRenamed   NoteKind = "renamed"
	NoteKeptName  NoteKind = "keptName"
	NoteStopped   NoteKind = "stopped"
	NoteGone      NoteKind = "gone"
	// NoteFailed is a check that failed, Text saying why.
	NoteFailed NoteKind = "failed"
)

// Note is the message about the last thing done to a subscription. It keeps what happened rather than the words, so the page says it in the language it shows.
type Note struct {
	Kind           NoteKind
	Count, Skipped int
	Held           bool
	Text           string
}

// Waiting is one new video waiting for the user, and whether it fits the rules as they are now.
type Waiting struct {
	Video
	Fits bool
}

// Stage is where a video a subscription added is now.
type Stage string

const (
	StageQueued  Stage = "queued"
	StageRunning Stage = "running"
	StageDone    Stage = "done"
	StageFailed  Stage = "failed"
	// StageGone is a video the library no longer has a job for, as one removed.
	StageGone Stage = "gone"
)

// Added is one video a subscription added, with the title its job learned or the list gave, and where it is now.
type Added struct {
	URL, Title string
	Stage      Stage
}
