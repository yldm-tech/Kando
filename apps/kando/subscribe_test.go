package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/playlist"
	"kando/apps/kando/subscribe"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// The channel the subscription tests follow, what it held when subscribed, and two videos it uploads later, one short.
const testSubChannel = "https://www.youtube.com/@earthscience"

var (
	testSubOld   = media.ListedVideo{URL: "https://www.youtube.com/watch?v=ooooooooooo", Title: "Old Rivers", Duration: 9 * time.Minute}
	testSubLong  = media.ListedVideo{URL: "https://www.youtube.com/watch?v=lllllllllll", Title: "How Rivers Shape Landscapes", Duration: 12 * time.Minute}
	testSubShort = media.ListedVideo{URL: "https://www.youtube.com/watch?v=sssssssssss", Title: "Tides in 40 Seconds", Duration: 40 * time.Second}
)

// subscribeTo subscribes to testSubChannel through the sidebar's + and the dialog, the channel holding testSubOld, and returns the subscription once its page shows.
func subscribeTo(t *testing.T, tt *nsgotest.App, a *app, p *listingPipeline, updates chan func()) *subscribe.Sub {
	t.Helper()
	p.lists[testSubChannel] = media.Listing{Title: "Earth Science", Videos: []media.ListedVideo{testSubOld}}
	tt.Press("id:subscribe-add")
	if !tt.Exists("id:subscribe-ask") || !a.modalOpen() {
		t.Fatalf("the dialog did not open: %q", tt.Texts())
	}
	tt.Type("id:subscribe-link", testSubChannel)
	tt.Press("id:subscribe-submit")
	if got := <-p.calls; got != (listCall{testSubChannel, playlist.ChannelLimit}) {
		t.Fatalf("listed %+v, want the channel's most recent", got)
	}
	waitUntil(t, tt, a, updates, func() bool { return !a.subs.ask.reading })
	if len(a.subs.store.Subs) != 1 {
		t.Fatalf("subscriptions %+v, dialog error %q", a.subs.store.Subs, a.subs.ask.err)
	}
	return &a.subs.store.Subs[0]
}

// checkNow presses 立即检查 on the page shown and waits for the check to report.
func checkNow(t *testing.T, tt *nsgotest.App, a *app, p *listingPipeline, updates chan func()) {
	t.Helper()
	tt.Press("id:subscription-check")
	<-p.calls
	waitUntil(t, tt, a, updates, func() bool { return a.subs.checking == "" })
}

// A channel pasted in the dialog becomes a subscription of the open library: its page shows, named after the channel, the videos it holds now are left out, and the sidebar lists it.
func TestSubscribeFromTheDialog(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{})
	a, tt, updates := newTestApp(t, p)
	msgs := testlocale.Messages()
	wantText(t, tt, msgs.SubscribeEmpty())
	sub := subscribeTo(t, tt, a, p, updates)
	if sub.Name != "Earth Science" || sub.Library != "/videos" || !slices.Equal(sub.Seen, []string{testSubOld.URL}) {
		t.Fatalf("subscription %+v", sub)
	}
	if a.page != pageSubscription || a.subs.shown != sub.ID || a.modalOpen() || tt.Exists("id:subscribe-ask") {
		t.Fatalf("page %q shown %q modal %v", a.page, a.subs.shown, a.modalOpen())
	}
	if len(a.library.Jobs) != 0 {
		t.Fatalf("the videos already there were queued: %+v", a.library.Jobs)
	}
	for _, want := range []string{"Earth Science", msgs.SubscribeNoteSubscribed("Earth Science"), msgs.SubscribeKicker("YouTube", msgs.SubscribeKindChannel()), msgs.SubscribeAddedNone()} {
		wantText(t, tt, want)
	}
	if !tt.Exists("id:subscription-"+sub.ID) || hasText(tt, msgs.SubscribeEmpty()) {
		t.Fatalf("the sidebar does not list it: %q", tt.Texts())
	}
	// The same channel pasted again shows its page rather than subscribing twice.
	tt.Press("id:subscribe-add")
	tt.Type("id:subscribe-link", "youtube.com/@EarthScience/")
	tt.Press("id:subscribe-submit")
	if len(a.subs.store.Subs) != 1 || a.subs.notes[sub.ID].Kind != subscribe.NoteAlready || tt.Exists("id:subscribe-ask") {
		t.Fatalf("subscribed again: %+v", a.subs.store.Subs)
	}
	wantText(t, tt, msgs.SubscribeNoteAlready())
}

