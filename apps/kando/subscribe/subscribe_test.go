package subscribe

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"testing"
	"time"

	"kando/packages/media"
)

var (
	t0      = time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	listing = media.Listing{Title: "Earth Science", Videos: []media.ListedVideo{
		{URL: "https://www.youtube.com/watch?v=aaaaaaaaaaa", Title: "Rivers", Duration: 12 * time.Minute},
		{URL: "https://www.youtube.com/watch?v=bbbbbbbbbbb", Title: "Tides", Duration: 40 * time.Second},
	}}
)

// A new subscription is named after its list and counts the videos the list holds now as seen, so a check finds only those that come after.
func TestNewSeedsTheVideosThereNow(t *testing.T) {
	s := New(" https://www.youtube.com/@earthscience ", "/videos", listing, t0)
	if s.ID == "" || s.Source != "https://www.youtube.com/@earthscience" || s.Name != "Earth Science" || s.Title != "Earth Science" || s.Library != "/videos" || !s.Channel() {
		t.Fatalf("new: %+v", s)
	}
	if s.Rules != DefaultRules() || !s.Checked.Equal(t0) {
		t.Fatalf("rules %+v, checked %v", s.Rules, s.Checked)
	}
	found := s.Take(listing, t0.Add(time.Hour))
	if len(found.Fresh) != 0 || found.Skipped != 0 {
		t.Fatalf("the videos there when subscribed came back as new: %+v", found)
	}
	if other := New("https://www.youtube.com/playlist?list=PLx", "/videos", listing, t0); other.ID == s.ID {
		t.Fatal("two subscriptions share an ID")
	}
	if untitled := New("https://space.bilibili.com/12345", "/videos", media.Listing{}, t0); untitled.Name != "space.bilibili.com/12345" {
		t.Fatalf("a list without a name is called %q", untitled.Name)
	}
}

// A check keeps the videos it has not seen that fit the rules, in the list's order, skips the others, and finds each video new only once.
func TestTakeFindsEachNewVideoOnce(t *testing.T) {
	s := New("https://www.youtube.com/@earthscience", "/videos", media.Listing{Title: "Earth Science"}, t0)
	s.Filter = FilterLong
	found := s.Take(listing, t0.Add(time.Minute))
	if len(found.Fresh) != 1 || found.Fresh[0].Title != "Rivers" || found.Fresh[0].Duration != 12*time.Minute || found.Skipped != 1 {
		t.Fatalf("found %+v", found)
	}
	again := s.Take(listing, t0.Add(2*time.Minute))
	if len(again.Fresh) != 0 || again.Skipped != 0 || !s.Checked.Equal(t0.Add(2*time.Minute)) {
		t.Fatalf("again: %+v, checked %v", again, s.Checked)
	}
}

// The filter takes every video, those longer than five minutes (and those whose length is not known), or those whose titles hold a keyword in any case; keywords are split at commas, Chinese commas and spaces.
func TestRulesFit(t *testing.T) {
	long, short, unknown := Video{Title: "Rivers of Japan", Duration: 6 * time.Minute}, Video{Title: "Tides", Duration: time.Minute}, Video{Title: "线性代数 P3"}
	cases := []struct {
		rules Rules
		want  [3]bool
	}{
		{Rules{Filter: FilterAll}, [3]bool{true, true, true}},
		{Rules{Filter: FilterLong}, [3]bool{true, false, true}},
		{Rules{Filter: FilterKeywords}, [3]bool{true, true, true}},
		{Rules{Filter: FilterKeywords, Keywords: "japan，tides"}, [3]bool{true, true, false}},
		{Rules{Filter: FilterKeywords, Keywords: " 线性代数 , x"}, [3]bool{false, false, true}},
	}
	for _, c := range cases {
		got := [3]bool{c.rules.Fits(long), c.rules.Fits(short), c.rules.Fits(unknown)}
		if got != c.want {
			t.Errorf("%+v: %v, want %v", c.rules, got, c.want)
		}
	}
}

