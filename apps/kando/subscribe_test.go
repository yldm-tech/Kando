package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/playlist"
	"kando/apps/kando/remote"
	"kando/apps/kando/shelf"
	"kando/apps/kando/sidebar"
	"kando/apps/kando/subscribe"
	kcore "kando/packages/core"
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

// subscribeTo subscribes to testSubChannel through the sidebar's + and the dialog, the channel holding testSubOld, adding none of its videos now (不加), and returns the subscription once its page shows.
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
	if !tt.Exists("id:subscribe-recent") || len(a.subs.store.Subs) != 0 {
		t.Fatalf("the dialog does not ask how many to add now: %q, dialog error %q", tt.Texts(), a.subs.ask.err)
	}
	tt.Press("id:subscribe-recent-0")
	tt.Press("id:subscribe-submit")
	if len(a.subs.store.Subs) != 1 {
		t.Fatalf("subscriptions %+v, dialog error %q", a.subs.store.Subs, a.subs.ask.err)
	}
	return &a.subs.store.Subs[0]
}

// subscribeLaunch looks for due checks as the app does once its window is up (subscribeHooks' ready), the subscriptions made so far counting as made before it opened.
func subscribeLaunch(a *app) {
	a.subs.opened = time.Now()
	a.subscribeDue()
}

