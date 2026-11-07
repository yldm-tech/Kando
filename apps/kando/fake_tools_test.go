package main

import (
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The screenshot fixture (screenshot_helpers_test.go) runs the window with fake ffmpeg, yt-dlp and whisper-cli instead of whatever this machine has installed, so the scenes show the same versions, paths and pictures everywhere. Their videos are placeholders the fake ffmpeg plays: a gradient (writeFakeClip) or one of the prototype's scene pictures in testdata/scenes (writeFakeScene). Each fake is a link named after the tool to this test binary; run under that name, the binary acts as the tool and exits before any test runs.

// The fake tools' versions, as their --version (ffmpeg -version) prints them.
const (
	fakeFFmpegVersion  = "ffmpeg version 7.1.1 Copyright (c) 2000-2025 the FFmpeg developers"
	fakeYTDLPVersion   = "2026.09.14"
	fakeWhisperVersion = "whisper-cli version 1.8.2"
)

// The fake ffmpeg's clips are 640×360 at ten frames a second, and its scene clips 1920×1080, the size of the larger scene pictures. How long each is and its picture are in the video file (writeFakeClip, writeFakeScene).
const (
	fakeClipWidth   = 640
	fakeClipHeight  = 360
	fakeSceneWidth  = 1920
	fakeSceneHeight = 1080
	fakeClipFPS     = 10
)

func init() {
	switch filepath.Base(os.Args[0]) {
	case "ffmpeg":
		os.Exit(fakeFFmpeg(os.Args[1:]))
	case "yt-dlp":
		os.Exit(fakeYTDLP(os.Args[1:]))
	case "whisper-cli":
		os.Exit(fakeVersionOnly(os.Args[1:], "--version", fakeWhisperVersion))
	}
}

// fakeTool links name in dir to this test binary and returns the link, for media.Tools.
func fakeTool(t *testing.T, dir, name string) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.Symlink(exe, path); err != nil {
		t.Fatal(err)
	}
	return path
}

// fakeClip is a video for the fake ffmpeg: its length and its picture, which is the same in every frame: the colors at the top and bottom of a gradient, or, when scene is set, the scene picture whose files are scene+"-640.png" and scene+"-1920.png".
type fakeClip struct {
	seconds     int
	top, bottom color.RGBA
	scene       string
}

// screenshotScenes is the folder of the prototype's scene pictures, each drawn at 640×360 and 1920×1080 (<name>-640.png, <name>-1920.png). They are the prototype's art for each film, which the fake ffmpeg shows as the film's picture.
const screenshotScenes = "testdata/scenes"

