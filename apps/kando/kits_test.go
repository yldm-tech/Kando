package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/onboarding"
	"kando/apps/kando/settings"
	"kando/packages/media"
)

// fakeKits points Kando's own folder at a temporary one and replaces the download with one that writes a placeholder program there, after release says so. fail names programs whose download fails instead.
func fakeKits(t *testing.T, a *app, release <-chan struct{}, fail ...string) string {
	t.Helper()
	if _, ok := media.KitFor("ffmpeg"); !ok {
		t.Skip("Kando downloads no programs on this platform")
	}
	dir := t.TempDir()
	saved := media.OwnToolsDir
	media.OwnToolsDir = dir
	t.Cleanup(func() { media.OwnToolsDir = saved })
	// Programs on PATH or in Homebrew's folder would be found in place of the downloads.
	t.Setenv("PATH", t.TempDir())
	a.kits.install = func(ctx context.Context, kit media.Kit, dir string, progress func(written, total int64)) (string, error) {
		progress(kit.Size/2, kit.Size)
		select {
		case <-release:
		case <-ctx.Done():
			return "", ctx.Err()
		}
		for _, name := range fail {
			if name == kit.Program {
				return "", rejected(503)
			}
		}
		path := filepath.Join(dir, kit.ID, kit.Program)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		return path, os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755)
	}
	return dir
}

func TestKitsDownloadWhatIsMissingWhenReady(t *testing.T) {
	a, tt, updates := newTestApp(media.Tools{})
	release := make(chan struct{})
	dir := fakeKits(t, a, release)

	a.kitsHooks().ready()
	if got := a.kitsDownloading(); strings.Join(got, ",") != "yt-dlp,ffmpeg,whisper-cli" {
		t.Fatalf("downloading %v, want all three programs", got)
	}
	// The sidebar says the programs are on their way rather than missing, and so does the message bar for a link.
	tt.Frame()
	if msg := a.readiness().Message(); !strings.Contains(msg, "正在下载组件") {
		t.Fatalf("status line %q", msg)
	}
	if msg, section := a.precheck([]string{"https://example.com/v"}); !strings.Contains(msg, "还在下载") || section != settings.SectionGeneral {
		t.Fatalf("precheck = %q, %q", msg, section)
	}
	waitUntil(t, updates, func() bool {
		p, ok := a.kitsProgress("yt-dlp")
		return ok && p == 0.5
	})

	close(release)
	waitUntil(t, updates, func() bool { return len(a.kitsDownloading()) == 0 })
	tools := a.pipeline.(media.Tools)
	for program, path := range map[string]string{"yt-dlp": tools.YTDLP, "ffmpeg": tools.FFmpeg, "whisper-cli": tools.Whisper} {
		kit, _ := media.KitFor(program)
		if want := filepath.Join(dir, kit.ID, program); path != want {
			t.Errorf("%s at %q, want the download %q", program, path, want)
		}
	}
	if msg := a.readiness().Message(); strings.Contains(msg, "组件") {
		t.Fatalf("status line still about components: %q", msg)
	}
}

func TestKitsLeaveInstalledProgramsAlone(t *testing.T) {
	a, _, _ := newTestApp(media.Tools{YTDLP: "/opt/homebrew/bin/yt-dlp", FFmpeg: "/opt/homebrew/bin/ffmpeg"})
	fakeKits(t, a, make(chan struct{}))
	a.kitsHooks().ready()
	if got := a.kitsDownloading(); len(got) != 1 || got[0] != "whisper-cli" {
		t.Fatalf("downloading %v, want whisper-cli alone", got)
	}
	a.kitsCancel()
}

