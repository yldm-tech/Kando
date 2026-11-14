package main

import (
	"testing"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/sites/catalog"
	"kando/packages/core"
)

func TestHelpMenuOpensSupportedSites(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	msgs := testlocale.Messages()
	it := findMenuItem(t, a.menuBar(a.cx), msgs.MenuGroupHelp(), msgs.SitesMenuItem())
	choose(tt, a, it)
	if !a.sites.open {
		t.Fatal("the sheet did not open")
	}
	for _, want := range []string{msgs.SitesTitle(), "YouTube", "youtube.com", "哔哩哔哩", msgs.SitesDone()} {
		wantText(t, tt, want)
	}
	if !a.modalOpen() {
		t.Fatal("the sheet does not hold the menu bar's commands")
	}
	if !tt.Find(msgs.SitesSearchLabel()).Focused {
		t.Fatal("the search does not have the focus")
	}
	tt.Press("id:sites-done")
	if a.sites.open || a.modalOpen() || tt.Exists("id:sites") {
		t.Fatalf("完成 left the sheet open %v modal %v", a.sites.open, a.modalOpen())
	}
}

// Typing narrows the grid; a link shows the one site it is on, and a site off the list says so and what may still work. The sheet opens afresh each time.
func TestSupportedSitesSearch(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	msgs := testlocale.Messages()
	a.OpenSites()
	refresh(tt, a)
	typeFocused(t, tt, "niconico")
	wantText(t, tt, "nicovideo.jp")
	if hasText(tt, "youtube.com") {
		t.Fatal("the search kept YouTube")
	}
	a.sites.query = "https://www.bilibili.com/video/BV1xx411c7mD"
	refresh(tt, a)
	wantText(t, tt, msgs.SitesFound(1, msgs.SitesSource(catalog.Load().YTDLP)))
	wantText(t, tt, "bilibili.com")
	a.sites.query = "https://missav.ai/dvaj-762"
	refresh(tt, a)
	wantText(t, tt, msgs.SitesNotFound("https://missav.ai/dvaj-762"))
	// The first Escape clears the search, as a search field's does; the next closes the sheet.
	tt.Key("escape")
	if !a.sites.open || a.sites.query != "" {
		t.Fatalf("Escape in the search: open %v, query %q", a.sites.open, a.sites.query)
	}
	tt.Key("escape")
	if a.sites.open || a.modalOpen() {
		t.Fatal("Escape left the sheet open")
	}
	a.OpenSites()
	refresh(tt, a)
	if a.sites.query != "" || tt.Find("id:sites-search").Text != "" {
		t.Fatalf("the sheet opened with %q", a.sites.query)
	}
	wantText(t, tt, "YouTube")
}

// The card of a site yt-dlp does not know offers the list.
func TestUnsupportedCardOpensSupportedSites(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	failedJob(t, a, "https://example.com/video/1", core.FailUnsupported, "")
	refresh(tt, a)
	tt.Press(whole(testlocale.Messages().QueueRemedySites()))
	if !a.sites.open {
		t.Fatal("the sheet did not open")
	}
	wantText(t, tt, testlocale.Messages().SitesTitle())
}

// The page of an empty library links to the list.
func TestEmptyLibraryOpensSupportedSites(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	wantText(t, tt, testlocale.Messages().FilmsEmptyTitle())
	tt.Press(whole(testlocale.Messages().FilmsSupportedSites()))
	if !a.sites.open {
		t.Fatal("the sheet did not open")
	}
}

func TestSitesItemIsAMenuOnlyRow(t *testing.T) {
	a, _, _ := newTestApp(t, &fakePipeline{})
	for _, s := range a.sitesKeys(a.cx) {
		if s.Key != "" || s.Group != groupHelp {
			t.Fatalf("row %+v", s)
		}
	}
}

// The sheet speaks the interface's language: in English its title, count and button are English.
func TestSupportedSitesInEnglish(t *testing.T) {
	testlocale.Use(t, "en")
	a, tt, _ := newTestApp(t, &fakePipeline{})
	a.OpenSites()
	refresh(tt, a)
	for _, want := range []string{"Supported Websites", "Done", "List from yt-dlp " + catalog.Load().YTDLP} {
		wantText(t, tt, want)
	}
	if hasText(tt, "完成") || hasText(tt, "需要登录、会员或有 DRM 加密的视频，即使网站在列表里也不一定能下载。") {
		t.Fatal("the sheet still reads Chinese")
	}
}
