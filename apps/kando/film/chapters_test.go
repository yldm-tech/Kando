package film

import (
	"slices"
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
	"kando/packages/core"
)

// riverChapters are the chapters a YouTube description marks in a three-minute film, the first two translated.
var riverChapters = []core.Chapter{
	{Start: 0, Title: "Intro", Translation: "开场"},
	{Start: 24 * time.Second, Title: "Where Rivers Begin", Translation: "河流的源头"},
	{Start: 58 * time.Second, Title: "Carving Valleys"},
}

// A site's chapters are 片段 from each start to the next, the last to the film's end, named by the translation with the site's title as the detail, or by the title alone; they are at the cues' time, the subtitle delay before the video's.
func TestSiteChaptersRunToTheNext(t *testing.T) {
	s := time.Second
	want := []Chapter{
		{Start: 0, End: 24 * s, Title: "开场", Detail: "Intro"},
		{Start: 24 * s, End: 58 * s, Title: "河流的源头", Detail: "Where Rivers Begin"},
		{Start: 58 * s, End: 180 * s, Title: "Carving Valleys"},
	}
	if got := SiteChapters(riverChapters, 180*s, 0); !slices.Equal(got, want) {
		t.Fatalf("chapters %#v", got)
	}
	// The length not known yet, the last ends where it starts; a delay of half a second moves them all back by it.
	got := SiteChapters(riverChapters, 0, s/2)
	if got[2].End != got[2].Start || got[0].Start != -s/2 || got[1].Start != 24*s-s/2 {
		t.Fatalf("delayed %#v", got)
	}
}

// A film is cut at its site's chapters when the user chose them for it, or chose nothing while 导入平台章节 is on; otherwise, and for a film with fewer than two, evenly at its lines.
func TestChaptersForFollowsTheFilmThenTheSetting(t *testing.T) {
	s := time.Second
	for _, tc := range []struct {
		use      core.ChapterUse
		chapters []core.Chapter
		importOn bool
		site     bool
	}{
		{"", riverChapters, true, true},
		{"", riverChapters, false, false},
		{core.ChaptersPlatform, riverChapters, false, true},
		{core.ChaptersEven, riverChapters, true, false},
		{core.ChaptersPlatform, riverChapters[:1], true, false},
	} {
		info := Info{Chapters: tc.chapters, ChapterUse: tc.use}
		got := ChaptersFor(info, tc.importOn, chapterCues, 180*s, 0)
		want := Chapters(chapterCues, 180*s)
		if tc.site {
			want = SiteChapters(tc.chapters, 180*s, 0)
		}
		if CutsAtSite(info, tc.importOn) != tc.site || !slices.Equal(got, want) {
			t.Errorf("%+v: chapters %#v", tc, got)
		}
	}
}

// The clips tab of a film whose site marks chapters lists them under a note that says how many and where from, and its button cuts the film by time instead, which the note then says; a film without chapters has neither.
func TestClipsTabCutsAtTheSitesChapters(t *testing.T) {
	msgs := testlocale.Messages()
	a := newFakeApp()
	a.side = "clips"
	a.item.Source = "https://www.youtube.com/watch?v=a1B2c3D4e5F"
	a.info = Info{Folder: "/films/talk", Chapters: riverChapters}
	app := start(t, a)
	site := msgs.FilmChapterSite("youtube")
	if got := app.Find("id:film-clips-note"); got == nil || got.Text != msgs.FilmClipsChaptersTranslated(3, site) {
		t.Fatalf("note %q", app.Texts())
	}
	first := msgs.FilmClip(1, cueClock(0), cueClock(24*time.Second), "开场")
	if !app.Exists(`label:"` + first + `"`) {
		t.Fatalf("the clips tab lists %q", app.Texts())
	}
	app.Press("id:film-clips-cut")
	if !slices.Equal(a.chapterUses, []core.ChapterUse{core.ChaptersEven}) {
		t.Fatalf("uses %q", a.chapterUses)
	}
	if got := app.Find("id:film-clips-note"); got == nil || got.Text != msgs.FilmClipsEven() || app.Exists(`label:"`+first+`"`) {
		t.Fatalf("cut by time: %q", app.Texts())
	}
	if !app.Exists(`label:"`+clipName(a, 1)+`"`) || !app.Exists(`label:"`+msgs.FilmClipsUseChapters()+`"`) {
		t.Fatalf("even cuts: %q", app.Texts())
	}
	app.Press("id:film-clips-cut")
	if !slices.Equal(a.chapterUses, []core.ChapterUse{core.ChaptersEven, core.ChaptersPlatform}) {
		t.Fatalf("uses %q", a.chapterUses)
	}

	b := newFakeApp()
	b.side = "clips"
	app = start(t, b)
	if app.Exists("id:film-clips-note") || app.Exists("id:film-clips-cut") {
		t.Fatalf("a film without chapters: %q", app.Texts())
	}
}

// The info tab says how many chapters the film's site marks and where from, and that it is cut by time instead when it is; a bilibili film's come from its 视频看点.
func TestInfoTabNamesTheChapters(t *testing.T) {
	msgs := testlocale.Messages()
	for _, tc := range []struct {
		source string
		use    core.ChapterUse
		want   string
	}{
		{"https://www.youtube.com/watch?v=a1B2c3D4e5F", "", msgs.FilmChaptersInfo(3, msgs.FilmChapterSite("youtube"))},
		{"https://www.bilibili.com/video/BV1xK4y1A7nM", core.ChaptersEven, msgs.FilmChaptersInfoEven(3, msgs.FilmChapterSite("bilibili"))},
		{"https://vimeo.com/76979871", "", msgs.FilmChaptersInfo(3, msgs.FilmChapterSite("other"))},
	} {
		a := newFakeApp()
		a.side = "info"
		a.item.Source = tc.source
		a.info = Info{Folder: "/films/talk", Chapters: riverChapters, ChapterUse: tc.use}
		app := start(t, a)
		if got := app.Find("id:film-info-chapters"); got == nil || got.Text != tc.want {
			t.Fatalf("%+v: info tab %q", tc, app.Texts())
		}
	}
	a := newFakeApp()
	a.side = "info"
	if app := start(t, a); app.Exists("id:film-info-chapters") {
		t.Fatal("a film without chapters names them")
	}
}

func (f *fakeApp) ImportChapters() bool { return !f.evenChapters }

func (f *fakeApp) SetFilmChapterUse(use core.ChapterUse) {
	f.chapterUses = append(f.chapterUses, use)
	f.info.ChapterUse = use
}