// nextCall is the next list the pipeline was asked for, failing the test when none is within a few seconds.
func nextCall(t *testing.T, p *listingPipeline) listCall {
	t.Helper()
	select {
	case got := <-p.calls:
		return got
	case <-time.After(5 * time.Second):
		t.Fatal("nothing was listed")
	}
	return listCall{}
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
	if sub.Name != "Earth Science" || sub.Library != testVideos || !slices.Equal(sub.Seen, []string{testSubOld.URL}) {
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

// oneVideoPipeline is a listingPipeline for which the links in one list as one video, as yt-dlp lists a bilibili upload of one part.
type oneVideoPipeline struct {
	*listingPipeline
	one map[string]bool
}

func (p *oneVideoPipeline) ListVideos(ctx context.Context, source string, limit int) (media.Listing, error) {
	if !p.one[source] {
		return p.listingPipeline.ListVideos(ctx, source, limit)
	}
	p.calls <- listCall{source, limit}
	return media.Listing{}, fmt.Errorf("yt-dlp: %w", media.ErrOneVideo)
}

// A bilibili video's page, whatever part or tracking it carries, is read as its whole upload: one of several parts becomes a subscription to that upload, whose parts now count as seen; one of a single part is refused as one video, and the dialog stays open.
func TestSubscribeToABilibiliUploadOfParts(t *testing.T) {
	const parts, single = "https://www.bilibili.com/video/BV1Eb4u6bEsd", "https://www.bilibili.com/video/BV1GJ411x7h7"
	p := newListingPipeline(map[string]media.Listing{parts: {Title: "英语动画短片", Videos: []media.ListedVideo{{URL: parts + "?p=1", Title: "p01"}, {URL: parts + "?p=2", Title: "p02"}}}})
	a, tt, updates := newTestApp(t, &oneVideoPipeline{listingPipeline: p, one: map[string]bool{single: true}})
	msgs := testlocale.Messages()

	tt.Press("id:subscribe-add")
	tt.Type("id:subscribe-link", single+"/?spm_id_from=333.1007")
	tt.Press("id:subscribe-submit")
	if got := nextCall(t, p); got.source != single {
		t.Fatalf("listed %+v, want the whole upload", got)
	}
	waitUntil(t, tt, a, updates, func() bool { return !a.subs.ask.reading })
	if len(a.subs.store.Subs) != 0 || a.subs.ask.err != msgs.SubscribeAddNotList() || !tt.Exists("id:subscribe-ask") {
		t.Fatalf("an upload of one part: subscriptions %+v, dialog error %q", a.subs.store.Subs, a.subs.ask.err)
	}

	tt.Press("id:subscribe-cancel")
	tt.Press("id:subscribe-add")
	tt.Type("id:subscribe-link", parts+"?p=3&vd_source=abc")
	tt.Press("id:subscribe-submit")
	if got := nextCall(t, p); got.source != parts {
		t.Fatalf("listed %+v, want the whole upload", got)
	}
	waitUntil(t, tt, a, updates, func() bool { return !a.subs.ask.reading })
	tt.Press("id:subscribe-submit")
	if len(a.subs.store.Subs) != 1 {
		t.Fatalf("subscriptions %+v, dialog error %q", a.subs.store.Subs, a.subs.ask.err)
	}
	// Its parts are queued, as the user subscribed for them.
	if sub := a.subs.store.Subs[0]; sub.Source != parts || sub.Name != "英语动画短片" || len(sub.Seen) != 2 || len(sub.Added) != 2 || len(a.library.Jobs) != 2 {
		t.Fatalf("subscription %+v, jobs %+v", sub, a.library.Jobs)
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
	if job := a.library.Jobs[0]; job.LibraryDir != testVideos {
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

// A subscription that adds by itself queues at most three of the new videos a check finds, the first in the list's order; the others wait on its page as if it asked first, and the page says so. Adding them from the page is not capped.
func TestSubscribeCheckQueuesAtMostThree(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{})
	a, tt, updates := newTestApp(t, p)
	msgs := testlocale.Messages()
	sub := subscribeTo(t, tt, a, p, updates)
	if sub.Mode != subscribe.ModeAuto {
		t.Fatalf("a new subscription's mode is %q, want auto", sub.Mode)
	}
	wantText(t, tt, msgs.SubscribeWhenNewAuto(subscribe.AddPerCheck))
	var fresh []media.ListedVideo
	var links []string
	for _, id := range []string{"aaaaaaaaaaa", "bbbbbbbbbbb", "ccccccccccc", "ddddddddddd", "eeeeeeeeeee"} {
		v := media.ListedVideo{URL: "https://www.youtube.com/watch?v=" + id, Title: "Video " + id[:1], Duration: 8 * time.Minute}
		fresh = append(fresh, v)
		links = append(links, v.URL)
	}
	p.lists[testSubChannel] = media.Listing{Title: "Earth Science", Videos: append(slices.Clone(fresh), testSubOld)}
	checkNow(t, tt, a, p, updates)
	if got := playlistJobSources(a); !slices.Equal(got, links[:3]) {
		t.Fatalf("jobs %q, want the first three", got)
	}
	var pending []string
	for _, v := range sub.Pending {
		pending = append(pending, v.URL)
	}
	if !slices.Equal(pending, links[3:]) || len(sub.Added) != 3 {
		t.Fatalf("pending %q, added %+v", pending, sub.Added)
	}
	wantText(t, tt, msgs.SubscribeNoteCapped(5, 3, 2))
	if got := tt.Find("id:subscription-waiting-" + sub.ID).Text; got != "2" {
		t.Fatalf("sidebar count %q", got)
	}
	tt.Press("id:subscription-add-all")
	if got := playlistJobSources(a); !slices.Equal(got, links) || len(sub.Pending) != 0 {
		t.Fatalf("jobs %q, pending %+v", got, sub.Pending)
	}
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

// A channel its site says twice in a row is not there shows a banner; once is a failed check like any other. Kept in the sidebar, it is checked no more, and the page cannot check it.
func TestSubscribeGone(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{})
	a, tt, updates := newTestApp(t, p)
	msgs := testlocale.Messages()
	sub := subscribeTo(t, tt, a, p, updates)
	delete(p.lists, testSubChannel)
	checkNow(t, tt, a, p, updates)
	if sub.Gone || tt.Exists("id:subscription-warn-"+sub.ID) || !tt.Exists("id:subscription-error") {
		t.Fatalf("gone after one answer: %+v", sub)
	}
	checkNow(t, tt, a, p, updates)
	if !sub.Gone || !tt.Exists("id:subscription-warn-"+sub.ID) {
		t.Fatalf("not gone: %+v", sub)
	}
	wantText(t, tt, msgs.SubscribeGoneTitle("channel"))
	wantText(t, tt, msgs.SubscribeStatusPair(msgs.SubscribeStatusGone("channel"), msgs.SubscribeAddedCount(len(sub.Added))))
	if !tt.Find("id:subscription-check").Props.Disabled {
		t.Fatal("立即检查 is on for a list its site deleted")
	}
	tt.Press("id:subscription-stop")
	if !sub.Stopped || tt.Exists("id:subscription-banner") || !tt.Find("id:subscription-check").Props.Disabled {
		t.Fatalf("after stopping: %+v", sub)
	}
	subscribeLaunch(a)
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
	open := subscribe.New(testSubChannel, testVideos, media.Listing{Title: "Earth Science"}, now.Add(-time.Minute))
	hourly := subscribe.New(testPlaylist, testVideos, media.Listing{Title: "French in Action"}, now.Add(-2*time.Hour))
	hourly.Cadence = subscribe.CadenceHourly
	paused := subscribe.New("https://www.youtube.com/@paused", testVideos, media.Listing{Title: "Paused"}, now)
	paused.Paused = true
	a.subs.store.Subs = []subscribe.Sub{open, hourly, paused}
	refresh(tt, a)
	subscribeLaunch(a)
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
	// Later, only a subscription whose hour has come is due, as the app finds when it comes to the front.
	a.subscribeHooks().becameActive()
	if a.subs.checking != "" || len(p.calls) != 0 {
		t.Fatal("checked again before its time")
	}
	a.subs.store.Subs[1].Checked = now.Add(-2 * time.Hour)
	a.subscribeHooks().becameActive()
	if got := nextCall(t, p); got.source != testPlaylist || a.subs.checking != hourly.ID {
		t.Fatalf("coming to the front checked %+v, checking %q", got, a.subs.checking)
	}
	waitUntil(t, tt, a, updates, func() bool { return a.subs.checking == "" })
}

// A subscription of a library that is not open queues its videos in its own library, and the sidebar lists only the open library's.
func TestSubscribeQueuesInItsOwnLibrary(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{testSubChannel: {Title: "Earth Science", Videos: []media.ListedVideo{testSubLong}}})
	a, tt, updates := newTestApp(t, p)
	a.edit(func(s *session) { s.Libraries = []string{testVideos, "/other.kando"} })
	other := subscribe.New(testSubChannel, "/other.kando", media.Listing{Title: "Earth Science"}, time.Now().Add(-time.Minute))
	a.subs.store.Subs = []subscribe.Sub{other}
	refresh(tt, a)
	if tt.Exists("id:subscription-" + other.ID) {
		t.Fatal("the sidebar lists another library's subscription")
	}
	subscribeLaunch(a)
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
	testVideosFor(t)
	dir := t.TempDir()
	a := newApp(&fakePipeline{}, func(fn func()) { fn() })
	a.settingsPath = filepath.Join(dir, "settings.json")
	a.subscribeLoad()
	a.subs.store.Subs = []subscribe.Sub{subscribe.New(testSubChannel, testVideos, media.Listing{Title: "Earth Science"}, time.Now())}
	a.subscribeSave()
	a.subscribeLibraryMoved(func(path string) (string, bool) { return replacePathPrefix(testVideos, "/films.kando", path) })
	b := newApp(&fakePipeline{}, func(fn func()) { fn() })
	b.settingsPath = a.settingsPath
	b.subscribeLoad()
	if len(b.subs.store.Subs) != 1 || b.subs.store.Subs[0].Name != "Earth Science" || b.subs.store.Subs[0].Library != filepath.Clean("/films.kando") {
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

// A subscription that checks each time the app opens, whose library is locked at launch, is passed over, and checked once its library is unlocked, not at the next launch; checked, it is not due again until then.
func TestSubscribeChecksOnceItsLibraryIsUnlocked(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{testSubChannel: {Title: "Earth Science", Videos: []media.ListedVideo{testSubLong}}})
	a, tt, updates := newTestApp(t, p)
	a.library.SetLock(testVideos, shelf.Lock{Algorithm: "test", Iterations: 1, Salt: []byte("salt"), Key: []byte("key")})
	a.subs.store.Subs = []subscribe.Sub{subscribe.New(testSubChannel, testVideos, media.Listing{Title: "Earth Science"}, time.Now().Add(-time.Minute))}
	refresh(tt, a)
	if !a.lockShut() {
		t.Fatal("the library is not locked")
	}
	subscribeLaunch(a)
	a.subscribeDue()
	refresh(tt, a)
	if a.subs.checking != "" || len(p.calls) != 0 {
		t.Fatal("a locked library's subscription was checked")
	}
	// The unlock dialog keeps the unlocked library cleaned, a native path on every OS.
	a.lock.unlocked = filepath.Clean(testVideos)
	refresh(tt, a)
	if got := nextCall(t, p); got.source != testSubChannel {
		t.Fatalf("listed %+v", got)
	}
	waitUntil(t, tt, a, updates, func() bool { return a.subs.checking == "" })
	if got := playlistJobSources(a); !slices.Equal(got, []string{testSubLong.URL}) {
		t.Fatalf("jobs %q", got)
	}
	a.subscribeDue()
	if a.subs.checking != "" || len(p.calls) != 0 {
		t.Fatal("checked again in the same launch")
	}
}

// A subscription of a library taken off the list is not checked, so yt-dlp does not run for a library the user cannot see; once the library is added back, it is checked again.
func TestSubscribeLeavesAForgottenLibraryAlone(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{testSubChannel: {Title: "Earth Science", Videos: []media.ListedVideo{testSubLong}}})
	a, tt, updates := newTestApp(t, p)
	a.edit(func(s *session) { s.Libraries = []string{testVideos, "/other.kando"} })
	a.subs.store.Subs = []subscribe.Sub{subscribe.New(testSubChannel, "/other.kando", media.Listing{Title: "Earth Science"}, time.Now().Add(-time.Minute))}
	a.forgetLibrary("/other.kando")
	refresh(tt, a)
	subscribeLaunch(a)
	if a.subs.checking != "" || len(p.calls) != 0 {
		t.Fatal("a forgotten library's subscription was checked")
	}
	a.edit(func(s *session) { s.Libraries = []string{testVideos, "/other.kando"} })
	a.subscribeDue()
	if got := nextCall(t, p); got.source != testSubChannel {
		t.Fatalf("listed %+v", got)
	}
	waitUntil(t, tt, a, updates, func() bool { return a.subs.checking == "" })
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].LibraryDir != "/other.kando" {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
}

// Pausing a subscription while it is checked stops yt-dlp, and the page cannot check it until it is resumed.
func TestSubscribePauseStopsTheCheck(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{})
	a, tt, updates := newTestApp(t, p)
	sub := subscribeTo(t, tt, a, p, updates)
	p.hold = true
	tt.Press("id:subscription-check")
	nextCall(t, p)
	if a.subs.checking != sub.ID {
		t.Fatalf("checking %q", a.subs.checking)
	}
	tt.Press("id:subscription-pause")
	if err := <-p.stopped; !errors.Is(err, context.Canceled) {
		t.Fatalf("the check stopped with %v", err)
	}
	if !sub.Paused || a.subs.checking != "" || !tt.Find("id:subscription-check").Props.Disabled {
		t.Fatalf("after pausing: %+v, checking %q", sub, a.subs.checking)
	}
	p.hold = false
	tt.Press("id:subscription-pause")
	if sub.Paused || tt.Find("id:subscription-check").Props.Disabled {
		t.Fatalf("after resuming: %+v", sub)
	}
}

// 立即检查 puts the subscription shown before the others waiting their turn; unsubscribing from the one being checked stops it, and the next one starts.
func TestSubscribeCheckNowGoesFirst(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{})
	a, tt, updates := newTestApp(t, p)
	shown := subscribeTo(t, tt, a, p, updates).ID
	first := subscribe.New(testPlaylist, testVideos, media.Listing{Title: "French in Action"}, time.Now())
	second := subscribe.New("https://www.youtube.com/@second", testVideos, media.Listing{Title: "Second"}, time.Now())
	a.subs.store.Subs = append(a.subs.store.Subs, first, second)
	p.hold = true
	a.subs.due = []string{first.ID, second.ID}
	a.subscribeNext()
	if got := nextCall(t, p); got.source != testPlaylist {
		t.Fatalf("listed %+v", got)
	}
	refresh(tt, a)
	tt.Press("id:subscription-check")
	if a.subs.checking != first.ID || !slices.Equal(a.subs.due, []string{shown, second.ID}) {
		t.Fatalf("checking %q, due %q", a.subs.checking, a.subs.due)
	}
	a.subscribeShow(first.ID)
	refresh(tt, a)
	tt.Press("id:subscription-unsubscribe")
	if err := <-p.stopped; !errors.Is(err, context.Canceled) {
		t.Fatalf("the check stopped with %v", err)
	}
	if got := nextCall(t, p); got.source != testSubChannel || a.subs.checking != shown || len(a.subs.store.Subs) != 2 {
		t.Fatalf("listed %+v, checking %q, subscriptions %d", got, a.subs.checking, len(a.subs.store.Subs))
	}
	a.subscribeStopCheck()
	<-p.stopped
}

// The rules are changed on the page: keywords typed in their field pick the new videos a check takes, and the cadence chosen is kept.
func TestSubscribeRulesOnThePage(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{})
	a, tt, updates := newTestApp(t, p)
	msgs := testlocale.Messages()
	sub := subscribeTo(t, tt, a, p, updates)
	tt.Press(whole(msgs.SubscribeFilterKeywords()))
	tt.Type("id:subscription-keywords", "rivers，潮汐")
	tt.Press(whole(msgs.SubscribeCadenceDaily()))
	if sub.Filter != subscribe.FilterKeywords || sub.Keywords != "rivers，潮汐" || sub.Cadence != subscribe.CadenceDaily {
		t.Fatalf("rules %+v", sub.Rules)
	}
	p.lists[testSubChannel] = media.Listing{Title: "Earth Science", Videos: []media.ListedVideo{testSubShort, testSubLong, testSubOld}}
	checkNow(t, tt, a, p, updates)
	if got := playlistJobSources(a); !slices.Equal(got, []string{testSubLong.URL}) {
		t.Fatalf("jobs %q", got)
	}
}

