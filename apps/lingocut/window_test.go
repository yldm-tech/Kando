package main

import (
	"image"
	"testing"

	"lingocut/packages/core"

	"github.com/egoist/mygo/ui"
)

// TestWindowKeepsContentBelowTheTitleBar checks that the drag band under the title bar pushes the sidebar and the content down by the title bar's height, with and without an open film.
func TestWindowKeepsContentBelowTheTitleBar(t *testing.T) {
	plain, plainTT, _ := newTestApp(&fakePipeline{})
	barred, barredTT, _ := newTestApp(&fakePipeline{})
	barredTT.SetTitleBar(ui.TitleBar{Height: 28, Left: 80})
	shifted := func(texts ...string) {
		t.Helper()
		plainTT.Frame()
		barredTT.Frame()
		for _, text := range texts {
			before, ok := plainTT.Find(text)
			if !ok {
				t.Fatalf("missing %q in %q", text, plainTT.Texts())
			}
			after, ok := barredTT.Find(text)
			if !ok {
				t.Fatalf("missing %q in %q", text, barredTT.Texts())
			}
			if d := after.Y - before.Y; abs32(d-28) > 0.5 {
				t.Fatalf("%q moved %v under a 28-high title bar", text, d)
			}
		}
	}
	shifted("全部影片", "开始处理")

	// An open film hides the sidebar; its top row still clears the traffic lights.
	for _, a := range []*app{plain, barred} {
		a.library.Jobs = []core.Job{{ID: "j", Source: "https://example.com/v", Stage: core.StageQueued}}
		a.openKey = "j"
	}
	shifted("返回")
	if barredTT.HasText("全部影片") {
		t.Fatalf("sidebar shown over an open film: %q", barredTT.Texts())
	}
}

func TestVibrancyShowsThroughTheSidebarOnly(t *testing.T) {
	_, tt, _ := newTestApp(&fakePipeline{})
	tt.SetDark(true)
	tt.SetTitleBar(ui.TitleBar{Height: 28, Left: 80})
	tt.SetVibrancy(true)
	tt.Frame()
	img := tt.Image()
	scale := float32(img.Bounds().Dx()) / 1100
	// Below the last sidebar item and below the shelf, both regions are empty.
	if a := alphaAt(img, scale, 120, 680); a != 0 {
		t.Fatalf("sidebar alpha = %d, want the window's material to show", a)
	}
	if a := alphaAt(img, scale, 700, 680); a != 255 {
		t.Fatalf("content alpha = %d, want it opaque", a)
	}

	_, plain, _ := newTestApp(&fakePipeline{})
	plain.SetDark(true)
	plain.Frame()
	if a := alphaAt(plain.Image(), scale, 120, 680); a != 255 {
		t.Fatalf("sidebar alpha without vibrancy = %d, want opaque", a)
	}
}

func alphaAt(img *image.RGBA, scale, x, y float32) uint8 {
	return img.RGBAAt(int(x*scale), int(y*scale)).A
}

func TestPrimaryButtonUsesTheAccent(t *testing.T) {
	_, tt, _ := newTestApp(&fakePipeline{})
	tt.SetDark(true)
	tt.Frame()
	r, ok := tt.Find("开始处理")
	if !ok {
		t.Fatalf("missing start button in %q", tt.Texts())
	}
	img := tt.Image()
	scale := float32(img.Bounds().Dx()) / 1100
	// The label is padded inside the button; a point just before it is the button's face.
	px := img.RGBAAt(int((r.X-4)*scale), int((r.Y+r.H/2)*scale))
	want := ui.Hex("#0A66D8")
	for _, ch := range [][2]uint8{{px.R, want.R}, {px.G, want.G}, {px.B, want.B}} {
		if d := int(ch[0]) - int(ch[1]); d < -3 || d > 3 {
			t.Fatalf("button face = %v, want about %v", px, want)
		}
	}
}

func TestSidebarArrowKeysChangeThePage(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	tt.Frame()
	if err := tt.Click("全部影片"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.page != pageFilms {
		t.Fatalf("page %q after clicking the chosen item", a.page)
	}
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if a.page != pageYouTube {
		t.Fatalf("page %q after Down, want %q", a.page, pageYouTube)
	}
	tt.Key(0, ui.KeyUp)
	tt.Frame()
	if a.page != pageFilms {
		t.Fatalf("page %q after Up, want %q", a.page, pageFilms)
	}
}