// A link to one video is refused with why, and the dialog stays open; Cancel closes it and gives back the window's modal flag.
func TestSubscribeRefusesAVideo(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{})
	a, tt, _ := newTestApp(t, p)
	tt.Press("id:subscribe-add")
	tt.Type("id:subscribe-link", "https://www.youtube.com/watch?v=aaaaaaaaaaa")
	tt.Press("id:subscribe-submit")
	wantText(t, tt, testlocale.Messages().SubscribeAddNotList())
	if len(p.calls) != 0 || len(a.subs.store.Subs) != 0 {
		t.Fatal("a video was listed or subscribed to")
	}
	tt.Press("id:subscribe-cancel")
	if tt.Exists("id:subscribe-ask") || a.modalOpen() {
		t.Fatal("the dialog stayed")
	}
}

// A check queues the new videos that fit the rules, one job each in the subscription's library, skips the others, and the page says so and lists the videos added with where each is.
func TestSubscribeCheckQueuesNewVideos(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{})
	a, tt, updates := newTestApp(t, p)
	msgs := testlocale.Messages()
	sub := subscribeTo(t, tt, a, p, updates)
	tt.Press(whole(msgs.SubscribeFilterLong()))
	if sub.Filter != subscribe.FilterLong {
		t.Fatalf("filter %q", sub.Filter)
	}
	p.lists[testSubChannel] = media.Listing{Title: "Earth Science", Videos: []media.ListedVideo{testSubShort, testSubLong, testSubOld}}
	checkNow(t, tt, a, p, updates)
	if got := playlistJobSources(a); !slices.Equal(got, []string{testSubLong.URL}) {
		t.Fatalf("jobs %q", got)
	}
	if job := a.library.Jobs[0]; job.LibraryDir != "/videos" {
		t.Fatalf("job %+v", job)
	}
	wantText(t, tt, msgs.SubscribeNoteQueued(1)+" "+msgs.SubscribeNoteSkipped(1))
	wantText(t, tt, "How Rivers Shape Landscapes")
	if got := tt.Find("id:subscription-added-count").Text; got != "1" {
		t.Fatalf("added count %q", got)
	}
	// A check that finds nothing new says so and queues nothing again.
	checkNow(t, tt, a, p, updates)
	if len(a.library.Jobs) != 1 {
		t.Fatalf("queued again: %q", playlistJobSources(a))
	}
	wantText(t, tt, msgs.SubscribeNoteNone())
}

// A subscription that asks first lists its new videos on its page and in the sidebar's count, and queues only those added: one by one, or all at once; a video skipped is not queued and does not come back.
func TestSubscribeAsksFirst(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{})
	a, tt, updates := newTestApp(t, p)
	msgs := testlocale.Messages()
	sub := subscribeTo(t, tt, a, p, updates)
	tt.Press(whole(msgs.SubscribeModeAsk()))
	extra := media.ListedVideo{URL: "https://www.youtube.com/watch?v=eeeeeeeeeee", Title: "Erosion", Duration: 7 * time.Minute}
	p.lists[testSubChannel] = media.Listing{Title: "Earth Science", Videos: []media.ListedVideo{testSubShort, testSubLong, extra, testSubOld}}
	checkNow(t, tt, a, p, updates)
	if len(a.library.Jobs) != 0 || len(sub.Pending) != 3 {
		t.Fatalf("jobs %q, pending %+v", playlistJobSources(a), sub.Pending)
	}
	wantText(t, tt, msgs.SubscribeNoteHeld(3))
	wantText(t, tt, msgs.SubscribePendingCount(3))
	if got := tt.Find("id:subscription-waiting-" + sub.ID).Text; got != "3" {
		t.Fatalf("sidebar count %q", got)
	}
	tt.Press("id:subscription-skip-" + testSubShort.URL)
	tt.Press("id:subscription-add-" + testSubLong.URL)
	if got := playlistJobSources(a); !slices.Equal(got, []string{testSubLong.URL}) {
		t.Fatalf("jobs %q", got)
	}
	wantText(t, tt, msgs.SubscribeNoteAdded(1))
	tt.Press("id:subscription-add-all")
	if got := playlistJobSources(a); !slices.Equal(got, []string{testSubLong.URL, extra.URL}) || len(sub.Pending) != 0 {
		t.Fatalf("jobs %q, pending %+v", got, sub.Pending)
	}
	if tt.Exists("id:subscription-waiting-count") {
		t.Fatal("the list of new videos stayed")
	}
	checkNow(t, tt, a, p, updates)
	if len(sub.Pending) != 0 {
		t.Fatalf("a video skipped or added came back: %+v", sub.Pending)
	}
}