// timeoutPipeline is a listingPipeline whose yt-dlp takes too long once slow is set.
type timeoutPipeline struct {
	*listingPipeline
	slow bool
}

func (p *timeoutPipeline) ListVideos(ctx context.Context, source string, limit int) (media.Listing, error) {
	if !p.slow {
		return p.listingPipeline.ListVideos(ctx, source, limit)
	}
	p.calls <- listCall{source, limit}
	return media.Listing{}, media.ErrListTimeout
}

// Why a check failed is kept as data, not in the words of the language shown then, so the page says it in the language it shows after a switch.
func TestSubscribeFailureFollowsTheLanguage(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{})
	slow := &timeoutPipeline{listingPipeline: p}
	a, tt, updates := newTestApp(t, slow)
	sub := subscribeTo(t, tt, a, p, updates)
	slow.slow = true
	checkNow(t, tt, a, p, updates)
	if !sub.TimedOut || sub.Err != "" {
		t.Fatalf("kept %+v", sub)
	}
	m := testlocale.Messages()
	wantText(t, tt, m.SubscribeStatusFailed(m.PlaylistTimedOut()))
	wantText(t, tt, m.SubscribeNoteFailed(m.PlaylistTimedOut()))
	onEngine(a, func() { a.SetInterfaceLanguage("en") })
	refresh(tt, a)
	en := locales.In("en")
	wantText(t, tt, en.SubscribeStatusFailed(en.PlaylistTimedOut()))
	wantText(t, tt, en.SubscribeNoteFailed(en.PlaylistTimedOut()))
}

// New videos of a subscription are held, with why, when a tool a job needs is missing, whichever library is open: a locked one, or none, which is not a reason of the subscription's own library.
func TestSubscribeEnqueueAsksForTheTools(t *testing.T) {
	testVideosFor(t)
	a := newApp(media.Tools{YTDLP: "yt-dlp", FFmpeg: "ffmpeg"}, func(fn func()) { fn() })
	a.prefs.language = "简体中文"
	a.library.SetLock(testVideos, shelf.Lock{Algorithm: "test", Iterations: 1, Salt: []byte("salt"), Key: []byte("key")})
	m := testlocale.Messages()
	want := m.PrecheckToolsMissing("whisper-cli")
	for _, open := range []string{testVideos, ""} {
		a.edit(func(s *session) { s.Libraries, s.LibraryDir = []string{testVideos, "/other.kando"}, open })
		sub := subscribe.New(testSubChannel, "/other.kando", media.Listing{Title: "Earth Science"}, time.Now())
		failed, why := a.subscribeEnqueue(&sub, []subscribe.Video{{URL: testSubLong.URL}}, nil)
		said := ""
		if why != nil {
			said = why(m)
		}
		if len(failed) != 1 || said != want || len(a.library.Jobs) != 0 {
			t.Fatalf("open %q: failed %d, why %q, jobs %d", open, len(failed), said, len(a.library.Jobs))
		}
	}
}