// A new name the site gives the list is kept until the user answers, and the site going back to the old one forgets it; answering takes the new name as the subscription's or keeps the old one, and later checks compare with the new.
func TestTakeNoticesARename(t *testing.T) {
	s := New("https://www.youtube.com/@earthscience", "/videos", listing, t0)
	s.Take(media.Listing{Title: "Earth Science Lab"}, t0)
	if s.Renamed != "Earth Science Lab" || s.Name != "Earth Science" {
		t.Fatalf("after a rename: %+v", s)
	}
	s.Take(media.Listing{Title: "Earth Science"}, t0)
	if s.Renamed != "" {
		t.Fatalf("renamed back, still %q", s.Renamed)
	}
	s.Take(media.Listing{Title: "Earth Science Lab"}, t0)
	kept := s
	kept.KeepName(false)
	if kept.Name != "Earth Science" || kept.Title != "Earth Science Lab" || kept.Renamed != "" {
		t.Fatalf("keeping the old name: %+v", kept)
	}
	s.KeepName(true)
	if s.Name != "Earth Science Lab" || s.Title != "Earth Science Lab" || s.Renamed != "" {
		t.Fatalf("using the new name: %+v", s)
	}
	s.Take(media.Listing{Title: "Earth Science Lab"}, t0)
	if s.Renamed != "" {
		t.Fatalf("the name answered for came back as a rename: %q", s.Renamed)
	}
}

// A list yt-dlp says does not exist twice in a row is gone, and then checked once a day rather than on its cadence; once is a failure like any other, kept to show and tried again. Any other failure in between starts the count again, and a check that lists it again brings it back. yt-dlp taking too long is kept as TimedOut rather than as words.
func TestFailTellsGoneFromUnreachable(t *testing.T) {
	s := New("https://www.youtube.com/playlist?list=PLx", "/videos", listing, t0)
	s.Cadence = CadenceHourly
	missing := errors.New("yt-dlp: exit status 1: ERROR: [youtube:tab] PLx: The playlist does not exist.")
	unreachable := errors.New("yt-dlp: exit status 1: ERROR: Unable to download webpage: <urlopen error [Errno 8] nodename nor servname provided>")
	s.Fail(missing, t0)
	if s.Gone || s.Err == "" || s.Missing != 1 || !s.Due(t0.Add(time.Hour), t0) {
		t.Fatalf("gone after one answer: %+v", s)
	}
	s.Fail(unreachable, t0)
	s.Fail(missing, t0)
	if s.Gone || s.Missing != 1 {
		t.Fatalf("an answer in between did not start the count again: %+v", s)
	}
	s.Fail(missing, t0)
	if !s.Gone || s.Err != "" || s.Due(t0.Add(23*time.Hour), t0) || !s.Due(t0.Add(24*time.Hour), t0) {
		t.Fatalf("gone: %+v", s)
	}
	s.Take(listing, t0)
	if s.Gone || s.Missing != 0 {
		t.Fatalf("listed again, still gone: %+v", s)
	}
	s.Fail(unreachable, t0)
	if s.Gone || s.Err == "" || s.TimedOut {
		t.Fatalf("unreachable: %+v", s)
	}
	s.Fail(media.ErrListTimeout, t0)
	if s.Err != "" || !s.TimedOut {
		t.Fatalf("timed out: %+v", s)
	}
	s.Take(listing, t0)
	if s.TimedOut || s.Err != "" {
		t.Fatalf("a check that worked kept the failure: %+v", s)
	}
}

// Each cadence is due when its time came: on open until it was checked since the app opened, however long after the launch that is, hourly and daily once their time passed since the last check, and none while paused or stopped.
func TestDue(t *testing.T) {
	s := New("https://www.youtube.com/@earthscience", "/videos", listing, t0)
	check := func(cadence Cadence, after, opened time.Duration, want bool) {
		t.Helper()
		s.Cadence = cadence
		if got := s.Due(t0.Add(after), t0.Add(opened)); got != want {
			t.Errorf("%s after %v, opened %v: due %v, want %v", cadence, after, opened, got, want)
		}
	}
	check(CadenceOpen, time.Minute, time.Second, true)
	check(CadenceOpen, 72*time.Hour, time.Hour, true)
	check(CadenceOpen, 72*time.Hour, -time.Hour, false)
	check(CadenceOpen, -time.Hour, -2*time.Hour, true)
	check(CadenceHourly, 59*time.Minute, time.Second, false)
	check(CadenceHourly, time.Hour, -time.Hour, true)
	check(CadenceDaily, 23*time.Hour, time.Second, false)
	check(CadenceDaily, 24*time.Hour, -time.Hour, true)
	check(CadenceDaily, -time.Hour, -2*time.Hour, true)
	s.Paused = true
	check(CadenceOpen, time.Minute, time.Second, false)
	s.Paused, s.Stopped = false, true
	check(CadenceHourly, 48*time.Hour, time.Second, false)
}

