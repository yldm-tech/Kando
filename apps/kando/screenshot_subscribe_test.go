package main

import (
	"strings"
	"testing"
	"time"

	"kando/apps/kando/subscribe"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// screenshotSubscriptions are the design's three subscriptions of 我的影片: a YouTube channel its site renamed, a bilibili collection that asks first with two new videos waiting, one too short for its rules, and a collection deleted on bilibili.
func screenshotSubscriptions(f *screenshotFixture) []subscribe.Sub {
	checked := time.Now().Add(-25 * time.Minute)
	channel := subscribe.New("https://www.youtube.com/@earthscience", f.lib, media.Listing{Title: "Earth Science"}, checked)
	channel.Renamed = "Earth Science Lab"
	channel.Queued([]subscribe.Video{{URL: "https://www.youtube.com/watch?v=ooooooooooo", Title: "How Rivers Shape Landscapes"}})
	course := subscribe.New("https://space.bilibili.com/12345/channel/collectiondetail?sid=67890", f.lib, media.Listing{Title: "线性代数的几何意义"}, checked)
	course.Mode, course.Filter, course.Cadence = subscribe.ModeAsk, subscribe.FilterLong, subscribe.CadenceDaily
	course.Hold([]subscribe.Video{
		{URL: "https://www.bilibili.com/video/BV1xx411c7mD", Title: "线性代数的几何意义 · P3 线性变换", Duration: 14*time.Minute + 20*time.Second},
		{URL: "https://www.bilibili.com/video/BV1xx411c7mE", Title: "线性代数的几何意义 · 预告", Duration: 48 * time.Second},
	})
	gone := subscribe.New("https://space.bilibili.com/24680/channel/collectiondetail?sid=13579", f.lib, media.Listing{Title: "日语听力每日一练"}, checked)
	gone.Gone = true
	return []subscribe.Sub{channel, course, gone}
}

// TestScreenshotsSubscribe renders the subscriptions in the design's states:
//   - subscribe-waiting: the sidebar's 订阅 with the design's three subscriptions, and the page of the collection that asks first, its two new videos waiting, one marked as not fitting the rules
//   - subscribe-renamed: the page of the channel its site renamed, with the banner that asks which name to keep
//   - subscribe-gone: the page of the collection deleted on bilibili, with the banner that offers to keep it without checks
//   - subscribe-ask: the dialog a link is pasted in, a channel's link typed in it
//   - subscribe-capped: the page of the channel, which adds by itself, after a check found five new videos: three joined the queue and two wait below
func TestScreenshotsSubscribe(t *testing.T) {
	f := newScreenshotFixture(t)
	show := func(t *testing.T, a *app, tt *nsgotest.App, index int) {
		t.Helper()
		screenshotReady(t, f, a)
		a.subs.store.Subs = screenshotSubscriptions(f)
		refresh(tt, a)
		tt.Press("id:subscription-" + a.subs.store.Subs[index].ID)
		tt.HoverOut()
		if !tt.Exists("id:subscription-page") {
			t.Fatalf("the page did not show: %q", tt.Texts())
		}
	}
	f.shoot(t, "subscribe-waiting", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		show(t, a, tt, 1)
		if !tt.Exists("id:subscription-waiting-count") {
			t.Fatalf("no new videos: %q", tt.Texts())
		}
	})
	f.shoot(t, "subscribe-renamed", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		show(t, a, tt, 0)
		if !tt.Exists("id:subscription-banner") {
			t.Fatalf("no banner: %q", tt.Texts())
		}
	})
	f.shoot(t, "subscribe-gone", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		show(t, a, tt, 2)
		if !tt.Exists("id:subscription-stop") {
			t.Fatalf("no banner: %q", tt.Texts())
		}
	})
	f.shoot(t, "subscribe-capped", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		show(t, a, tt, 0)
		sub := &a.subs.store.Subs[0]
		sub.Renamed = ""
		var fresh []media.ListedVideo
		for i, title := range []string{"Why Glaciers Move", "The Deepest Lake on Earth", "How Deltas Form", "Volcanoes Under the Ice", "Reading a River Map"} {
			fresh = append(fresh, media.ListedVideo{URL: "https://www.youtube.com/watch?v=" + strings.Repeat(string(rune('a'+i)), 11), Title: title, Duration: time.Duration(9+i) * time.Minute})
		}
		a.subscribeChecked(a.subs.run, sub.ID, media.Listing{Title: "Earth Science", Videos: fresh}, nil)
		refresh(tt, a)
		if !tt.Exists("id:subscription-note") || !tt.Exists("id:subscription-waiting-count") {
			t.Fatalf("no note or no videos waiting: %q", tt.Texts())
		}
	})
	f.shoot(t, "subscribe-ask", func(t *testing.T, a *app, tt *nsgotest.App, _ chan func()) {
		screenshotReady(t, f, a)
		a.subs.store.Subs = screenshotSubscriptions(f)
		refresh(tt, a)
		tt.Press("id:subscribe-add")
		tt.Type("id:subscribe-link", "https://www.youtube.com/@MindLab")
		if !tt.Exists("id:subscribe-ask") {
			t.Fatal("the dialog did not open")
		}
	})
}