// The control socket lists the open library's subscriptions, subscribes it to a list without leaving the page the window shows, and checks one now, queueing at most three of its new videos and holding the rest as the window's check does.
func TestRemoteSubscriptions(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{testSubChannel: {Title: "Earth Science", Videos: []media.ListedVideo{testSubOld}}})
	a, _, updates := newTestApp(t, p)
	remoteServe(t, a)
	subs, err := remoteCall[[]remote.Subscription](t, updates, remote.MethodSubscriptions, nil)
	if err != nil || subs == nil || len(subs) != 0 {
		t.Fatalf("subscriptions %+v, %v", subs, err)
	}
	_, err = remoteCall[remote.Subscribed](t, updates, remote.MethodSubscribe, remote.SubscribeParams{Source: testSubOld.URL})
	remoteWantCode(t, err, remote.CodeInvalid)

	page := a.page
	got, err := remoteCall[remote.Subscribed](t, updates, remote.MethodSubscribe, remote.SubscribeParams{Source: testSubChannel})
	if err != nil {
		t.Fatal(err)
	}
	sub := got.Subscription
	if got.Already || sub.ID == "" || sub.Name != "Earth Science" || sub.Kind != "channel" || sub.Platform != "youtube" || sub.Mode != "auto" || sub.AddPerCheck != subscribe.AddPerCheck || len(sub.Waiting) != 0 || sub.Added != 0 {
		t.Fatalf("subscribed %+v", got)
	}
	if nextCall(t, p) != (listCall{testSubChannel, playlist.ChannelLimit}) || len(a.subs.store.Subs) != 1 || a.page != page {
		t.Fatalf("subscriptions %+v, page %q", a.subs.store.Subs, a.page)
	}
	again, err := remoteCall[remote.Subscribed](t, updates, remote.MethodSubscribe, remote.SubscribeParams{Source: "youtube.com/@EarthScience/"})
	if err != nil || !again.Already || again.Subscription.ID != sub.ID || len(p.calls) != 0 {
		t.Fatalf("subscribed again %+v, %v", again, err)
	}

	var fresh []media.ListedVideo
	for _, id := range []string{"aaaaaaaaaaa", "bbbbbbbbbbb", "ccccccccccc", "ddddddddddd", "eeeeeeeeeee"} {
		fresh = append(fresh, media.ListedVideo{URL: "https://www.youtube.com/watch?v=" + id, Title: "Video " + id[:1], Duration: 8 * time.Minute})
	}
	p.lists[testSubChannel] = media.Listing{Title: "Earth Science", Videos: append(append(slices.Clone(fresh), testSubShort), testSubOld)}
	checked, err := remoteCall[remote.SubscriptionChecked](t, updates, remote.MethodSubscriptionCheck, remote.IDParams{ID: sub.ID})
	if err != nil {
		t.Fatal(err)
	}
	nextCall(t, p)
	if checked.New != 6 || checked.Queued != 3 || checked.Held != 3 || checked.Skipped != 0 || len(checked.Subscription.Waiting) != 3 || checked.Subscription.Added != 3 {
		t.Fatalf("checked %+v", checked)
	}
	if got := playlistJobSources(a); len(got) != 3 || got[0] != fresh[0].URL {
		t.Fatalf("jobs %q", got)
	}
	if a.subs.notes[sub.ID].Over != 3 {
		t.Fatalf("note %+v", a.subs.notes[sub.ID])
	}
	subs, err = remoteCall[[]remote.Subscription](t, updates, remote.MethodSubscriptions, nil)
	if err != nil || len(subs) != 1 || subs[0].Added != 3 || len(subs[0].Waiting) != 3 || subs[0].Waiting[0].URL != fresh[3].URL || subs[0].Checked.IsZero() {
		t.Fatalf("subscriptions %+v, %v", subs, err)
	}

	_, err = remoteCall[remote.SubscriptionChecked](t, updates, remote.MethodSubscriptionCheck, remote.IDParams{ID: "nope"})
	remoteWantCode(t, err, remote.CodeNotFound)
	// A list yt-dlp cannot read fails with why, which the subscription keeps.
	delete(p.lists, testSubChannel)
	_, err = remoteCall[remote.SubscriptionChecked](t, updates, remote.MethodSubscriptionCheck, remote.IDParams{ID: sub.ID})
	remoteWantCode(t, err, remote.CodeFailed)
	nextCall(t, p)
	if s := a.subs.store.Subs[0]; s.Err == "" || s.Missing != 1 {
		t.Fatalf("subscription after a failed check %+v", s)
	}
	a.subs.store.Subs[0].Paused = true
	_, err = remoteCall[remote.SubscriptionChecked](t, updates, remote.MethodSubscriptionCheck, remote.IDParams{ID: sub.ID})
	remoteWantCode(t, err, remote.CodeNotReady)
	if len(p.calls) != 0 {
		t.Fatal("a paused subscription was listed")
	}
}

// A channel the control socket subscribes the library to while the dialog reads the same one is not subscribed to twice: once the dialog's read lands, the dialog closes on the subscription the socket made.
func TestSubscribeDialogAfterTheSocket(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{testSubChannel: {Title: "Earth Science", Videos: []media.ListedVideo{testSubOld}}})
	a, tt, updates := newTestApp(t, p)
	tt.Press("id:subscribe-add")
	tt.Type("id:subscribe-link", testSubChannel)
	tt.Press("id:subscribe-submit")
	nextCall(t, p)
	// What remoteSubscribe's last step does, landing before the dialog's read.
	onEngine(a, func() {
		sub := subscribe.New(testSubChannel, testVideos, media.Listing{Title: "Earth Science", Videos: []media.ListedVideo{testSubOld}}, time.Now())
		a.subs.store.Subs = append(a.subs.store.Subs, sub)
		a.Changed()
	})
	waitUntil(t, tt, a, updates, func() bool { return !a.subs.ask.reading })
	if len(a.subs.store.Subs) != 1 {
		t.Fatalf("subscribed twice: %+v", a.subs.store.Subs)
	}
	id := a.subs.store.Subs[0].ID
	if tt.Exists("id:subscribe-ask") || a.modalOpen() || a.page != pageSubscription || a.subs.shown != id || a.subs.notes[id].Kind != subscribe.NoteAlready {
		t.Fatalf("page %q shown %q note %+v modal %v", a.page, a.subs.shown, a.subs.notes[id], a.modalOpen())
	}
	wantText(t, tt, testlocale.Messages().SubscribeNoteAlready())
}

