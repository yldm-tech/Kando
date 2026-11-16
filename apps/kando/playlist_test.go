package main

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/playlist"
	"kando/apps/kando/remote"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
)

// listCall is one ListVideos a listingPipeline was asked.
type listCall struct {
	source string
	limit  int
}

// listingPipeline is a fakePipeline whose jobs wait to be cancelled (fakePipeline.block), so the tests' queue keeps still, and which lists the playlists of lists as media.Tools lists them with yt-dlp. Each ListVideos is reported on calls; with hold set it waits until its context ends, and stopped receives its error.
type listingPipeline struct {
	fakePipeline
	lists   map[string]media.Listing
	hold    bool
	calls   chan listCall
	stopped chan error
}

func newListingPipeline(lists map[string]media.Listing) *listingPipeline {
	return &listingPipeline{fakePipeline: fakePipeline{block: true}, lists: lists, calls: make(chan listCall, 8), stopped: make(chan error, 8)}
}

func (p *listingPipeline) ListVideos(ctx context.Context, source string, limit int) (media.Listing, error) {
	p.calls <- listCall{source, limit}
	if p.hold {
		<-ctx.Done()
		p.stopped <- ctx.Err()
		return media.Listing{}, ctx.Err()
	}
	listing, ok := p.lists[source]
	if !ok {
		return media.Listing{}, errors.New("yt-dlp: exit status 1: ERROR: [youtube:tab] nope: The playlist does not exist.")
	}
	if limit > 0 && len(listing.Videos) > limit {
		listing.Videos = listing.Videos[:limit]
	}
	return listing, nil
}

// The playlist the tests list, and one of its videos the library already has.
const (
	testPlaylist = "https://www.youtube.com/playlist?list=PLx0sYbCqOb8"
	testChannel  = "https://www.youtube.com/@veritasium"
)

var testListing = media.Listing{Title: "French in Action", Videos: []media.ListedVideo{
	{URL: "https://www.youtube.com/watch?v=aaaaaaaaaaa", Title: "Episode 1"},
	{URL: "https://www.youtube.com/watch?v=bbbbbbbbbbb", Title: "Episode 2"},
	{URL: "https://www.youtube.com/watch?v=ccccccccccc", Title: "Episode 3"},
}}

// playlistJobSources is the sources of the open library's jobs, in the order they were queued.
func playlistJobSources(a *app) []string {
	var out []string
	for _, job := range a.library.Jobs {
		out = append(out, job.Source)
	}
	slices.Reverse(out)
	return out
}

// A playlist pasted into the link field is not a job of its own: the dialog reads its videos off the engine's goroutine, says how many there are and which the library has, and queues the others one job each, in the playlist's order, only once asked.
func TestPlaylistAddsItsVideosOnceAsked(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{testPlaylist: testListing})
	a, tt, updates := newTestApp(t, p)
	msgs := testlocale.Messages()
	a.library.Jobs = []core.Job{{ID: "have", Source: "https://youtu.be/bbbbbbbbbbb", Stage: core.StageComplete, VideoPath: "/videos/Episode 2/video.mp4", LibraryDir: "/videos"}}
	refresh(tt, a)

	startJob(t, tt, testPlaylist)
	if got := <-p.calls; got != (listCall{testPlaylist, 0}) {
		t.Fatalf("listed %+v, want the whole playlist", got)
	}
	if !tt.Exists("id:playlist-spinner") || !hasText(tt, msgs.PlaylistReading()) || tt.Exists("id:playlist-add") || !a.modalOpen() {
		t.Fatalf("while reading: %q", tt.Texts())
	}
	if len(a.library.Jobs) != 1 || a.queue.source != "" {
		t.Fatalf("jobs %+v, field %q", a.library.Jobs, a.queue.source)
	}
	waitUntil(t, tt, a, updates, func() bool { return !a.playlist.loading })
	for _, want := range []string{"French in Action", msgs.PlaylistVideos(3), msgs.PlaylistHave(1), msgs.PlaylistAdd(2)} {
		wantText(t, tt, want)
	}
	if hasText(tt, msgs.PlaylistChannelLimit(playlist.ChannelLimit)) {
		t.Fatal("a playlist says a channel's limit")
	}
	tt.Press("id:playlist-add")
	if got := playlistJobSources(a); !slices.Equal(got, []string{"https://youtu.be/bbbbbbbbbbb", "https://www.youtube.com/watch?v=aaaaaaaaaaa", "https://www.youtube.com/watch?v=ccccccccccc"}) {
		t.Fatalf("jobs %q", got)
	}
	if tt.Exists("id:playlist-confirm") || a.modalOpen() || !hasText(tt, msgs.LibraryAddedStarting(2)) {
		t.Fatalf("after adding: modal %v, texts %q", a.modalOpen(), tt.Texts())
	}
}

