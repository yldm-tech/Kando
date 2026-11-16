package films

import (
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
)

func keysOf(items []Film) string {
	var keys []string
	for _, item := range items {
		keys = append(keys, item.Key)
	}
	return strings.Join(keys, ",")
}

// neverPlayed is App.PlayedAt of a library none of whose films played.
func neverPlayed(string) time.Time { return time.Time{} }

// TestSortedByWatched puts the films played last first and those never played after them, in the order they were added.
func TestSortedByWatched(t *testing.T) {
	items := []Film{
		{Key: "new", VideoPath: "/new.mp4"},
		{Key: "week", VideoPath: "/week.mp4"},
		{Key: "fresh", VideoPath: "/fresh.mp4"},
		{Key: "today", VideoPath: "/today.mp4"},
		{Key: "old", VideoPath: "/old.mp4"},
	}
	now := time.Date(2026, 11, 16, 20, 0, 0, 0, time.Local)
	played := map[string]time.Time{"/today.mp4": now, "/week.mp4": now.AddDate(0, 0, -7)}
	before := slices.Clone(items)
	if got := keysOf(Sorted(items, SortWatched, testlocale.Current(), func(path string) time.Time { return played[path] })); got != "today,week,new,fresh,old" {
		t.Fatalf("recently watched %s", got)
	}
	if !slices.Equal(items, before) {
		t.Fatalf("Sorted changed the shelf's order: %s", keysOf(items))
	}
	if got := SortLabel(testlocale.Messages(), SortWatched); got != "最近观看" {
		t.Fatalf("label %q", got)
	}
}

// TestSortedBySpoken groups the grid by the language heard in each film, by its name as the interface's language orders words, those whose language is not known last; within a language the films keep the order they were added in.
func TestSortedBySpoken(t *testing.T) {
	testlocale.Use(t, "en")
	items := []Film{
		{Key: "unknown", Name: "Found"},
		{Key: "ja-new", Spoken: "ja"},
		{Key: "en", Spoken: "en"},
		{Key: "ja-old", Spoken: "ja"},
		{Key: "it", Spoken: "it"},
	}
	// English, IT (a language the settings do not name), Japanese, then the one not known.
	if got := keysOf(Sorted(items, SortSpoken, testlocale.Current(), neverPlayed)); got != "en,it,ja-new,ja-old,unknown" {
		t.Fatalf("by spoken language %s", got)
	}
}

// TestPosterLanguages: a poster's tag names the language heard in its film and adds the translation's only when it is not the one set for new films; a film whose spoken language is not known says it is bilingual.
func TestPosterLanguages(t *testing.T) {
	m := testlocale.Messages()
	for _, tc := range []struct {
		item   Film
		target string
		want   string
	}{
		{Film{Spoken: "ja", Language: "简体中文"}, "简体中文", "JA"},
		{Film{Spoken: "ja", Language: "English"}, "简体中文", m.FilmsPosterLanguages("JA", "EN")},
		{Film{Language: "简体中文"}, "简体中文", m.FilmsPosterBilingual(m.FilmsLanguageTagSimplifiedChinese())},
	} {
		if got := PosterLanguages(m, tc.item, tc.target); got != tc.want {
			t.Errorf("PosterLanguages(%+v, %q) = %q, want %q", tc.item, tc.target, got, tc.want)
		}
	}
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
	if got := keysOf(Sorted(items, SortTitle, testlocale.Current(), neverPlayed)); got != "banana,l2,l10,talk-new,talk-old,an,zhong" {
		t.Fatalf("by title %s", got)
	}
	if !slices.Equal(items, before) {
		t.Fatalf("Sorted changed the shelf's order: %s", keysOf(items))
	}
	if got := keysOf(Sorted(items, SortAdded, testlocale.Current(), neverPlayed)); got != keysOf(before) {
		t.Fatalf("recently added %s", got)
	}
}

// TestSortedByTitleFollowsTheInterfaceLanguage: in English Chinese titles are not ordered by pinyin, so 中 (zhōng) is no longer after 安 (ān).
func TestSortedByTitleFollowsTheInterfaceLanguage(t *testing.T) {
	items := []Film{{Key: "an", Name: "安静"}, {Key: "zhong", Name: "中国"}}
	if got := keysOf(Sorted(items, SortTitle, testlocale.Current(), neverPlayed)); got != "an,zhong" {
		t.Fatalf("by pinyin %s", got)
	}
	testlocale.Use(t, "en")
	if got := keysOf(Sorted(items, SortTitle, testlocale.Current(), neverPlayed)); got != "zhong,an" {
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
	if got := keysOf(Sorted(items, SortSource, testlocale.Current(), neverPlayed)); got != "yt-new,yt-old,bili,ted,vimeo,found,picked" {
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
	if got := keysOf(Sorted(items, "duration", testlocale.Current(), neverPlayed)); got != "b,a" {
		t.Fatalf("an unknown order sorted the films: %s", got)
	}
	if got := SortLabel(testlocale.Messages(), "duration"); got != "最近添加" {
		t.Fatalf("label of an unknown order %q", got)
	}
}
