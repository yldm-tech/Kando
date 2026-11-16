package media

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestListVideosPlaylist(t *testing.T) {
	dir := t.TempDir()
	args := filepath.Join(dir, "args")
	tools := Tools{YTDLP: writeFakeTool(t, dir, "yt-dlp", fakeTool{Args: args, StdoutFile: "testdata/ytdlp/list/playlist.json"}), Cookies: "chrome"}
	listing, err := tools.ListVideos(context.Background(), "https://www.youtube.com/playlist?list=PLFs4vir_WsTwEd-nJgVJCZPNL3HALHHpF", 0)
	if err != nil {
		t.Fatal(err)
	}
	// The private video and the second copy of the first are left out.
	want := Listing{Title: "The Universe and Space stuff", Videos: []ListedVideo{
		{URL: "https://www.youtube.com/watch?v=4_aOIA-vyBo", Title: "Three Ways to Destroy the Universe", Duration: 377 * time.Second},
		{URL: "https://www.youtube.com/watch?v=e-P5IFTqB98", Title: "Black Holes Explained – From Birth to Death", Duration: 358500 * time.Millisecond},
		{URL: "https://www.youtube.com/watch?v=bbbbbbbbbbb", Title: "The Last Star"},
	}}
	if !reflect.DeepEqual(listing, want) {
		t.Fatalf("listing\n%+v\nwant\n%+v", listing, want)
	}
	data, _ := os.ReadFile(args)
	got := string(data)
	for _, arg := range []string{"--flat-playlist\n", "-J\n", "--cookies-from-browser\nchrome\n", "--\nhttps://www.youtube.com/playlist?list=PLFs4vir_WsTwEd-nJgVJCZPNL3HALHHpF\n"} {
		if !strings.Contains(got, arg) {
			t.Errorf("args lack %q:\n%s", arg, got)
		}
	}
	if strings.Contains(got, "--playlist-end") {
		t.Errorf("a playlist is listed whole, args:\n%s", got)
	}
}

func TestListVideosChannel(t *testing.T) {
	dir := t.TempDir()
	args := filepath.Join(dir, "args")
	tools := Tools{YTDLP: writeFakeTool(t, dir, "yt-dlp", fakeTool{Args: args, StdoutFile: "testdata/ytdlp/list/channel.json"})}
	listing, err := tools.ListVideos(context.Background(), "https://www.youtube.com/@kurzgesagt", 1)
	if err != nil {
		t.Fatal(err)
	}
	// The channel is named as itself, not as its Videos tab, its playlist is no video, and the limit keeps the newest.
	if listing.Title != "Kurzgesagt – In a Nutshell" || len(listing.Videos) != 1 || listing.Videos[0].URL != "https://www.youtube.com/watch?v=fEjvSbI_PT4" {
		t.Fatalf("listing %+v", listing)
	}
	data, _ := os.ReadFile(args)
	if !strings.Contains(string(data), "--playlist-end\n1\n") || !strings.HasSuffix(string(data), "--\nhttps://www.youtube.com/@kurzgesagt/videos\n") {
		t.Fatalf("args:\n%s", data)
	}
}

func TestListSource(t *testing.T) {
	cases := map[string]string{
		"https://www.youtube.com/@kurzgesagt":                              "https://www.youtube.com/@kurzgesagt/videos",
		"youtube.com/@kurzgesagt/":                                         "https://youtube.com/@kurzgesagt/videos",
		"https://www.youtube.com/channel/UCsXVk37bltHxD1rDPwtNM8Q":         "https://www.youtube.com/channel/UCsXVk37bltHxD1rDPwtNM8Q/videos",
		"https://www.youtube.com/c/veritasium":                             "https://www.youtube.com/c/veritasium/videos",
		"https://www.youtube.com/@kurzgesagt/shorts":                       "https://www.youtube.com/@kurzgesagt/shorts",
		"https://www.youtube.com/channel/UCsXVk37bltHxD1rDPwtNM8Q/streams": "https://www.youtube.com/channel/UCsXVk37bltHxD1rDPwtNM8Q/streams",
		"https://www.youtube.com/playlist?list=PLx0sYbCqOb8":               "https://www.youtube.com/playlist?list=PLx0sYbCqOb8",
		"https://space.bilibili.com/946974":                                "https://space.bilibili.com/946974",
	}
	for in, want := range cases {
		if got := listSource(in); got != want {
			t.Errorf("listSource(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestListVideosFails(t *testing.T) {
	dir := t.TempDir()
	tools := Tools{YTDLP: writeFakeTool(t, dir, "yt-dlp", fakeTool{Stderr: "ERROR: [youtube:tab] PLnope: The playlist does not exist.\n", Exit: 1})}
	_, err := tools.ListVideos(context.Background(), "https://www.youtube.com/playlist?list=PLnope", 0)
	if err == nil || !strings.Contains(err.Error(), "The playlist does not exist.") {
		t.Fatalf("err %v", err)
	}
	if _, err := (Tools{}).ListVideos(context.Background(), "https://www.youtube.com/playlist?list=PLnope", 0); err == nil {
		t.Fatal("listed without yt-dlp")
	}
}

func TestListVideosCanceled(t *testing.T) {
	dir := t.TempDir()
	tools := Tools{YTDLP: writeFakeTool(t, dir, "yt-dlp", fakeTool{Wait: true})}
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)
	start := time.Now()
	_, err := tools.ListVideos(ctx, "https://www.youtube.com/playlist?list=PLx", 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v, want context.Canceled", err)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatal("the listing did not stop with its context")
	}
}

func TestParseListingOneVideo(t *testing.T) {
	if _, err := parseListing([]byte(`{"_type": "video", "id": "x", "title": "One"}`)); err == nil {
		t.Fatal("a video read as a list")
	}
}
