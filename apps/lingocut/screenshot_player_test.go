package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lingocut/packages/core"
	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
)

// riverCues are the prototype's subtitles (template script 1713-1732), each 10 seconds apart from 0.4 seconds in, as the prototype times them.
var riverCues = [][2]string{
	{"每条河都始于高山上的一股细流。", "Every river begins as a trickle high in the mountains."},
	{"水往低处流，沿途裹挟起泥沙和石块。", "As water flows downhill, it picks up sand and stones."},
	{"这些颗粒就像砂纸一样打磨着河床。", "Those particles act like sandpaper on the riverbed."},
	{"数千年间，河道被切割得越来越深。", "Over thousands of years, the channel cuts deeper."},
	{"陡峭的 V 形谷就是这样形成的。", "That is how steep V-shaped valleys are formed."},
	{"到了下游，地势变缓，河流也慢了下来。", "Lower down, the land flattens and the river slows."},
	{"它不再向下切割，而是开始左右摆动。", "Instead of cutting down, it starts to swing sideways."},
	{"这些大幅度的弯道被称为河曲。", "These sweeping curves are called meanders."},
	{"外侧河岸不断被侵蚀，内侧则不断淤积。", "The outer bank erodes while the inner bank grows."},
	{"有时河曲弯得太远，会把自己截断。", "Sometimes a meander loops so far it cuts itself off."},
	{"留下来的就是一个新月形的牛轭湖。", "What remains is a crescent-shaped oxbow lake."},
	{"洪水期间，河水会漫过河岸。", "During floods, the river spills over its banks."},
	{"它在平原上留下细腻而肥沃的淤泥。", "It leaves behind fine, fertile silt across the plain."},
	{"这就是为什么早期农田多沿河分布。", "That is why so many early farms grew along rivers."},
	{"接近大海时，水流终于失去了力量。", "Near the sea, the current finally loses its energy."},
	{"泥沙沉积下来，一层层堆积成三角洲。", "Sediment drops out and builds a delta, layer by layer."},
	{"从高山到大海，河流从未停止重塑大地。", "From mountain to sea, the river never stops reshaping the land."},
	{"下次过桥时，不妨看看桥下的流水。", "Next time you cross a bridge, look at the water below."},
}

// TestScreenshotsPlayer renders Colibri open from the library every scene starts from (screenshot_helpers_test.go), once its subtitles and, with ffmpeg, its picture have loaded. With ffmpeg it also renders the prototype's film, How Rivers Shape Landscapes, at the prototype's window size: the player held on a line, its subtitle mode menu, the style view and the 片段 and 信息 tabs.
func TestScreenshotsPlayer(t *testing.T) {
	f := newScreenshotFixture(t)
	f.shoot(t, "film", func(a *app, tt *ui.Tester, updates chan func()) {
		screenshotClick(t, tt, "Colibri")
		_, tools := f.pipeline.(media.Tools)
		deadline := time.After(8 * time.Second)
		for !tt.HasText("蜂鸟每秒振翅五十次。") || (tools && !tt.HasText("播放")) {
			select {
			case fn := <-updates:
				fn()
			case <-deadline:
				t.Fatalf("film did not load: %q err %q", tt.Texts(), a.player.playErr)
			}
			tt.Frame()
		}
	})

	tools, ok := f.pipeline.(media.Tools)
	if !ok {
		t.Skip("ffmpeg is not installed, so the prototype's film has no picture")
	}
	// The library is in the fixture's fake home, so the 信息 tab shows its folder as ~/我的影片.lingocut/….
	lib := filepath.Join(f.home, "我的影片.lingocut")
	folder := filepath.Join(lib, "How Rivers Shape Landscapes")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	clip := filepath.Join(folder, "video.mp4")
	// A sky over a green valley, three minutes long like the prototype's film.
	writeFakeClip(t, clip, 180, "8DB9D2", "3E6A5B")
	var srt strings.Builder
	for i, cue := range riverCues {
		start := time.Duration(i)*10*time.Second + 400*time.Millisecond
		fmt.Fprintf(&srt, "%d\n%s --> %s\n%s\n%s\n\n", i+1, srtClock(start), srtClock(start+9*time.Second), cue[0], cue[1])
	}
	subs := filepath.Join(folder, "video.bilingual.srt")
	if err := os.WriteFile(subs, []byte(srt.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	job := core.Job{ID: "rivers", Source: "https://www.youtube.com/watch?v=riverfilm01", Language: "简体中文", Stage: core.StageComplete, VideoPath: clip, SubtitlePath: subs, LibraryDir: lib}

	shoot := func(name string, steps func(a *app, tt *ui.Tester)) {
		t.Helper()
		updates := make(chan func(), 64)
		a := newApp(tools, func(fn func()) { updates <- fn })
		a.edit(func(s *session) { s.LibraryDir = lib })
		a.library.Jobs = []core.Job{job}
		t.Cleanup(a.stopPlayback)
		tt := ui.NewTester(a.view, 1304, 782)
		tt.SetScale(2)
		tt.SetTitleBar(ui.TitleBar{Height: 28, Left: 80})
		tt.Frame()
		// Held on the second line, 12 seconds in.
		a.player.playFrom = 12 * time.Second
		if err := tt.Click("How Rivers Shape Landscapes"); err != nil {
			t.Fatalf("%v; texts %q", err, tt.Texts())
		}
		deadline := time.After(10 * time.Second)
		for a.player.play == nil || !a.player.subs.ready || a.player.film.info.Folder == "" || !tt.HasText(riverCues[1][0]) {
			select {
			case fn := <-updates:
				fn()
			case <-deadline:
				t.Fatalf("film did not load: %q err %q", tt.Texts(), a.player.playErr)
			}
			tt.Frame()
		}
		// Let the picture's first frame come.
		for range 20 {
			tt.Advance(50 * time.Millisecond)
			tt.Frame()
			time.Sleep(20 * time.Millisecond)
		}
		steps(a, tt)
		tt.Frame()
		file, err := os.Create(filepath.Join(f.out, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := png.Encode(file, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	shoot("player", func(*app, *ui.Tester) {})
	shoot("player-menu", func(_ *app, tt *ui.Tester) { screenshotClick(t, tt, "字幕：双语") })
	shoot("player-style", func(_ *app, tt *ui.Tester) {
		screenshotClick(t, tt, "字幕：双语")
		screenshotClick(t, tt, "样式…")
	})
	shoot("player-clips", func(_ *app, tt *ui.Tester) { screenshotClick(t, tt, "片段") })
	shoot("player-info", func(_ *app, tt *ui.Tester) { screenshotClick(t, tt, "信息") })
}

func srtClock(d time.Duration) string {
	ms := d.Milliseconds()
	return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3_600_000, ms/60_000%60, ms/1000%60, ms%1000)
}