// Waiting videos are held each once, leave when queued or skipped, and a queued one is added each once.
func TestPendingAndAdded(t *testing.T) {
	var s Sub
	a, b, c := Video{URL: "a"}, Video{URL: "b"}, Video{URL: "c"}
	s.Hold([]Video{a, b})
	s.Hold([]Video{b, c})
	if !reflect.DeepEqual(s.Pending, []Video{a, b, c}) {
		t.Fatalf("pending %+v", s.Pending)
	}
	if got := s.Waiting([]string{"c", "a"}); !reflect.DeepEqual(got, []Video{a, c}) {
		t.Fatalf("waiting %+v", got)
	}
	s.Queued([]Video{b})
	s.Queued([]Video{b})
	s.Skip([]string{"c"})
	if !reflect.DeepEqual(s.Pending, []Video{a}) || !reflect.DeepEqual(s.Added, []Video{b}) {
		t.Fatalf("pending %+v, added %+v", s.Pending, s.Added)
	}
}

// A check that adds by itself queues at most three new videos, the first in the list's order, and holds the rest; three or fewer are all queued.
func TestCapQueuesThreeAndHoldsTheRest(t *testing.T) {
	if AddPerCheck != 3 {
		t.Fatalf("AddPerCheck = %d, want 3", AddPerCheck)
	}
	var fresh []Video
	for _, link := range []string{"a", "b", "c", "d", "e"} {
		fresh = append(fresh, Video{URL: link})
	}
	queue, hold := Cap(fresh)
	if !reflect.DeepEqual(urls(queue), []string{"a", "b", "c"}) || !reflect.DeepEqual(urls(hold), []string{"d", "e"}) {
		t.Fatalf("queue %+v, hold %+v", queue, hold)
	}
	// Appending to what is queued does not write over what is held.
	_ = append(queue, Video{URL: "x"})
	if hold[0].URL != "d" {
		t.Fatalf("hold %+v", hold)
	}
	for _, n := range []int{0, 1, 3} {
		queue, hold := Cap(fresh[:n])
		if len(queue) != n || hold != nil {
			t.Fatalf("Cap of %d: queue %+v, hold %+v", n, queue, hold)
		}
	}
}

// The store reads back what it saved, readable by the user alone, puts rules it does not know back to the defaults, and leaves out entries without an ID or a link; a missing file has none.
func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Kando", "subscriptions.json")
	store, err := Load(path)
	if err != nil || len(store.Subs) != 0 {
		t.Fatalf("missing: %+v, %v", store, err)
	}
	sub := New("https://www.youtube.com/@earthscience", "/videos", listing, t0)
	sub.Hold([]Video{{URL: "https://www.youtube.com/watch?v=ccccccccccc", Title: "Waves", Duration: time.Minute}})
	store.Subs = append(store.Subs, sub)
	if err := store.Save(); err != nil {
		t.Fatal(err)
	}
	back, err := Load(path)
	if err != nil || len(back.Subs) != 1 || !reflect.DeepEqual(back.Subs[0], sub) {
		t.Fatalf("read back %+v, %v; want %+v", back.Subs, err, sub)
	}
	if info, err := os.Stat(path); err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("file %v, %v", info, err)
	}
	odd := `{"subscriptions":[{"id":"x","source":"https://www.youtube.com/@a","library":"/v","name":"A","mode":"never","filter":"?","cadence":"weekly"},{"id":"","source":"https://www.youtube.com/@b"},{"id":"y","source":" "}]}`
	if err := os.WriteFile(path, []byte(odd), 0o600); err != nil {
		t.Fatal(err)
	}
	back, err = Load(path)
	if err != nil || len(back.Subs) != 1 || back.Subs[0].Rules != DefaultRules() {
		t.Fatalf("odd file: %+v, %v", back.Subs, err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("a file that is not JSON read")
	}
	if err := (Store{}).Save(); err != nil {
		t.Fatal(err)
	}
}

// A link pasted again finds the subscription to the same list in the same library, however it is written; another library's is another subscription. The store lists a library's subscriptions and follows a library that moved.
func TestStoreFindsAndMoves(t *testing.T) {
	var store Store
	store.Subs = []Sub{
		New("https://www.youtube.com/@earthscience/", "/videos", listing, t0),
		New("https://space.bilibili.com/12345", "/other.kando", listing, t0),
	}
	if sub, ok := store.Following("youtube.com/@EarthScience", "/videos/"); !ok || sub.ID != store.Subs[0].ID {
		t.Fatalf("following: %v %v", sub, ok)
	}
	if _, ok := store.Following("https://www.youtube.com/@earthscience", "/other.kando"); ok {
		t.Fatal("another library's subscription found")
	}
	if got := store.In("/videos"); len(got) != 1 || got[0].ID != store.Subs[0].ID {
		t.Fatalf("in: %+v", got)
	}
	if len(store.In("")) != 0 {
		t.Fatal("no library has subscriptions")
	}
	if !store.MoveLibrary("/other.kando", "/renamed.kando") || store.Subs[1].Library != "/renamed.kando" || store.MoveLibrary("/nowhere", "/x") {
		t.Fatalf("moved: %+v", store.Subs)
	}
	id := store.Subs[0].ID
	store.Remove(id)
	if _, ok := store.Find(id); ok || len(store.Subs) != 1 {
		t.Fatalf("removed: %+v", store.Subs)
	}
	if !slices.ContainsFunc(store.Subs, func(s Sub) bool { return s.Library == "/renamed.kando" }) {
		t.Fatal("the other subscription went too")
	}
}