// writeFakeScene writes a placeholder video at path that the fake ffmpeg plays as a clip of seconds showing the prototype's scene name (river, market, ocean, lecture, meeting, street, podcast or generic) from testdata/scenes. The placeholder names the pictures by their absolute path, since the fake ffmpeg does not run in this package's folder.
func writeFakeScene(t *testing.T, path string, seconds int, scene string) {
	t.Helper()
	base, err := filepath.Abs(filepath.Join(screenshotScenes, scene))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(base + "-1920.png"); err != nil {
		t.Fatalf("scene %q: %v", scene, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(fmt.Sprintf("kando fake scene %d %s\n", seconds, base)), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeFakeClip writes a placeholder video at path that the fake ffmpeg plays as a clip of seconds whose picture fades from top to bottom, two colors as RRGGBB. Scenes make their videos with it instead of encoding one with an ffmpeg of this machine.
func writeFakeClip(t *testing.T, path string, seconds int, top, bottom string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(fmt.Sprintf("kando fake clip %d %s %s\n", seconds, top, bottom)), 0o644); err != nil {
		t.Fatal(err)
	}
}

// readFakeClip reads a placeholder writeFakeClip or writeFakeScene wrote; ok is false for any other file.
func readFakeClip(path string) (clip fakeClip, ok bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return clip, false
	}
	if rest, found := strings.CutPrefix(strings.TrimSuffix(string(data), "\n"), "kando fake scene "); found {
		seconds, scene, _ := strings.Cut(rest, " ")
		clip.seconds, err = strconv.Atoi(seconds)
		clip.scene = scene
		return clip, err == nil && clip.seconds > 0 && scene != ""
	}
	var top, bottom string
	if _, err := fmt.Sscanf(string(data), "kando fake clip %d %s %s", &clip.seconds, &top, &bottom); err != nil || clip.seconds <= 0 {
		return clip, false
	}
	clip.top, ok = fakeColor(top)
	if !ok {
		return clip, false
	}
	clip.bottom, ok = fakeColor(bottom)
	return clip, ok
}

// fakeColor parses RRGGBB.
func fakeColor(hex string) (color.RGBA, bool) {
	n, err := strconv.ParseUint(hex, 16, 32)
	if err != nil || len(hex) != 6 {
		return color.RGBA{}, false
	}
	return color.RGBA{uint8(n >> 16), uint8(n >> 8), uint8(n), 255}, true
}

// fakeVersionOnly answers flag with version and fails at anything else, as a tool with no network or model would.
func fakeVersionOnly(args []string, flag, version string) int {
	if slices.Contains(args, flag) {
		fmt.Println(version)
		return 0
	}
	fmt.Fprintln(os.Stderr, "fake tool: only "+flag+" is supported")
	return 1
}

// fakeYTDLPFolder is the folder the fake yt-dlp downloads its one video into, as the real one names "Me at the zoo" with Kando's output template.
const fakeYTDLPFolder = "Me at the zoo [jNQXAC9IVRw]"

// fakeYTDLP answers --version, and downloads "Me at the zoo" whatever the source as the real yt-dlp downloads a video YouTube serves as separate video and sound: the video track video.f395.mp4, then the sound track, written as video.f251.webm.part and renamed to video.f251.webm when done, both in the temp path (-P temp:) or, without one, in the video's folder under the home path (-P); then it merges them into <home>/Me at the zoo [jNQXAC9IVRw]/video.mp4, a clip the fake ffmpeg plays, and prints its path. A track gone by then fails the merge as the real one fails. A video already there is reported without downloading it again. When FAKE_YTDLP_GATE names a folder, the fake writes "tracks" there while the sound track is still a .part file and waits for "go" before it goes on, so a test can look at the library in between.
func fakeYTDLP(args []string) int {
	if slices.Contains(args, "--version") {
		fmt.Println(fakeYTDLPVersion)
		return 0
	}
	var home, temp string
	for i := 0; i+1 < len(args) && args[i] != "--"; i++ {
		if args[i] != "-P" {
			continue
		}
		i++
		if rest, ok := strings.CutPrefix(args[i], "temp:"); ok {
			temp = rest
		} else {
			home = args[i]
		}
	}
	if home == "" {
		fmt.Fprintln(os.Stderr, "fake yt-dlp: no -P home path")
		return 1
	}
	final := filepath.Join(home, fakeYTDLPFolder, "video.mp4")
	if _, err := os.Stat(final); err == nil {
		fmt.Println(final)
		return 0
	}
	work := filepath.Join(home, fakeYTDLPFolder)
	if temp != "" {
		work = filepath.Join(temp, fakeYTDLPFolder)
	}
	video, sound := filepath.Join(work, "video.f395.mp4"), filepath.Join(work, "video.f251.webm")
	fail := func(err error) int {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		return 1
	}
	if err := os.MkdirAll(work, 0o755); err != nil {
		return fail(err)
	}
	if err := os.WriteFile(video, []byte("kando fake track 395\n"), 0o644); err != nil {
		return fail(err)
	}
	if err := os.WriteFile(sound+".part", []byte("kando fake track 251\n"), 0o644); err != nil {
		return fail(err)
	}
	if gate := os.Getenv("FAKE_YTDLP_GATE"); gate != "" {
		if err := os.WriteFile(filepath.Join(gate, "tracks"), nil, 0o644); err != nil {
			return fail(err)
		}
		for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(10 * time.Millisecond) {
			if _, err := os.Stat(filepath.Join(gate, "go")); err == nil {
				break
			}
			if time.Now().After(deadline) {
				return fail(fmt.Errorf("no go in %s", gate))
			}
		}
	}
	if err := os.Rename(sound+".part", sound); err != nil {
		return fail(err)
	}
	for _, track := range []string{video, sound} {
		if _, err := os.Stat(track); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: Postprocessing: [Errno 2] No such file or directory: '%s'\n", track)
			return 1
		}
	}
	merged := filepath.Join(work, "video.temp.mp4")
	if err := os.WriteFile(merged, []byte("kando fake clip 19 6b8e4e 2f3b2a\n"), 0o644); err != nil {
		return fail(err)
	}
	for _, track := range []string{video, sound} {
		if err := os.Remove(track); err != nil {
			return fail(err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return fail(err)
	}
	if err := os.Rename(merged, final); err != nil {
		return fail(err)
	}
	fmt.Println(final)
	return 0
}

// fakeFFmpeg answers the calls Kando makes to ffmpeg: -version; probing a video (-i alone), which describes the clip on stderr; streaming its frames as raw RGBA, for the player; and one frame as JPEG, for a poster. The input must be a placeholder from writeFakeClip; any other file is not a video to it.
func fakeFFmpeg(args []string) int {
	if slices.Contains(args, "-version") {
		fmt.Println(fakeFFmpegVersion)
		return 0
	}
	input := fakeArg(args, "-i")
	clip, ok := readFakeClip(input)
	if !ok {
		fmt.Fprintf(os.Stderr, "%s: Invalid data found when processing input\n", filepath.Base(input))
		return 1
	}
	width, height := clip.scale(fakeArg(args, "-vf"))
	switch {
	case slices.Contains(args, "rawvideo"):
		at, _ := strconv.ParseFloat(fakeArg(args, "-ss"), 64)
		picture, err := clip.picture(width, height)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		pix := picture.Pix
		for frame := int(at * fakeClipFPS); frame < clip.seconds*fakeClipFPS; frame++ {
			if _, err := os.Stdout.Write(pix); err != nil {
				return 1
			}
		}
		return 0
	case slices.Contains(args, "mjpeg"):
		picture, err := clip.picture(width, height)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err := jpeg.Encode(os.Stdout, picture, &jpeg.Options{Quality: 90}); err != nil {
			return 1
		}
		return 0
	}
	ownWidth, ownHeight := clip.size()
	fmt.Fprintf(os.Stderr, "Input #0, mov,mp4,m4a,3gp,3g2,mj2, from '%s':\n  Duration: %02d:%02d:%02d.00, start: 0.000000, bitrate: 98 kb/s\n  Stream #0:0[0x1](und): Video: mpeg4 (Simple Profile) (mp4v / 0x7634706D), yuv420p, %dx%d [SAR 1:1 DAR 16:9], %d fps, %d tbr, %d tbn (default)\nAt least one output file must be specified\n",
		filepath.Base(input), clip.seconds/3600, clip.seconds/60%60, clip.seconds%60, ownWidth, ownHeight, fakeClipFPS, fakeClipFPS, fakeClipFPS*1000)
	return 1
}

// fakeArg is the value after flag in args, or "".
func fakeArg(args []string, flag string) string {
	if i := slices.Index(args, flag); i >= 0 && i+1 < len(args) {
		return args[i+1]
	}
	return ""
}

// size is the clip's own picture size, as the fake ffmpeg describes it when probed.
func (c fakeClip) size() (width, height int) {
	if c.scene != "" {
		return fakeSceneWidth, fakeSceneHeight
	}
	return fakeClipWidth, fakeClipHeight
}

// scale is the size a -vf scale=W:H filter asks for, the clip's own without one; a negative H keeps the clip's shape at width W, rounded to an even height as ffmpeg's -2 does.
func (c fakeClip) scale(filter string) (width, height int) {
	ownWidth, ownHeight := c.size()
	width, height = ownWidth, ownHeight
	w, h, ok := strings.Cut(strings.TrimPrefix(filter, "scale="), ":")
	if !strings.HasPrefix(filter, "scale=") || !ok {
		return width, height
	}
	if n, err := strconv.Atoi(w); err == nil && n > 0 {
		width = n
	}
	if n, err := strconv.Atoi(h); err == nil && n > 0 {
		height = n
	} else {
		height = width * ownHeight / ownWidth / 2 * 2
	}
	return width, height
}

// picture is every frame of the clip at width×height: the scene picture scaled to that size, or top's color at the top fading to bottom's at the bottom.
func (c fakeClip) picture(width, height int) (*image.RGBA, error) {
	if c.scene != "" {
		return scenePicture(c.scene, width, height)
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	mix := func(a, b uint8, y int) uint8 { return uint8(int(a) + (int(b)-int(a))*y/max(height-1, 1)) }
	for y := range height {
		line := color.RGBA{mix(c.top.R, c.bottom.R, y), mix(c.top.G, c.bottom.G, y), mix(c.top.B, c.bottom.B, y), 255}
		for x := range width {
			img.SetRGBA(x, y, line)
		}
	}
	return img, nil
}

// scenePicture is the scene picture at base scaled to width×height: the 640-wide file for a picture no wider than it, such as a poster, the 1920-wide one otherwise, each sampled bilinearly.
func scenePicture(base string, width, height int) (*image.RGBA, error) {
	name := base + "-1920.png"
	if width <= 640 {
		name = base + "-640.png"
	}
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	src, err := png.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(name), err)
	}
	bounds := src.Bounds()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	// at is the source pixel nearest to x, y, clamped to the picture.
	at := func(x, y int) color.RGBA {
		x = min(max(x, 0), bounds.Dx()-1)
		y = min(max(y, 0), bounds.Dy()-1)
		r, g, b, a := src.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
		return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
	}
	mix := func(a, b uint8, f float64) float64 { return float64(a) + (float64(b)-float64(a))*f }
	for y := range height {
		sy := (float64(y)+0.5)*float64(bounds.Dy())/float64(height) - 0.5
		y0, fy := int(math.Floor(sy)), sy-math.Floor(sy)
		for x := range width {
			sx := (float64(x)+0.5)*float64(bounds.Dx())/float64(width) - 0.5
			x0, fx := int(math.Floor(sx)), sx-math.Floor(sx)
			p00, p10, p01, p11 := at(x0, y0), at(x0+1, y0), at(x0, y0+1), at(x0+1, y0+1)
			blend := func(c00, c10, c01, c11 uint8) uint8 {
				return uint8(math.Round(mix(c00, c10, fx)*(1-fy) + mix(c01, c11, fx)*fy))
			}
			img.SetRGBA(x, y, color.RGBA{blend(p00.R, p10.R, p01.R, p11.R), blend(p00.G, p10.G, p01.G, p11.G), blend(p00.B, p10.B, p01.B, p11.B), 255})
		}
	}
	return img, nil
}