// A channel lists only its most recent videos, and the dialog says how many it takes.
func TestPlaylistChannelTakesItsMostRecent(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{testChannel: {Title: "Veritasium", Videos: testListing.Videos}})
	a, tt, updates := newTestApp(t, p)
	msgs := testlocale.Messages()
	startJob(t, tt, testChannel)
	if got := <-p.calls; got != (listCall{testChannel, playlist.ChannelLimit}) {
		t.Fatalf("listed %+v, want the channel's %d most recent", got, playlist.ChannelLimit)
	}
	waitUntil(t, tt, a, updates, func() bool { return !a.playlist.loading })
	for _, want := range []string{"Veritasium", msgs.PlaylistVideos(3), msgs.PlaylistChannelLimit(playlist.ChannelLimit), msgs.PlaylistAdd(3)} {
		wantText(t, tt, want)
	}
}

// Cancel closes the dialog while the videos are still being read, stops yt-dlp, gives the window's modal flag back and queues nothing; what the reading finds afterwards opens nothing.
func TestPlaylistCancelStopsTheReading(t *testing.T) {
	p := newListingPipeline(nil)
	p.hold = true
	a, tt, updates := newTestApp(t, p)
	startJob(t, tt, testPlaylist)
	<-p.calls
	tt.Press("id:playlist-cancel")
	select {
	case err := <-p.stopped:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("reading stopped with %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the reading did not stop")
	}
	waitUpdate(t, tt, a, updates)
	if a.playlist.open || tt.Exists("id:playlist-confirm") || a.modalOpen() || len(a.library.Jobs) != 0 {
		t.Fatalf("open %v, modal %v, jobs %+v", a.playlist.open, a.modalOpen(), a.library.Jobs)
	}
}

// A list yt-dlp cannot read says why, with nothing to add; Escape closes it.
func TestPlaylistSaysWhyItCannotBeRead(t *testing.T) {
	p := newListingPipeline(nil)
	a, tt, updates := newTestApp(t, p)
	startJob(t, tt, testPlaylist)
	waitUntil(t, tt, a, updates, func() bool { return !a.playlist.loading })
	wantText(t, tt, testlocale.Messages().PlaylistFailed("The playlist does not exist."))
	if tt.Exists("id:playlist-add") {
		t.Fatal("a list that could not be read offers to add")
	}
	tt.Key("escape")
	if a.playlist.open || a.modalOpen() {
		t.Fatal("Escape left the dialog open")
	}
}

// Links pasted together, between spaces or on lines of their own, are added one job each, and the link field's tag counts them; a playlist among them opens the dialog for it once the others are queued.
func TestStartAddsEveryPastedLink(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{testPlaylist: testListing})
	a, tt, updates := newTestApp(t, p)
	msgs := testlocale.Messages()
	// A paste holds its lines, which typing a line break would submit instead.
	a.queue.source = "https://youtu.be/TRWgBxVAb1M\nhttps://vimeo.com/76979871  " + testPlaylist
	refresh(tt, a)
	wantText(t, tt, msgs.FilmsLinkCount(3))
	tt.Press(whole(msgs.FilmsStart()))
	if got := playlistJobSources(a); !slices.Equal(got, []string{"https://youtu.be/TRWgBxVAb1M", "https://vimeo.com/76979871"}) {
		t.Fatalf("jobs %q", got)
	}
	waitUntil(t, tt, a, updates, func() bool { return a.playlist.open && !a.playlist.loading })
	tt.Press("id:playlist-add")
	if got := len(a.library.Jobs); got != 5 {
		t.Fatalf("%d jobs, want 5", got)
	}
}

