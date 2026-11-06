package main

import (
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The screenshot fixture (screenshot_helpers_test.go) runs the window with fake ffmpeg, yt-dlp and whisper-cli instead of whatever this machine has installed, so the scenes show the same versions, paths and pictures everywhere. Their videos are placeholders the fake ffmpeg plays (writeFakeClip). Each fake is a link named after the tool to this test binary; run under that name, the binary acts as the tool and exits before any test runs.

// The fake tools' versions, as their --version (ffmpeg -version) prints them.
const (
	fakeFFmpegVersion  = "ffmpeg version 7.1.1 Copyright (c) 2000-2025 the FFmpeg developers"
	fakeYTDLPVersion   = "2025.10.22"
	fakeWhisperVersion = "whisper-cli version 1.8.2"
)

// The fake ffmpeg's clips are 640×360 at ten frames a second. How long each is and its picture are in the video file (writeFakeClip).
const (
	fakeClipWidth  = 640
	fakeClipHeight = 360
	fakeClipFPS    = 10
)

func init() {
	switch filepath.Base(os.Args[0]) {
	case "ffmpeg":
		os.Exit(fakeFFmpeg(os.Args[1:]))
	case "yt-dlp":
		os.Exit(fakeVersionOnly(os.Args[1:], "--version", fakeYTDLPVersion))
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

// fakeClip is a video for the fake ffmpeg: its length and the colors at the top and bottom of its picture, which is the same in every frame.
type fakeClip struct {
	seconds     int
	top, bottom color.RGBA
}

// writeFakeClip writes a placeholder video at path that the fake ffmpeg plays as a clip of seconds whose picture fades from top to bottom, two colors as RRGGBB. Scenes make their videos with it instead of encoding one with an ffmpeg of this machine.
func writeFakeClip(t *testing.T, path string, seconds int, top, bottom string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(fmt.Sprintf("lingocut fake clip %d %s %s\n", seconds, top, bottom)), 0o644); err != nil {
		t.Fatal(err)
	}
}

// readFakeClip reads a placeholder writeFakeClip wrote; ok is false for any other file.
func readFakeClip(path string) (clip fakeClip, ok bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return clip, false
	}
	var top, bottom string
	if _, err := fmt.Sscanf(string(data), "lingocut fake clip %d %s %s", &clip.seconds, &top, &bottom); err != nil || clip.seconds <= 0 {
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

// fakeFFmpeg answers the calls LingoCut makes to ffmpeg: -version; probing a video (-i alone), which describes the clip on stderr; streaming its frames as raw RGBA, for the player; and one frame as JPEG, for a poster. The input must be a placeholder from writeFakeClip; any other file is not a video to it.
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
	width, height := fakeScale(fakeArg(args, "-vf"))
	switch {
	case slices.Contains(args, "rawvideo"):
		at, _ := strconv.ParseFloat(fakeArg(args, "-ss"), 64)
		pix := clip.picture(width, height).Pix
		for frame := int(at * fakeClipFPS); frame < clip.seconds*fakeClipFPS; frame++ {
			if _, err := os.Stdout.Write(pix); err != nil {
				return 1
			}
		}
		return 0
	case slices.Contains(args, "mjpeg"):
		if err := jpeg.Encode(os.Stdout, clip.picture(width, height), nil); err != nil {
			return 1
		}
		return 0
	}
	fmt.Fprintf(os.Stderr, "Input #0, mov,mp4,m4a,3gp,3g2,mj2, from '%s':\n  Duration: %02d:%02d:%02d.00, start: 0.000000, bitrate: 98 kb/s\n  Stream #0:0[0x1](und): Video: mpeg4 (Simple Profile) (mp4v / 0x7634706D), yuv420p, %dx%d [SAR 1:1 DAR 16:9], %d fps, %d tbr, %d tbn (default)\nAt least one output file must be specified\n",
		filepath.Base(input), clip.seconds/3600, clip.seconds/60%60, clip.seconds%60, fakeClipWidth, fakeClipHeight, fakeClipFPS, fakeClipFPS, fakeClipFPS*1000)
	return 1
}

// fakeArg is the value after flag in args, or "".
func fakeArg(args []string, flag string) string {
	if i := slices.Index(args, flag); i >= 0 && i+1 < len(args) {
		return args[i+1]
	}
	return ""
}

// fakeScale is the size a -vf scale=W:H filter asks for, the clip's own without one; a negative H keeps the clip's shape at width W, rounded to an even height as ffmpeg's -2 does.
func fakeScale(filter string) (width, height int) {
	width, height = fakeClipWidth, fakeClipHeight
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
		height = width * fakeClipHeight / fakeClipWidth / 2 * 2
	}
	return width, height
}

// picture is every frame of the clip at width×height: top's color at the top fading to bottom's at the bottom.
func (c fakeClip) picture(width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	mix := func(a, b uint8, y int) uint8 { return uint8(int(a) + (int(b)-int(a))*y/max(height-1, 1)) }
	for y := range height {
		line := color.RGBA{mix(c.top.R, c.bottom.R, y), mix(c.top.G, c.bottom.G, y), mix(c.top.B, c.bottom.B, y), 255}
		for x := range width {
			img.SetRGBA(x, y, line)
		}
	}
	return img
}