// A library locked while yt-dlp reads a list for the control socket is not changed and nothing of it is answered: subscribing and checking both say it is locked.
func TestRemoteSubscriptionsLockedWhileListing(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{testSubChannel: {Title: "Earth Science", Videos: []media.ListedVideo{testSubOld}}})
	a, _, updates := newTestApp(t, p)
	a.library.SetLock(testVideos, shelf.Lock{Algorithm: "test", Iterations: 1, Salt: []byte("salt"), Key: []byte("key")})
	a.lock.unlocked = filepath.Clean(testVideos)
	remoteServe(t, a)
	lockWhileListing := func() { p.listed = func() { a.update(func() { a.lock.unlocked = "" }) } }

	lockWhileListing()
	_, err := remoteCall[remote.Subscribed](t, updates, remote.MethodSubscribe, remote.SubscribeParams{Source: testSubChannel})
	remoteWantCode(t, err, remote.CodeNotReady)
	nextCall(t, p)
	if len(a.subs.store.Subs) != 0 {
		t.Fatalf("a locked library was subscribed: %+v", a.subs.store.Subs)
	}

	a.lock.unlocked = filepath.Clean(testVideos)
	p.listed = nil
	got, err := remoteCall[remote.Subscribed](t, updates, remote.MethodSubscribe, remote.SubscribeParams{Source: testSubChannel})
	if err != nil {
		t.Fatal(err)
	}
	nextCall(t, p)
	p.lists[testSubChannel] = media.Listing{Title: "Earth Science", Videos: []media.ListedVideo{testSubLong, testSubOld}}
	lockWhileListing()
	_, err = remoteCall[remote.SubscriptionChecked](t, updates, remote.MethodSubscriptionCheck, remote.IDParams{ID: got.Subscription.ID})
	remoteWantCode(t, err, remote.CodeNotReady)
	nextCall(t, p)
	if s := a.subs.store.Subs[0]; slices.Contains(s.Seen, testSubLong.URL) || len(s.Pending) != 0 || len(s.Added) != 0 || len(a.library.Jobs) != 0 {
		t.Fatalf("a locked library's subscription was checked: %+v, jobs %+v", s, a.library.Jobs)
	}
}

// A check over the cap whose videos cannot be queued, as no language is chosen, holds them all in the list's order, those it tried first and those past the cap after, and the control socket says why with the counts.
func TestRemoteSubscriptionCheckNotQueuedOverTheCap(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{testSubChannel: {Title: "Earth Science", Videos: []media.ListedVideo{testSubOld}}})
	a, _, updates := newTestApp(t, p)
	remoteServe(t, a)
	got, err := remoteCall[remote.Subscribed](t, updates, remote.MethodSubscribe, remote.SubscribeParams{Source: testSubChannel})
	if err != nil {
		t.Fatal(err)
	}
	nextCall(t, p)
	var fresh []media.ListedVideo
	var links []string
	for _, id := range []string{"aaaaaaaaaaa", "bbbbbbbbbbb", "ccccccccccc", "ddddddddddd", "eeeeeeeeeee"} {
		v := media.ListedVideo{URL: "https://www.youtube.com/watch?v=" + id, Title: "Video " + id[:1], Duration: 8 * time.Minute}
		fresh = append(fresh, v)
		links = append(links, v.URL)
	}
	p.lists[testSubChannel] = media.Listing{Title: "Earth Science", Videos: append(slices.Clone(fresh), testSubOld)}
	a.prefs.language = ""
	checked, err := remoteCall[remote.SubscriptionChecked](t, updates, remote.MethodSubscriptionCheck, remote.IDParams{ID: got.Subscription.ID})
	if err != nil {
		t.Fatal(err)
	}
	nextCall(t, p)
	if checked.New != 5 || checked.Queued != 0 || checked.Held != 5 || checked.Subscription.Added != 0 || checked.Note != remoteMessages().QueueNoLanguage() {
		t.Fatalf("checked %+v", checked)
	}
	var waiting []string
	for _, v := range checked.Subscription.Waiting {
		waiting = append(waiting, v.URL)
	}
	if !slices.Equal(waiting, links) || len(a.library.Jobs) != 0 {
		t.Fatalf("waiting %q, jobs %+v", waiting, a.library.Jobs)
	}
	if note := a.subs.notes[got.Subscription.ID]; note.Kind != subscribe.NoteNotQueued || note.Count != 5 || note.Over != 2 {
		t.Fatalf("note %+v", note)
	}
}

// The bilibili collection the collection tests follow, and its three episodes.
const testSubCollection = "https://space.bilibili.com/12345/channel/collectiondetail?sid=67890"

var testSubEpisodes = media.Listing{Title: "线性代数的几何意义", Videos: []media.ListedVideo{
	{URL: "https://www.bilibili.com/video/BV1xx411c7m1", Title: "P1 向量", Duration: 12 * time.Minute},
	{URL: "https://www.bilibili.com/video/BV1xx411c7m2", Title: "P2 向量与基", Duration: 11 * time.Minute},
	{URL: "https://www.bilibili.com/video/BV1xx411c7m3", Title: "P3 线性变换", Duration: 14 * time.Minute},
}}

// subscribeRead pastes source in the subscribe dialog and waits for the card that shows what was read.
func subscribeRead(t *testing.T, tt *nsgotest.App, a *app, p *listingPipeline, updates chan func(), source string) {
	t.Helper()
	tt.Press("id:subscribe-add")
	tt.Type("id:subscribe-link", source)
	tt.Press("id:subscribe-submit")
	nextCall(t, p)
	waitUntil(t, tt, a, updates, func() bool { return !a.subs.ask.reading })
	if !a.subs.ask.read || len(a.subs.store.Subs) != 0 {
		t.Fatalf("the dialog did not show what it read: %q, error %q", tt.Texts(), a.subs.ask.err)
	}
}

// Subscribing to a collection from the dialog queues all its episodes, in the list's order, as one series: the card says how many it holds, the button that it adds them, and the page lists them as added.
func TestSubscribeToACollectionAddsItsEpisodes(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{testSubCollection: testSubEpisodes})
	a, tt, updates := newTestApp(t, p)
	msgs := testlocale.Messages()
	subscribeRead(t, tt, a, p, updates, testSubCollection)
	for _, want := range []string{"线性代数的几何意义", msgs.SubscribeAddWholeCollection(3), msgs.SubscribeAddConfirmList(3)} {
		wantText(t, tt, want)
	}
	if tt.Exists("id:subscribe-recent") || !tt.Exists("id:subscribe-series") {
		t.Fatalf("a collection's card: %q", tt.Texts())
	}
	tt.Press("id:subscribe-submit")
	if len(a.subs.store.Subs) != 1 || tt.Exists("id:subscribe-ask") {
		t.Fatalf("subscriptions %+v, error %q", a.subs.store.Subs, a.subs.ask.err)
	}
	sub := a.subs.store.Subs[0]
	want := []string{testSubEpisodes.Videos[0].URL, testSubEpisodes.Videos[1].URL, testSubEpisodes.Videos[2].URL}
	if got := playlistJobSources(a); !slices.Equal(got, want) {
		t.Fatalf("queued %q, want the episodes in order", got)
	}
	if !sub.Series || len(sub.Added) != 3 || sub.Added[0].Title != "P1 向量" || len(sub.Pending) != 0 || !sub.Offered {
		t.Fatalf("subscription %+v", sub)
	}
	for _, job := range a.library.Jobs {
		if job.Series != glossarySeriesOf(sub) {
			t.Fatalf("job %q is not in the subscription's series: %q", job.Source, job.Series)
		}
	}
	refresh(tt, a)
	wantText(t, tt, msgs.SubscribeNoteSubscribedAdded("线性代数的几何意义", 3))
	if hasText(tt, msgs.SubscribeAddedNone()) {
		t.Fatalf("the page lists none added: %q", tt.Texts())
	}
}

