package playlist

import (
	"context"
	"slices"
	"testing"

	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) {
	testlocale.Pin(locales.Default)
	nsgotest.Main(m)
}

func TestSplit(t *testing.T) {
	videos, lists := Split([]string{
		" https://youtu.be/TRWgBxVAb1M ",
		"https://www.youtube.com/playlist?list=PLx0sYbCqOb8",
		"https://www.youtube.com/watch?v=TRWgBxVAb1M&list=PLx0sYbCqOb8",
		"https://www.youtube.com/@veritasium",
		"https://www.youtube.com/playlist?list=PLx0sYbCqOb8",
		"https://www.youtube.com/results?search_query=french",
		"",
	})
	if want := []string{"https://youtu.be/TRWgBxVAb1M", "https://www.youtube.com/watch?v=TRWgBxVAb1M&list=PLx0sYbCqOb8", "https://www.youtube.com/results?search_query=french", ""}; !slices.Equal(videos, want) {
		t.Errorf("videos %q, want %q", videos, want)
	}
	if want := []string{"https://www.youtube.com/playlist?list=PLx0sYbCqOb8", "https://www.youtube.com/@veritasium"}; !slices.Equal(lists, want) {
		t.Errorf("lists %q, want %q", lists, want)
	}
}

// limits records the limit each source was listed with.
type limits map[string]int

func (l limits) ListVideos(_ context.Context, source string, limit int) (media.Listing, error) {
	l[source] = limit
	return media.Listing{Title: source}, nil
}

func TestReadLimitsAChannel(t *testing.T) {
	got := limits{}
	lists, err := Read(context.Background(), got, []string{"https://www.youtube.com/playlist?list=PLx0sYbCqOb8", "https://space.bilibili.com/946974"})
	if err != nil {
		t.Fatal(err)
	}
	if len(lists) != 2 || lists[0].Channel || !lists[1].Channel || lists[1].Title != "https://space.bilibili.com/946974" {
		t.Fatalf("lists %+v", lists)
	}
	if got["https://www.youtube.com/playlist?list=PLx0sYbCqOb8"] != 0 || got["https://space.bilibili.com/946974"] != ChannelLimit {
		t.Fatalf("limits %v", got)
	}
}

// The dialog says that it looks for the videos, then how many there are, what a channel gives and what the library has, and adds only those it does not have.
func TestConfirmShowsWhatWasRead(t *testing.T) {
	var look *core.Signal[Look]
	var answers []string
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		look = core.NewSignal(cx, Look{Loading: true})
		return Confirm{
			Open:     func() bool { return true },
			Look:     look.Get,
			OnAdd:    func() { answers = append(answers, "add") },
			OnCancel: func() { answers = append(answers, "cancel") },
		}
	}, nsgotest.Size(800, 600), nsgotest.Environment(testlocale.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop})))
	msgs := testlocale.Messages()
	has := func(s string) bool { return app.Exists(`text:"` + s + `"`) }
	if !has(msgs.PlaylistReading()) || !app.Exists("id:playlist-spinner") || app.Exists("id:playlist-add") {
		t.Fatalf("while reading: %q", app.Texts())
	}
	look.Set(Look{Lists: 1, Channels: 1, Title: "Veritasium", Videos: 50, Have: 2})
	app.Settle()
	for _, want := range []string{"Veritasium", msgs.PlaylistVideos(50), msgs.PlaylistChannelLimit(ChannelLimit), msgs.PlaylistHave(2), msgs.PlaylistAdd(48)} {
		if !has(want) {
			t.Errorf("missing %q in %q", want, app.Texts())
		}
	}
	app.Press("id:playlist-add")
	look.Set(Look{Lists: 2, Videos: 3, Have: 3})
	app.Settle()
	if !has(msgs.PlaylistSeveral(2)) || app.Exists("id:playlist-add") || has(msgs.PlaylistChannelLimit(ChannelLimit)) {
		t.Errorf("every video in the library: %q", app.Texts())
	}
	look.Set(Look{Lists: 1, Err: "boom"})
	app.Settle()
	if !has(msgs.PlaylistUntitled()) || !has("boom") || app.Exists("id:playlist-add") {
		t.Errorf("failed: %q", app.Texts())
	}
	app.Press("id:playlist-cancel")
	if !slices.Equal(answers, []string{"add", "cancel"}) {
		t.Fatalf("answers %q", answers)
	}
}