// A new name the site gives the channel shows a warning in the sidebar and a banner on its page; keeping the new name renames the subscription and the warning goes.
func TestSubscribeRename(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{})
	a, tt, updates := newTestApp(t, p)
	msgs := testlocale.Messages()
	sub := subscribeTo(t, tt, a, p, updates)
	p.lists[testSubChannel] = media.Listing{Title: "Earth Science Lab", Videos: []media.ListedVideo{testSubOld}}
	checkNow(t, tt, a, p, updates)
	if !tt.Exists("id:subscription-warn-"+sub.ID) || !tt.Exists("id:subscription-banner") {
		t.Fatalf("no warning: %q", tt.Texts())
	}
	wantText(t, tt, msgs.SubscribeRenamedBody("Earth Science Lab"))
	tt.Press("id:subscription-use-new-name")
	if sub.Name != "Earth Science Lab" || sub.Renamed != "" || tt.Exists("id:subscription-banner") || tt.Exists("id:subscription-warn-"+sub.ID) {
		t.Fatalf("after keeping the new name: %+v", sub)
	}
	wantText(t, tt, msgs.SubscribeNoteRenamed("Earth Science Lab"))
}

// A channel deleted on its site shows a banner; kept in the sidebar, it is checked no more, and the page cannot check it.
func TestSubscribeGone(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{})
	a, tt, updates := newTestApp(t, p)
	msgs := testlocale.Messages()
	sub := subscribeTo(t, tt, a, p, updates)
	delete(p.lists, testSubChannel)
	checkNow(t, tt, a, p, updates)
	if !sub.Gone || !tt.Exists("id:subscription-warn-"+sub.ID) {
		t.Fatalf("not gone: %+v", sub)
	}
	wantText(t, tt, msgs.SubscribeGoneTitle("Earth Science"))
	wantText(t, tt, msgs.SubscribeStatusGone())
	tt.Press("id:subscription-stop")
	if !sub.Stopped || tt.Exists("id:subscription-banner") || !tt.Find("id:subscription-check").Props.Disabled {
		t.Fatalf("after stopping: %+v", sub)
	}
	a.subscribeDue(true)
	if a.subs.checking != "" || len(p.calls) != 0 {
		t.Fatal("a stopped subscription was checked")
	}
}

// offlinePipeline is a listingPipeline that cannot reach the sites once down is set, as yt-dlp says when the network is down.
type offlinePipeline struct {
	*listingPipeline
	down bool
}

func (p *offlinePipeline) ListVideos(ctx context.Context, source string, limit int) (media.Listing, error) {
	if !p.down {
		return p.listingPipeline.ListVideos(ctx, source, limit)
	}
	p.calls <- listCall{source, limit}
	return media.Listing{}, errors.New("yt-dlp: exit status 1: ERROR: Unable to download webpage: <urlopen error [Errno 8] nodename nor servname provided, or not known>")
}

// A check that cannot reach the site says why and keeps the subscription as it was, to be checked again.
func TestSubscribeCheckFails(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{})
	offline := &offlinePipeline{listingPipeline: p}
	a, tt, updates := newTestApp(t, offline)
	sub := subscribeTo(t, tt, a, p, updates)
	offline.down = true
	checkNow(t, tt, a, p, updates)
	if sub.Gone || sub.Err == "" || !tt.Exists("id:subscription-error") {
		t.Fatalf("after failing: %+v, %q", sub, tt.Texts())
	}
}

// Unsubscribing removes the subscription and goes back to the library, whose message bar says the videos added stay; their jobs do.
func TestUnsubscribeKeepsTheVideos(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{})
	a, tt, updates := newTestApp(t, p)
	subscribeTo(t, tt, a, p, updates)
	p.lists[testSubChannel] = media.Listing{Title: "Earth Science", Videos: []media.ListedVideo{testSubLong, testSubOld}}
	checkNow(t, tt, a, p, updates)
	tt.Press("id:subscription-unsubscribe")
	if len(a.subs.store.Subs) != 0 || a.page != pageFilms || len(a.library.Jobs) != 1 {
		t.Fatalf("subs %+v page %q jobs %d", a.subs.store.Subs, a.page, len(a.library.Jobs))
	}
	wantText(t, tt, testlocale.Messages().SubscribeUnsubscribed("Earth Science"))
	wantText(t, tt, testlocale.Messages().SubscribeEmpty())
}

