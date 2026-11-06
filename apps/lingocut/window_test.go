package main

import (
	"image"
	"testing"

	"lingocut/apps/lingocut/settings"
	"lingocut/apps/lingocut/sidebar"
	"lingocut/packages/core"

	"github.com/egoist/mygo/ui"
)

func TestVibrancyShowsThroughTheSidebarOnly(t *testing.T) {
	_, tt, _ := newTestApp(&fakePipeline{})
	tt.SetDark(true)
	tt.SetTitleBar(ui.TitleBar{Height: 28, Left: 80})
	tt.SetVibrancy(true)
	tt.Frame()
	img := tt.Image()
	scale := float32(img.Bounds().Dx()) / 1100
	// The gutter left of the sidebar's card shows the material, the card below the last source lets it through, and the content below the shelf is opaque.
	// Only the card's soft shadow, a few percent, tints the gutter.
	if a := alphaAt(img, scale, 3, 400); a > 16 {
		t.Fatalf("gutter alpha = %d, want the window's material to show", a)
	}
	if a := alphaAt(img, scale, 120, 400); a == 0 || a == 255 {
		t.Fatalf("card alpha = %d, want it translucent", a)
	}
	if a := alphaAt(img, scale, 700, 680); a != 255 {
		t.Fatalf("content alpha = %d, want it opaque", a)
	}

	_, plain, _ := newTestApp(&fakePipeline{})
	plain.SetDark(true)
	plain.Frame()
	if a := alphaAt(plain.Image(), scale, 3, 400); a != 255 {
		t.Fatalf("gutter alpha without vibrancy = %d, want opaque", a)
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

func TestSidebarArrowKeysChangeTheSource(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	tt.Frame()
	if err := tt.Click("全部影片"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.page != pageFilms || a.side.filter != sidebar.SourceAll {
		t.Fatalf("page %q source %q after clicking the chosen item", a.page, a.side.filter)
	}
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if a.side.filter != sidebar.SourceYouTube {
		t.Fatalf("source %q after Down, want %q", a.side.filter, sidebar.SourceYouTube)
	}
	tt.Key(0, ui.KeyUp)
	tt.Frame()
	if a.side.filter != sidebar.SourceAll {
		t.Fatalf("source %q after Up, want %q", a.side.filter, sidebar.SourceAll)
	}
}

// TestSidebarSourceLeavesTheSettings checks that a source chosen while a settings page shows, where no source is chosen, goes back to the library filtered by it.
func TestSidebarSourceLeavesTheSettings(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	a.OpenSettings(settings.SectionTranslate)
	tt.Frame()
	if err := tt.Click("本地文件"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.page != pageFilms || a.side.filter != sidebar.SourceLocal {
		t.Fatalf("page %q source %q after choosing 本地文件 in the settings", a.page, a.side.filter)
	}
}

// TestWindowIsLightAndTheFilmDark checks the palette: the window is light with LingoCut's colors whatever the desktop's appearance, and an open film is drawn in the dark theme.
func TestWindowIsLightAndTheFilmDark(t *testing.T) {
	a, tt, _ := newTestApp(&fakePipeline{})
	tt.SetDark(true)
	tt.Frame()
	img := tt.Image()
	scale := float32(img.Bounds().Dx()) / 1100
	if px := img.RGBAAt(int(700*scale), int(680*scale)); px.R != 255 || px.G != 255 || px.B != 255 {
		t.Fatalf("content = %v, want white", px)
	}

	a.library.Jobs = []core.Job{{ID: "j", Source: "https://example.com/v", Stage: core.StageQueued}}
	a.openKey = "j"
	tt.Frame()
	r, ok := tt.Find("返回")
	if !ok {
		t.Fatalf("film not open: %q", tt.Texts())
	}
	img = tt.Image()
	// Left of the back button is the film page's own padding.
	px := img.RGBAAt(int((r.X-6)*scale), int((r.Y+r.H/2)*scale))
	if px.R > 60 || px.G > 60 || px.B > 60 {
		t.Fatalf("film page = %v, want dark", px)
	}
}

func TestTitleBarClearsTheTrafficLights(t *testing.T) {
	_, tt, _ := newTestApp(&fakePipeline{})
	tt.SetTitleBar(ui.TitleBar{Height: 28, Left: 70})
	title, ok := tt.Find("videos")
	if !ok {
		t.Fatalf("texts %q", tt.Texts())
	}
	if title.Y < 28 {
		t.Fatalf("title sits under the traffic lights at y=%v", title.Y)
	}

	// Without a title bar the sidebar keeps no room for the traffic lights.
	_, plain, _ := newTestApp(&fakePipeline{})
	plain.Frame()
	plainTitle, ok := plain.Find("videos")
	if !ok || plainTitle.Y >= title.Y {
		t.Fatalf("title y=%v without a title bar, %v with one", plainTitle.Y, title.Y)
	}
}