// In a window shorter than the collection's card, the card scrolls inside the dialog and its buttons stay on screen, so it can still be subscribed to.
func TestSubscribeCardFitsAShortWindow(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{testSubCollection: testSubEpisodes})
	a, tt, updates := newTestApp(t, p, nsgotest.Size(1024, 560))
	subscribeRead(t, tt, a, p, updates, testSubCollection)
	if !tt.Exists("id:subscribe-submit") || !tt.Exists("id:subscribe-cancel") {
		t.Fatalf("the buttons are off screen: %q", tt.Texts())
	}
	tt.Press("id:subscribe-submit")
	if len(a.subs.store.Subs) != 1 || len(a.library.Jobs) != 3 {
		t.Fatalf("subscriptions %+v, jobs %d", a.subs.store.Subs, len(a.library.Jobs))
	}
}

// Subscribing to a channel asks how many of its videos to add now: its latest 3 unless the user chooses otherwise, the rest counting as seen.
func TestSubscribeToAChannelAddsItsLatest(t *testing.T) {
	var videos []media.ListedVideo
	for _, id := range []string{"aaaaaaaaaaa", "bbbbbbbbbbb", "ccccccccccc", "ddddddddddd", "eeeeeeeeeee"} {
		videos = append(videos, media.ListedVideo{URL: "https://www.youtube.com/watch?v=" + id, Title: "Video " + id[:1], Duration: 8 * time.Minute})
	}
	p := newListingPipeline(map[string]media.Listing{testSubChannel: {Title: "Earth Science", Videos: videos}})
	a, tt, updates := newTestApp(t, p)
	msgs := testlocale.Messages()
	subscribeRead(t, tt, a, p, updates, testSubChannel)
	if a.subs.ask.choice.Recent != subscribe.RecentDefault || !tt.Exists("id:subscribe-recent") || tt.Exists("id:subscribe-series") {
		t.Fatalf("a channel's card: recent %d, %q", a.subs.ask.choice.Recent, tt.Texts())
	}
	wantText(t, tt, msgs.SubscribeAddConfirmChannel(3))
	tt.Press("id:subscribe-submit")
	if len(a.subs.store.Subs) != 1 {
		t.Fatalf("subscriptions %+v, error %q", a.subs.store.Subs, a.subs.ask.err)
	}
	sub := a.subs.store.Subs[0]
	if got := playlistJobSources(a); !slices.Equal(got, []string{videos[0].URL, videos[1].URL, videos[2].URL}) {
		t.Fatalf("queued %q, want the latest 3", got)
	}
	if len(sub.Seen) != 5 || len(sub.Added) != 3 || sub.Series {
		t.Fatalf("subscription %+v", sub)
	}
}

// Episodes that cannot be queued as the collection is subscribed to, as while no language to translate into is chosen, wait on the page, which says why.
func TestSubscribeHoldsEpisodesItCannotQueue(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{testSubCollection: testSubEpisodes})
	a, tt, updates := newTestApp(t, p)
	a.prefs.language = ""
	subscribeRead(t, tt, a, p, updates, testSubCollection)
	tt.Press("id:subscribe-submit")
	if len(a.subs.store.Subs) != 1 {
		t.Fatalf("subscriptions %+v, error %q", a.subs.store.Subs, a.subs.ask.err)
	}
	sub := a.subs.store.Subs[0]
	if len(a.library.Jobs) != 0 || len(sub.Pending) != 3 || sub.Pending[0].URL != testSubEpisodes.Videos[0].URL || len(sub.Added) != 0 {
		t.Fatalf("subscription %+v, jobs %+v", sub, a.library.Jobs)
	}
	refresh(tt, a)
	wantText(t, tt, testlocale.Messages().SubscribeNoteNotQueued(testlocale.Messages().QueueNoLanguage()))
	if !tt.Exists("id:subscription-waiting-count") {
		t.Fatalf("the page does not list them: %q", tt.Texts())
	}
}

// A collection subscribed to before Kando added a list's episodes counted them as seen and added none: its page offers them, 列出来 reads the list again, and they wait to be added, even for a subscription that adds new videos by itself; 全部加入 then queues them in order.
func TestSubscribeOffersTheEpisodesAnOldSubscriptionLeftOut(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{testSubCollection: testSubEpisodes})
	a, tt, updates := newTestApp(t, p)
	msgs := testlocale.Messages()
	old := subscribe.New(testSubCollection, testVideos, testSubEpisodes, time.Now().Add(-time.Hour))
	old.Offered = false
	a.subs.store.Subs = []subscribe.Sub{old}
	refresh(tt, a)
	tt.Press("id:subscription-" + old.ID)
	if !tt.Exists("id:subscription-earlier") {
		t.Fatalf("the page does not offer the episodes: %q", tt.Texts())
	}
	wantText(t, tt, msgs.SubscribeEarlierCount(3))
	tt.Press("id:subscription-earlier-list")
	nextCall(t, p)
	waitUntil(t, tt, a, updates, func() bool { return a.subs.checking == "" })
	sub := &a.subs.store.Subs[0]
	if len(sub.Pending) != 3 || sub.Pending[2].Title != "P3 线性变换" || len(a.library.Jobs) != 0 || !sub.Offered || tt.Exists("id:subscription-earlier") {
		t.Fatalf("subscription %+v, jobs %+v", sub, a.library.Jobs)
	}
	wantText(t, tt, msgs.SubscribeNoteNone()+" "+msgs.SubscribeNoteEarlier(3))
	tt.Press("id:subscription-add-all")
	want := []string{testSubEpisodes.Videos[0].URL, testSubEpisodes.Videos[1].URL, testSubEpisodes.Videos[2].URL}
	if got := playlistJobSources(a); !slices.Equal(got, want) || len(sub.Pending) != 0 || len(sub.Added) != 3 {
		t.Fatalf("queued %q, subscription %+v", got, sub)
	}
}