// At launch the subscriptions due are checked one at a time; the timer and the app coming to the front check those whose hour has come. A paused one is not checked.
func TestSubscribeChecksWhenDue(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{
		testSubChannel: {Title: "Earth Science", Videos: []media.ListedVideo{testSubOld}},
		testPlaylist:   testListing,
	})
	a, tt, updates := newTestApp(t, p)
	now := time.Now()
	open := subscribe.New(testSubChannel, "/videos", media.Listing{Title: "Earth Science"}, now)
	hourly := subscribe.New(testPlaylist, "/videos", media.Listing{Title: "French in Action"}, now.Add(-2*time.Hour))
	hourly.Cadence = subscribe.CadenceHourly
	paused := subscribe.New("https://www.youtube.com/@paused", "/videos", media.Listing{Title: "Paused"}, now)
	paused.Paused = true
	a.subs.store.Subs = []subscribe.Sub{open, hourly, paused}
	refresh(tt, a)
	a.subscribeDue(true)
	if got := <-p.calls; got.source != testSubChannel || a.subs.checking != open.ID || !slices.Equal(a.subs.due, []string{hourly.ID}) {
		t.Fatalf("first %+v, checking %q, due %q", got, a.subs.checking, a.subs.due)
	}
	waitUntil(t, tt, a, updates, func() bool { return a.subs.checking == hourly.ID })
	if got := <-p.calls; got.source != testPlaylist {
		t.Fatalf("second %+v", got)
	}
	waitUntil(t, tt, a, updates, func() bool { return a.subs.checking == "" })
	if got := playlistJobSources(a); !slices.Equal(got, []string{testSubOld.URL, testListing.Videos[0].URL, testListing.Videos[1].URL, testListing.Videos[2].URL}) {
		t.Fatalf("jobs %q", got)
	}
	// Later, only a subscription whose hour has come is due.
	a.subscribeDue(false)
	if a.subs.checking != "" || len(p.calls) != 0 {
		t.Fatal("checked again before its time")
	}
}

// A subscription of a library that is not open queues its videos in its own library, and the sidebar lists only the open library's.
func TestSubscribeQueuesInItsOwnLibrary(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{testSubChannel: {Title: "Earth Science", Videos: []media.ListedVideo{testSubLong}}})
	a, tt, updates := newTestApp(t, p)
	a.edit(func(s *session) { s.Libraries = []string{"/videos", "/other.kando"} })
	other := subscribe.New(testSubChannel, "/other.kando", media.Listing{Title: "Earth Science"}, time.Now())
	a.subs.store.Subs = []subscribe.Sub{other}
	refresh(tt, a)
	if tt.Exists("id:subscription-" + other.ID) {
		t.Fatal("the sidebar lists another library's subscription")
	}
	a.subscribeDue(true)
	<-p.calls
	waitUntil(t, tt, a, updates, func() bool { return a.subs.checking == "" })
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].LibraryDir != "/other.kando" || a.library.Jobs[0].Source != testSubLong.URL {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
}

// Another library opening takes the window off the page of a subscription of the library left.
func TestSubscribePageFollowsTheLibrary(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{})
	a, tt, updates := newTestApp(t, p)
	subscribeTo(t, tt, a, p, updates)
	a.edit(func(s *session) { s.LibraryDir = "/other.kando" })
	refresh(tt, a)
	if a.page != pageFilms || tt.Exists("id:subscription-page") {
		t.Fatalf("page %q", a.page)
	}
}

// The subscriptions are kept in subscriptions.json beside settings.json and read back at startup; they follow a library whose folder was renamed. A file that cannot be read is not written over.
func TestSubscriptionsAreKept(t *testing.T) {
	dir := t.TempDir()
	a := newApp(&fakePipeline{}, func(fn func()) { fn() })
	a.settingsPath = filepath.Join(dir, "settings.json")
	a.subscribeLoad()
	a.subs.store.Subs = []subscribe.Sub{subscribe.New(testSubChannel, "/videos", media.Listing{Title: "Earth Science"}, time.Now())}
	a.subscribeSave()
	a.subscribeLibraryMoved(func(path string) (string, bool) { return replacePathPrefix("/videos", "/films.kando", path) })
	b := newApp(&fakePipeline{}, func(fn func()) { fn() })
	b.settingsPath = a.settingsPath
	b.subscribeLoad()
	if len(b.subs.store.Subs) != 1 || b.subs.store.Subs[0].Name != "Earth Science" || b.subs.store.Subs[0].Library != "/films.kando" {
		t.Fatalf("read back %+v", b.subs.store.Subs)
	}
	path := filepath.Join(dir, "subscriptions.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := newApp(&fakePipeline{}, func(fn func()) { fn() })
	c.settingsPath = a.settingsPath
	c.subscribeLoad()
	if c.saveErr == "" {
		t.Fatal("a damaged file was not reported")
	}
	c.subscribeSave()
	if data, _ := os.ReadFile(path); string(data) != "{" {
		t.Fatalf("the damaged file was written over: %q", data)
	}
}