// A list's link is followed as it is, a bilibili video's page as its whole upload, and any other link not at all.
func TestSource(t *testing.T) {
	cases := []struct {
		typed, want string
		ok          bool
	}{
		{" https://space.bilibili.com/946974/channel/collectiondetail?sid=1 ", "https://space.bilibili.com/946974/channel/collectiondetail?sid=1", true},
		{"https://www.youtube.com/@veritasium", "https://www.youtube.com/@veritasium", true},
		{"https://www.bilibili.com/video/BV1Eb4u6bEsd", "https://www.bilibili.com/video/BV1Eb4u6bEsd", true},
		{"https://www.bilibili.com/video/BV1Eb4u6bEsd?p=4&spm_id_from=333", "https://www.bilibili.com/video/BV1Eb4u6bEsd", true},
		{"https://www.youtube.com/watch?v=aaaaaaaaaaa", "", false},
		{"https://b23.tv/abc123", "", false},
		{"not a link", "", false},
	}
	for _, c := range cases {
		if got, ok := Source(c.typed); got != c.want || ok != c.ok {
			t.Errorf("Source(%q) = %q, %v; want %q, %v", c.typed, got, ok, c.want, c.ok)
		}
	}
}

// Start is what a new subscription adds now: a playlist's or a collection's every video that fits the rules, in the list's order, and a channel's as many of its most recent, listed first, as asked.
func TestStartAddsAListWholeAndAChannelsLatest(t *testing.T) {
	list := New("https://www.youtube.com/playlist?list=PLx", "/videos", listing, t0)
	if got := urls(list.Start(listing, 0)); !slices.Equal(got, []string{listing.Videos[0].URL, listing.Videos[1].URL}) {
		t.Fatalf("a playlist adds %q, want every video in order", got)
	}
	list.Filter = FilterLong
	if got := urls(list.Start(listing, 0)); !slices.Equal(got, []string{listing.Videos[0].URL}) {
		t.Fatalf("a playlist that takes long videos adds %q", got)
	}
	channel := New("https://www.youtube.com/@earthscience", "/videos", listing, t0)
	for recent, want := range map[int]int{0: 0, 1: 1, 3: 2} {
		if got := channel.Start(listing, recent); len(got) != want || (want > 0 && got[0].URL != listing.Videos[0].URL) {
			t.Fatalf("a channel asked for its %d latest adds %+v", recent, got)
		}
	}
}

// A subscription to a collection made before Kando added a list's videos counted them as seen; its next check offers those it neither added nor holds, once, in the list's order, while a channel's and a new subscription's offer none.
func TestTakeOffersTheVideosAnOldSubscriptionLeftOut(t *testing.T) {
	const collection = "https://space.bilibili.com/12345/channel/collectiondetail?sid=67890"
	old := New(collection, "/videos", listing, t0)
	old.Offered = false
	old.Queued([]Video{{URL: listing.Videos[1].URL}})
	if old.Earlier() != 1 {
		t.Fatalf("earlier %d, want the one not added", old.Earlier())
	}
	found := old.Take(listing, t0.Add(time.Hour))
	if len(found.Earlier) != 1 || found.Earlier[0].URL != listing.Videos[0].URL || found.Earlier[0].Title != "Rivers" || len(found.Fresh) != 0 || !old.Offered || old.Earlier() != 0 {
		t.Fatalf("found %+v, offered %v", found, old.Offered)
	}
	if again := old.Take(listing, t0.Add(2*time.Hour)); len(again.Earlier) != 0 {
		t.Fatalf("offered twice: %+v", again)
	}
	if fresh := New(collection, "/videos", listing, t0); fresh.Earlier() != 0 || len(fresh.Take(listing, t0).Earlier) != 0 {
		t.Fatal("a new subscription offers its videos again")
	}
	channel := New("https://www.youtube.com/@earthscience", "/videos", listing, t0)
	channel.Offered = false
	if channel.Earlier() != 0 || len(channel.Take(listing, t0).Earlier) != 0 {
		t.Fatal("a channel offers the videos it had")
	}
}