// The control socket, and so a phone, subscribing to a collection queues its episodes in order, as the window's dialog does; a channel's videos there now count as seen, as neither says how many it wants.
func TestRemoteSubscribeAddsACollectionsEpisodes(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{testSubCollection: testSubEpisodes})
	a, _, updates := newTestApp(t, p)
	remoteServe(t, a)
	got, err := remoteCall[remote.Subscribed](t, updates, remote.MethodSubscribe, remote.SubscribeParams{Source: testSubCollection})
	if err != nil {
		t.Fatal(err)
	}
	nextCall(t, p)
	want := []string{testSubEpisodes.Videos[0].URL, testSubEpisodes.Videos[1].URL, testSubEpisodes.Videos[2].URL}
	if got.Subscription.Added != 3 || !slices.Equal(playlistJobSources(a), want) {
		t.Fatalf("subscribed %+v, queued %q", got, playlistJobSources(a))
	}
}

// The library locked, or another one opened, while the dialog's card shows is refused with why: nothing is subscribed to or queued.
func TestSubscribeDialogRefusesALibraryLockedOrChangedMeanwhile(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{testSubCollection: testSubEpisodes})
	a, tt, updates := newTestApp(t, p)
	msgs := testlocale.Messages()
	a.library.SetLock(testVideos, shelf.Lock{Algorithm: "test", Iterations: 1, Salt: []byte("salt"), Key: []byte("key")})
	a.lock.unlocked = filepath.Clean(testVideos)
	subscribeRead(t, tt, a, p, updates, testSubCollection)
	onEngine(a, func() { a.lock.unlocked = "" })
	refresh(tt, a)
	tt.Press("id:subscribe-submit")
	if len(a.subs.store.Subs) != 0 || len(a.library.Jobs) != 0 || a.subs.ask.err != msgs.LockLibraryIsLocked(sidebar.Title(testVideos)) {
		t.Fatalf("subscriptions %+v, jobs %d, error %q", a.subs.store.Subs, len(a.library.Jobs), a.subs.ask.err)
	}
	onEngine(a, func() {
		a.lock.unlocked = filepath.Clean(testVideos)
		a.edit(func(s *session) { s.Libraries, s.LibraryDir = []string{testVideos, "/other.kando"}, "/other.kando" })
	})
	// The window under the dialog changes as another library opens, so its button's action is run directly.
	onEngine(a, a.subscribeAskSubmit)
	if len(a.subs.store.Subs) != 0 || len(a.library.Jobs) != 0 || a.subs.ask.err != msgs.PrecheckNoLibrary() {
		t.Fatalf("subscriptions %+v, jobs %d, error %q", a.subs.store.Subs, len(a.library.Jobs), a.subs.ask.err)
	}
}

// The card's choices go on the subscription and its videos: the picture and the source of lines become its rules, which the videos it adds later take too, and the sign-in and the chapters it chose, which differ from the settings', go on the videos it adds now.
func TestSubscribeDialogChoicesGoOnTheJobs(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{testSubCollection: testSubEpisodes})
	a, tt, updates := newTestApp(t, p)
	withBrowsers(t, a, "chrome", "firefox")
	subscribeRead(t, tt, a, p, updates, testSubCollection)
	if tt.Exists("id:subscribe-only") {
		t.Fatal("a link to the list offers only one episode")
	}
	if c := a.subs.ask.choice; c.Browser != "" || !c.Chapters {
		t.Fatalf("the card does not start from the settings: %+v", c)
	}
	tt.Press("id:quality-720")
	tt.Press("id:subscribe-source-local")
	firefox := slices.IndexFunc(a.SignIn().Browsers, func(b media.Browser) bool { return b.ID == "firefox" })
	tt.Press("id:subscribe-signin")
	tt.Press(fmt.Sprintf("id:subscribe-signin-%d", firefox+1))
	tt.Press("id:subscribe-chapters")
	if c := a.subs.ask.choice; c.Browser != "firefox" || c.Chapters {
		t.Fatalf("the card's choice %+v", c)
	}
	tt.Press("id:subscribe-submit")
	if len(a.subs.store.Subs) != 1 || len(a.library.Jobs) != 3 {
		t.Fatalf("subscriptions %+v, jobs %d", a.subs.store.Subs, len(a.library.Jobs))
	}
	sub := &a.subs.store.Subs[0]
	if sub.Quality != kcore.Quality720 || sub.Subtitles != kcore.SubtitlesLocal {
		t.Fatalf("rules %+v", sub.Rules)
	}
	for _, job := range a.library.Jobs {
		if job.Quality != kcore.Quality720 || job.SubtitleSource != kcore.SubtitlesLocal || job.Cookies != "firefox" || job.ChapterUse != kcore.ChaptersEven {
			t.Fatalf("job %q: quality %q, source %q, cookies %q, chapters %q", job.Source, job.Quality, job.SubtitleSource, job.Cookies, job.ChapterUse)
		}
	}
	later := subscribe.Video{URL: "https://www.bilibili.com/video/BV1xx411c7m4", Title: "P4 矩阵乘法"}
	onEngine(a, func() { a.subscribeEnqueue(sub, []subscribe.Video{later}, nil) })
	if job, ok := a.queueFindSource(later.URL); !ok || job.Quality != kcore.Quality720 || job.SubtitleSource != kcore.SubtitlesLocal {
		t.Fatalf("a later video's job %+v", job)
	}
}

// The card's 未登录 makes the videos it adds now borrow no sign-in when the settings name a browser, and chapters it leaves as the settings have them are not written on the jobs.
func TestSubscribeDialogSignInNoneGoesOnTheJobs(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{testSubCollection: testSubEpisodes})
	a, tt, updates := newTestApp(t, p)
	withBrowsers(t, a, "chrome")
	a.signin.browser = "chrome"
	subscribeRead(t, tt, a, p, updates, testSubCollection)
	if c := a.subs.ask.choice; c.Browser != "chrome" {
		t.Fatalf("the card does not start from the settings: %+v", c)
	}
	tt.Press("id:subscribe-signin")
	tt.Press("id:subscribe-signin-0")
	tt.Press("id:subscribe-submit")
	if len(a.library.Jobs) != 3 {
		t.Fatalf("jobs %d", len(a.library.Jobs))
	}
	for _, job := range a.library.Jobs {
		if job.Cookies != kcore.CookiesNone || job.ChapterUse != "" {
			t.Fatalf("job %q: cookies %q, chapters %q", job.Source, job.Cookies, job.ChapterUse)
		}
	}
}