func TestKitsRetryFromSettings(t *testing.T) {
	// Only ffmpeg is missing, so that no download succeeds before 重试 and the tools are not looked up again, which would find a Homebrew ffmpeg on the machine running the test.
	a, _, updates := newTestApp(media.Tools{YTDLP: "/usr/local/bin/yt-dlp", Whisper: "/usr/local/bin/whisper-cli"})
	release := make(chan struct{})
	close(release)
	fakeKits(t, a, release, "ffmpeg")
	a.kitsInstallMissing()
	waitUntil(t, updates, func() bool { return len(a.kitsDownloading()) == 0 })
	if got := kitsError(a.kits.errs["ffmpeg"]); got != "服务器返回 HTTP 503" {
		t.Fatalf("ffmpeg's error says %q, errs %v", got, a.kits.errs)
	}

	st := settingsOpenAt(a, nil, settings.SectionGeneral)
	for _, text := range []string{"下载失败：服务器返回 HTTP 503", "重试"} {
		if !st.HasText(text) {
			t.Fatalf("settings lack %q: %v", text, st.Texts())
		}
	}
	// It works the second time.
	a.kits.install = func(ctx context.Context, kit media.Kit, dir string, progress func(written, total int64)) (string, error) {
		path := filepath.Join(dir, kit.ID, kit.Program)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		return path, os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755)
	}
	if err := st.Click("重试"); err != nil {
		t.Fatal(err)
	}
	if _, running := a.kitsProgress("ffmpeg"); !running {
		t.Fatal("重试 did not start the download")
	}
	waitUntil(t, updates, func() bool { return len(a.kitsDownloading()) == 0 })
	if a.pipeline.(media.Tools).FFmpeg == "" || a.kits.errs["ffmpeg"] != nil {
		t.Fatalf("ffmpeg = %q, errs %v", a.pipeline.(media.Tools).FFmpeg, a.kits.errs)
	}
}

func TestOnboardingShowsTheDownloads(t *testing.T) {
	a, _, updates := newTestApp(media.Tools{})
	release := make(chan struct{})
	fakeKits(t, a, release)
	a.kitsInstallMissing()
	waitUntil(t, updates, func() bool {
		p, ok := a.kitsProgress("yt-dlp")
		return ok && p == 0.5
	})
	for _, row := range a.onboardingTools() {
		if row.State != onboarding.ToolDownloading {
			t.Fatalf("%s is %v, want downloading", row.Name, row.State)
		}
	}
	close(release)
	waitUntil(t, updates, func() bool { return len(a.kitsDownloading()) == 0 })
	for _, row := range a.onboardingTools() {
		if row.State != onboarding.ToolReady {
			t.Fatalf("%s is %v once downloaded", row.Name, row.State)
		}
	}
}

func TestOnboardingShowsAFailedDownload(t *testing.T) {
	a, _, _ := newTestApp(media.Tools{YTDLP: "/usr/local/bin/yt-dlp", FFmpeg: "/usr/local/bin/ffmpeg"})
	fakeKits(t, a, make(chan struct{}))
	a.kits.errs["whisper-cli"] = rejected(503)
	for _, row := range a.onboardingTools() {
		want := onboarding.ToolReady
		if row.Name == "whisper-cli" {
			want = onboarding.ToolMissing
			if row.Err != "服务器返回 HTTP 503" {
				t.Fatalf("whisper-cli's error = %q", row.Err)
			}
		}
		if row.State != want {
			t.Fatalf("%s is %v, want %v", row.Name, row.State, want)
		}
	}
	// Where Kando cannot download, the guide says how to install it instead.
	media.OwnToolsDir = ""
	if rows := a.onboardingTools(); rows[2].State != onboarding.ToolUnavailable {
		t.Fatalf("without a folder of its own, whisper-cli is %v", rows[2].State)
	}
}

// TestKitsErrorSaysWhy checks the words for why a program's download failed: a stall, an HTTP status and a dropped connection each in plain words of the interface's language, anything else as it is.
func TestKitsErrorSaysWhy(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{nil, ""},
		{media.ErrStalled, "服务器一直没有发来数据"},
		{rejected(404), "服务器返回 HTTP 404"},
		{fmt.Errorf("%w: read: connection reset by peer", media.ErrDisconnected), "连接断了"},
		{errors.New("download ffmpeg: checksum 00, want 11"), "download ffmpeg: checksum 00, want 11"},
	} {
		if got := kitsError(tc.err); got != tc.want {
			t.Errorf("kitsError(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
	testlocale.Use(t, "en")
	if got := kitsError(media.ErrStalled); got != "the server stopped sending data" {
		t.Errorf("in English, a stall is %q", got)
	}
}
