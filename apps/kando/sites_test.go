package main

import (
	"testing"

	"kando/apps/kando/sites/catalog"
	"kando/packages/core"

	"github.com/egoist/mygo/ui"
)

func TestHelpMenuOpensSupportedSites(t *testing.T) {
	a, tt, updates := newTestApp(&fakePipeline{})
	tt.Frame()
	it := findMenuItem(t, a.menuBar(), "帮助", "支持的网站…")
	it.Click(it, nil)
	waitUntil(t, updates, func() bool { return a.sites.open })
	tt.Frame()
	for _, want := range []string{"支持的网站", "YouTube", "youtube.com", "哔哩哔哩", "完成"} {
		wantText(t, tt, want)
	}
	if !a.modalOpen() {
		t.Fatal("the sheet does not hold the menu bar's commands")
	}
	if !tt.Focused("搜索网站") {
		t.Fatal("the search does not have the focus")
	}
	if err := tt.Click("完成"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if a.sites.open || a.modalOpen() || tt.HasText("搜索网站名、域名，或粘贴视频链接") {
		t.Fatalf("完成 left the sheet open %v modal %v", a.sites.open, a.modalOpen())
	}
}

// Typing narrows the grid; a link shows the one site it is on, and a site off the list says so and what may still work. The sheet opens afresh each time.
func TestSupportedSitesSearch(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	tt.Frame()
	a.OpenSites()
	tt.Frame()
	tt.Type("niconico")
	tt.Frame()
	tt.Frame()
	wantText(t, tt, "nicovideo.jp")
	if tt.HasText("youtube.com") {
		t.Fatal("the search kept YouTube")
	}
	a.sites.query = "https://www.bilibili.com/video/BV1xx411c7mD"
	tt.Frame()
	wantText(t, tt, "找到 1 个 · 列表来自 yt-dlp "+catalog.Load().YTDLP)
	wantText(t, tt, "bilibili.com")
	a.sites.query = "https://missav.ai/dvaj-762"
	tt.Frame()
	wantText(t, tt, "列表里没有「https://missav.ai/dvaj-762」")
	// The first Escape clears the search, as a search field's does; the next closes the sheet.
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if !a.sites.open || a.sites.query != "" {
		t.Fatalf("Escape in the search: open %v, query %q", a.sites.open, a.sites.query)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	tt.Frame()
	if a.sites.open || a.modalOpen() {
		t.Fatal("Escape left the sheet open")
	}
	a.OpenSites()
	tt.Frame()
	if a.sites.query != "" {
		t.Fatalf("the sheet opened with %q", a.sites.query)
	}
	wantText(t, tt, "YouTube")
}

// The card of a site yt-dlp does not know offers the list.
func TestUnsupportedCardOpensSupportedSites(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	failedJob(t, a, "https://example.com/video/1", core.FailUnsupported, "")
	tt.Frame()
	if err := tt.Click("查看支持的网站"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.sites.open {
		t.Fatal("the sheet did not open")
	}
	wantText(t, tt, "支持的网站")
}

// The page of an empty library links to the list.
func TestEmptyLibraryOpensSupportedSites(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	tt.Frame()
	wantText(t, tt, "从一段视频开始")
	if err := tt.Click("支持的网站…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.sites.open {
		t.Fatal("the sheet did not open")
	}
}

func TestSitesItemIsAMenuOnlyRow(t *testing.T) {
	a, _, _ := newTestApp(&fakePipeline{})
	for _, s := range a.sitesKeys() {
		if s.Key != 0 || s.Group != "帮助" {
			t.Fatalf("row %+v", s)
		}
	}
}