// The page's 下载 and 优先用平台字幕 change the subscription's rules, which the videos it adds next take.
func TestSubscriptionPageQualityAndSubtitlesGoOnTheNextJobs(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{testSubCollection: testSubEpisodes})
	a, tt, updates := newTestApp(t, p)
	subscribeRead(t, tt, a, p, updates, testSubCollection)
	tt.Press("id:subscribe-submit")
	if len(a.subs.store.Subs) != 1 {
		t.Fatalf("subscriptions %+v", a.subs.store.Subs)
	}
	sub := &a.subs.store.Subs[0]
	platform := a.PlatformSubtitles()
	tt.Press("id:quality-720")
	tt.Press("id:subscription-platform-subtitles")
	want := kcore.SubtitlesPlatform
	if platform {
		want = kcore.SubtitlesLocal
	}
	if sub.Quality != kcore.Quality720 || sub.Subtitles != want {
		t.Fatalf("rules %+v, want 720p and %q", sub.Rules, want)
	}
	later := subscribe.Video{URL: "https://www.bilibili.com/video/BV1xx411c7m4", Title: "P4 矩阵乘法"}
	onEngine(a, func() { a.subscribeEnqueue(sub, []subscribe.Video{later}, nil) })
	if job, ok := a.queueFindSource(later.URL); !ok || job.Quality != kcore.Quality720 || job.SubtitleSource != want {
		t.Fatalf("the next video's job %+v", job)
	}
}

// A link to one episode of a collection offers to add only it; with 只要这一集, the subscription adds that one now and counts the others as seen.
func TestSubscribeDialogAddsOnlyTheEpisodeTheLinkNamed(t *testing.T) {
	episode := testSubEpisodes.Videos[1].URL
	p := newListingPipeline(map[string]media.Listing{testSubCollection: testSubEpisodes})
	a, tt, updates := newTestApp(t, p)
	onEngine(a, func() {
		a.subs.ask = subscribeAsk{open: true, release: a.holdModal(), read: true, source: testSubCollection, dir: testVideos, listing: testSubEpisodes,
			episode: subscribeAskEpisode(episode, testSubEpisodes), choice: subscribe.Choice{Subscribe: true, Series: true}}
	})
	refresh(tt, a)
	_ = updates
	if !tt.Exists("id:subscribe-only") {
		t.Fatalf("no choice of the one episode: %q", tt.Texts())
	}
	tt.Press("id:subscribe-only-this")
	wantText(t, tt, testlocale.Messages().SubscribeAddConfirmList(1))
	tt.Press("id:subscribe-submit")
	if got := playlistJobSources(a); !slices.Equal(got, []string{episode}) || len(a.subs.store.Subs) != 1 || len(a.subs.store.Subs[0].Seen) != 3 {
		t.Fatalf("queued %q, subscriptions %+v", got, a.subs.store.Subs)
	}
}

// With the card's subscribe switch off, the button only adds the videos, as the playlist dialog does: nothing is subscribed to.
func TestSubscribeDialogSwitchOffOnlyAdds(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{testSubCollection: testSubEpisodes})
	a, tt, updates := newTestApp(t, p)
	msgs := testlocale.Messages()
	subscribeRead(t, tt, a, p, updates, testSubCollection)
	tt.Press("id:subscribe-follow")
	wantText(t, tt, msgs.SubscribeAddJustAdd("collection", 3))
	tt.Press("id:subscribe-submit")
	want := []string{testSubEpisodes.Videos[0].URL, testSubEpisodes.Videos[1].URL, testSubEpisodes.Videos[2].URL}
	if got := playlistJobSources(a); !slices.Equal(got, want) || len(a.subs.store.Subs) != 0 || a.subs.ask.open {
		t.Fatalf("queued %q, subscriptions %+v, open %v", got, a.subs.store.Subs, a.subs.ask.open)
	}
}

// An old subscription's left-out episodes are offered only when the user asks: a scheduled check, as at launch, leaves them; 列出来 records the one the library has as added and offers the others.
func TestSubscribeOffersEarlierOnlyWhenAsked(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{testSubCollection: testSubEpisodes})
	a, tt, updates := newTestApp(t, p)
	old := subscribe.New(testSubCollection, testVideos, testSubEpisodes, time.Now().Add(-48*time.Hour))
	old.Offered, old.Cadence = false, subscribe.CadenceOpen
	a.subs.store.Subs = []subscribe.Sub{old}
	onEngine(a, func() { a.libraryEnqueue(testSubEpisodes.Videos[0].URL) })
	if len(a.library.Jobs) != 1 {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
	onEngine(a, func() { subscribeLaunch(a) })
	nextCall(t, p)
	waitUntil(t, tt, a, updates, func() bool { return a.subs.checking == "" })
	sub := &a.subs.store.Subs[0]
	if len(sub.Pending) != 0 || sub.Offered || len(a.library.Jobs) != 1 {
		t.Fatalf("the launch check offered the back catalogue: %+v", sub)
	}
	refresh(tt, a)
	tt.Press("id:subscription-" + old.ID)
	wantText(t, tt, testlocale.Messages().SubscribeEarlierCount(2))
	tt.Press("id:subscription-earlier-list")
	nextCall(t, p)
	waitUntil(t, tt, a, updates, func() bool { return a.subs.checking == "" })
	if len(sub.Added) != 1 || sub.Added[0].URL != testSubEpisodes.Videos[0].URL || len(sub.Pending) != 2 || sub.Pending[0].URL != testSubEpisodes.Videos[1].URL || !sub.Offered {
		t.Fatalf("subscription %+v", sub)
	}
}

// The playlist dialog's subscribe switch keeps waiting on the subscription's page the videos the queue refused, as for want of a language, so the collection is not left with nothing added and nothing waiting.
func TestPlaylistDialogSubscribeHoldsWhatItCouldNotQueue(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{testPlaylist: testListing})
	a, tt, updates := newTestApp(t, p)
	a.prefs.language = ""
	onEngine(a, func() { a.playlistAsk([]string{testPlaylist}, playlistFromLibrary) })
	<-p.calls
	waitUntil(t, tt, a, updates, func() bool { return a.playlist.open && !a.playlist.loading })
	tt.Press("id:playlist-subscribe")
	tt.Press("id:playlist-add")
	if len(a.subs.store.Subs) != 1 || len(a.library.Jobs) != 0 {
		t.Fatalf("subscriptions %+v, jobs %d", a.subs.store.Subs, len(a.library.Jobs))
	}
	if sub := a.subs.store.Subs[0]; len(sub.Pending) != len(testListing.Videos) || sub.Pending[0].URL != testListing.Videos[0].URL || len(sub.Added) != 0 {
		t.Fatalf("subscription %+v", sub)
	}
}

// The control socket subscribing to a channel adds none of its videos, which count as seen.
func TestRemoteSubscribeToAChannelAddsNone(t *testing.T) {
	videos := []media.ListedVideo{testSubLong, testSubOld}
	p := newListingPipeline(map[string]media.Listing{testSubChannel: {Title: "Earth Science", Videos: videos}})
	a, _, updates := newTestApp(t, p)
	remoteServe(t, a)
	got, err := remoteCall[remote.Subscribed](t, updates, remote.MethodSubscribe, remote.SubscribeParams{Source: testSubChannel})
	if err != nil {
		t.Fatal(err)
	}
	nextCall(t, p)
	if got.Subscription.Added != 0 || len(a.library.Jobs) != 0 || len(a.subs.store.Subs[0].Seen) != len(videos) {
		t.Fatalf("subscribed %+v, jobs %d", got, len(a.library.Jobs))
	}
}