// A video of a playlist added on its own is that video, and the message bar offers to add the whole playlist, which opens the dialog for it; the bar stays until it is used or closed.
func TestWholePlaylistFromTheMessageBar(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{testPlaylist: testListing})
	a, tt, updates := newTestApp(t, p)
	startJob(t, tt, "https://www.youtube.com/watch?v=aaaaaaaaaaa&list=PLx0sYbCqOb8")
	if got := playlistJobSources(a); len(got) != 1 || a.playlist.open {
		t.Fatalf("jobs %q, dialog open %v", got, a.playlist.open)
	}
	tt.Advance(noticeFor + time.Second)
	if !tt.Exists("id:notice-list") {
		t.Fatalf("no Add Whole Playlist: %q", tt.Texts())
	}
	tt.Press("id:notice-list")
	if got := <-p.calls; got.source != testPlaylist {
		t.Fatalf("listed %+v", got)
	}
	waitUntil(t, tt, a, updates, func() bool { return !a.playlist.loading })
	// The video added a moment ago is waiting, so only the other two are added.
	wantText(t, tt, testlocale.Messages().PlaylistAdd(2))
}

// On a playlist's or a channel's page, 下载视频 lists its videos and asks, as the link field does, and the browser's message bar says how many were added.
func TestBrowseDownloadsAPlaylist(t *testing.T) {
	p := newListingPipeline(map[string]media.Listing{testPlaylist: testListing})
	a, tt, updates, _ := browseApp(t, p)
	msgs := testlocale.Messages()
	browseOpen(t, a, tt, testPlaylist)
	if !tt.Enabled("id:browse-download") || !hasText(tt, msgs.BrowseDownloadList()) {
		t.Fatalf("下载视频 on a playlist: %q", tt.Texts())
	}
	tt.Press("id:browse-download")
	tt.AnswerScript(browsePage, `"`+testPlaylist+`"`, "")
	tt.Settle()
	waitUntil(t, tt, a, updates, func() bool { return a.playlist.open && !a.playlist.loading })
	tt.Press("id:playlist-add")
	if len(a.library.Jobs) != 3 || a.page != pageBrowse || !a.browse.notice.Open || a.browse.notice.Text != msgs.LibraryAddedStarting(3) {
		t.Fatalf("jobs %d, page %q, notice %+v", len(a.library.Jobs), a.page, a.browse.notice)
	}
	// A page of search results is still no video.
	browseReport(tt, ns.WebNavigation{URL: "https://www.youtube.com/results?search_query=french"})
	if tt.Enabled("id:browse-download") {
		t.Fatal("下载视频 is on on search results")
	}
}

// lists.expand lists each playlist and channel link, marks the videos the library has, and says why a link that is no list could not be listed.
func TestRemoteListsAPlaylist(t *testing.T) {
	if media.ListTimeout >= remote.ListTimeout {
		t.Fatalf("yt-dlp may list for %v, longer than a client waits (%v)", media.ListTimeout, remote.ListTimeout)
	}
	p := newListingPipeline(map[string]media.Listing{testPlaylist: testListing, testChannel: {Title: "Veritasium", Videos: testListing.Videos[:1]}})
	a, _, updates := newTestApp(t, p)
	remoteServe(t, a)
	a.library.Jobs = []core.Job{{ID: "have", Source: "https://youtu.be/bbbbbbbbbbb", Stage: core.StageQueued, LibraryDir: "/videos"}}
	lists, err := remoteCall[[]remote.List](t, updates, remote.MethodLists, remote.ListsParams{Sources: []string{testPlaylist, testChannel, "https://youtu.be/TRWgBxVAb1M"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(lists) != 3 {
		t.Fatalf("lists %+v", lists)
	}
	if l := lists[0]; l.Kind != "playlist" || l.Title != "French in Action" || l.Limit != 0 || len(l.Videos) != 3 || l.Videos[0].InLibrary || !l.Videos[1].InLibrary || l.Videos[1].Title != "Episode 2" {
		t.Errorf("playlist %+v", l)
	}
	if l := lists[1]; l.Kind != "channel" || l.Limit != playlist.ChannelLimit || len(l.Videos) != 1 {
		t.Errorf("channel %+v", l)
	}
	if l := lists[2]; l.Error == "" || len(l.Videos) != 0 {
		t.Errorf("a video %+v", l)
	}
}
