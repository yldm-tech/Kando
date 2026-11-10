package films

import (
	"slices"
	"strings"
	"testing"

	"kando/apps/kando/locales/testlocale"
)

func keysOf(items []Film) string {
	var keys []string
	for _, item := range items {
		keys = append(keys, item.Key)
	}
	return strings.Join(keys, ",")
}

// TestSortedByTitle orders the grid by name as the interface's language orders words: Latin letters regardless of case, numbers by value, then Chinese by pinyin. Films with the same name keep the order they were added in, and the shelf's own order is left alone.
func TestSortedByTitle(t *testing.T) {
	// Newest first, as List gives them.
	items := []Film{
		{Key: "zhong", Name: "中国"},
		{Key: "talk-new", Name: "Talk"},
		{Key: "l10", Name: "Lecture 10"},
		{Key: "an", Name: "安静"},
		{Key: "l2", Name: "lecture 2"},
		{Key: "talk-old", Name: "Talk"},
		{Key: "banana", Title: "Banana"},
	}
	before := slices.Clone(items)
	if got := keysOf(Sorted(items, SortTitle)); got != "banana,l2,l10,talk-new,talk-old,an,zhong" {
		t.Fatalf("by title %s", got)
	}
	if !slices.Equal(items, before) {
		t.Fatalf("Sorted changed the shelf's order: %s", keysOf(items))
	}
	if got := keysOf(Sorted(items, SortAdded)); got != keysOf(before) {
		t.Fatalf("recently added %s", got)
	}
}

// TestSortedByTitleFollowsTheInterfaceLanguage: in English Chinese titles are not ordered by pinyin, so 中 (zhōng) is no longer after 安 (ān).
func TestSortedByTitleFollowsTheInterfaceLanguage(t *testing.T) {
	items := []Film{{Key: "an", Name: "安静"}, {Key: "zhong", Name: "中国"}}
	if got := keysOf(Sorted(items, SortTitle)); got != "an,zhong" {
		t.Fatalf("by pinyin %s", got)
	}
	testlocale.Use(t, "en")
	if got := keysOf(Sorted(items, SortTitle)); got != "zhong,an" {
		t.Fatalf("in English %s", got)
	}
}

// TestSortedBySource groups the grid by source in the sidebar's order: YouTube, 哔哩哔哩, other sites by their names, then local files, a picked video and one found in the library folder alike. Within a source the films keep the order they were added in.
func TestSortedBySource(t *testing.T) {
	items := []Film{
		{Key: "found", Name: "Found"},
		{Key: "vimeo", Source: "https://vimeo.com/76979871"},
		{Key: "yt-new", Source: "https://youtu.be/TRWgBxVAb1M"},
		{Key: "picked", Source: "file:///Users/me/Movies/talk.mp4"},
		{Key: "bili", Source: "https://www.bilibili.com/video/BV1GJ411x7h7"},
		{Key: "ted", Source: "https://www.ted.com/talks/sir_ken_robinson_do_schools_kill_creativity"},
		{Key: "yt-old", Source: "https://www.youtube.com/watch?v=k9Yt2LmQ0pX"},
	}
	if got := keysOf(Sorted(items, SortSource)); got != "yt-new,yt-old,bili,ted,vimeo,found,picked" {
		t.Fatalf("by source %s", got)
	}
}

// TestValidSortFallsBackToRecentlyAdded: an order Kando does not know, as an empty one or one another version saved, shows the films as they were added.
func TestValidSortFallsBackToRecentlyAdded(t *testing.T) {
	for _, order := range []string{"", "duration", "Title"} {
		if got := ValidSort(order); got != SortAdded {
			t.Fatalf("ValidSort(%q) = %q", order, got)
		}
	}
	for _, order := range Sorts {
		if got := ValidSort(order); got != order {
			t.Fatalf("ValidSort(%q) = %q", order, got)
		}
	}
	items := []Film{{Key: "b", Name: "B"}, {Key: "a", Name: "A"}}
	if got := keysOf(Sorted(items, "duration")); got != "b,a" {
		t.Fatalf("an unknown order sorted the films: %s", got)
	}
	if got := SortLabel("duration"); got != "最近添加" {
		t.Fatalf("label of an unknown order %q", got)
	}
}
