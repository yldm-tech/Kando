package main

import (
	"context"
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
	a, tt, updates := newTestApp(t, media.Tools{})
	release := make(chan struct{})
	dir := fakeKits(t, a, release)

	a.kitsHooks().ready()
	if got := a.kitsDownloading(); strings.Join(got, ",") != "yt-dlp,ffmpeg,whisper-cli" {
		t.Fatalf("downloading %v, want all three programs", got)
	}
	// The sidebar says the programs are on their way rather than missing, and so does the message bar for a link.
	refresh(tt, a)
	if msg := a.readiness().Text(a.cx); !strings.Contains(msg, "正在下载组件") {
		t.Fatalf("status line %q", msg)
	}
	if msg, section := a.precheck([]string{"https://example.com/v"}); !strings.Contains(msg, "还在下载") || section != settings.SectionGeneral {
		t.Fatalf("precheck = %q, %q", msg, section)
	}
	waitUntil(t, tt, a, updates, func() bool {
		p, ok := a.kitsProgress("yt-dlp")
		return ok && p == 0.5
	})

	close(release)
	waitUntil(t, tt, a, updates, func() bool { return len(a.kitsDownloading()) == 0 })
	tools := a.pipeline.(media.Tools)
	for program, path := range map[string]string{"yt-dlp": tools.YTDLP, "ffmpeg": tools.FFmpeg, "whisper-cli": tools.Whisper} {
		kit, _ := media.KitFor(program)
		if want := filepath.Join(dir, kit.ID, program); path != want {
			t.Errorf("%s at %q, want the download %q", program, path, want)
		}
	}
	if msg := a.readiness().Text(a.cx); strings.Contains(msg, "组件") {
		t.Fatalf("status line still about components: %q", msg)
	}
}

// TestKitsDownloadQuietlyWhatTheComputerHas checks that Kando downloads its own copy of a program the computer has too, as it runs its own first, but quietly: neither the sidebar nor the guide show it, and only the program the computer lacks counts as downloading. A kit that is only a fallback (media.Kit.Fallback), as ffmpeg on Linux, is not downloaded while the computer's copy may work.
func TestKitsDownloadQuietlyWhatTheComputerHas(t *testing.T) {
	a, _, _ := newTestApp(t, media.Tools{YTDLP: "/opt/homebrew/bin/yt-dlp", FFmpeg: "/opt/homebrew/bin/ffmpeg"})
	fakeKits(t, a, make(chan struct{}))
	a.kitsHooks().ready()
	if got := a.kitsDownloading(); len(got) != 1 || got[0] != "whisper-cli" {
		t.Fatalf("downloading %v, want whisper-cli alone", got)
	}
	for _, program := range []string{"yt-dlp", "ffmpeg"} {
		kit, _ := media.KitFor(program)
		job := a.kits.running[program]
		if kit.Fallback != (job == nil) || (job != nil && !job.quiet) {
			t.Errorf("%s: download %+v, want a quiet one unless the kit is a fallback (%v)", program, job, kit.Fallback)
		}
		if _, shown := a.kitsProgress(program); shown {
			t.Errorf("%s's quiet download shows its progress", program)
		}
	}
	a.kitsCancel()
}

// TestKitsQuietDownloadThatFailsStaysQuiet checks that a quiet download that failed says nothing and is not started again on its own, but is when the user asks for it.
func TestKitsQuietDownloadThatFailsStaysQuiet(t *testing.T) {
	a, tt, updates := newTestApp(t, media.Tools{YTDLP: "/opt/homebrew/bin/yt-dlp", FFmpeg: "/opt/homebrew/bin/ffmpeg", Whisper: "/opt/homebrew/bin/whisper-cli"})
	release := make(chan struct{})
	close(release)
	fakeKits(t, a, release, "yt-dlp", "ffmpeg", "whisper-cli")
	a.kitsInstallWanted()
	waitUntil(t, tt, a, updates, func() bool { return len(a.kits.running) == 0 })
	if len(a.kits.errs) != 0 || !a.kits.quietFailed["yt-dlp"] {
		t.Fatalf("errs %v, quietly failed %v", a.kits.errs, a.kits.quietFailed)
	}
	a.kitsInstallWanted()
	if len(a.kits.running) != 0 {
		t.Fatalf("a quiet download that failed started again on its own: %v", a.kits.running)
	}
	a.kitsInstall("yt-dlp")
	if job := a.kits.running["yt-dlp"]; job == nil || job.quiet {
		t.Fatalf("the user's download of yt-dlp is %+v, want one that shows", job)
	}
	waitUntil(t, tt, a, updates, func() bool { return len(a.kits.running) == 0 })
	if a.kits.errs["yt-dlp"] == nil {
		t.Fatal("the user's download that failed does not say so")
	}
}

func TestKitsRetryFromSettings(t *testing.T) {
	// Only ffmpeg is missing, and the quiet downloads of the others and of Deno fail too, so that no download succeeds before 重试 and the tools are not looked up again, which would find a Homebrew ffmpeg on the machine running the test.
	a, tt, updates := newTestApp(t, media.Tools{YTDLP: "/usr/local/bin/yt-dlp", Whisper: "/usr/local/bin/whisper-cli"})
	release := make(chan struct{})
	close(release)
	fakeKits(t, a, release, "ffmpeg", "yt-dlp", "whisper-cli", "deno")
	a.kitsInstallWanted()
	waitUntil(t, tt, a, updates, func() bool { return len(a.kits.running) == 0 })
	if got := settings.KitsError(testlocale.Messages(), a.kits.errs["ffmpeg"]); got != "服务器返回 HTTP 503" {
		t.Fatalf("ffmpeg's error says %q, errs %v", got, a.kits.errs)
	}

	st := settingsOpenAt(a, tt, settings.SectionGeneral)
	for _, text := range []string{"下载失败：服务器返回 HTTP 503", "重试"} {
		wantText(t, st, text)
	}
	// It works the second time.
	a.kits.install = func(ctx context.Context, kit media.Kit, dir string, progress func(written, total int64)) (string, error) {
		path := filepath.Join(dir, kit.ID, kit.Program)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		return path, os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755)
	}
	st.Press("id:settings-tool-download-ffmpeg")
	if _, running := a.kitsProgress("ffmpeg"); !running {
		t.Fatal("重试 did not start the download")
	}
	waitUntil(t, tt, a, updates, func() bool { return len(a.kitsDownloading()) == 0 })
	if a.pipeline.(media.Tools).FFmpeg == "" || a.kits.errs["ffmpeg"] != nil {
		t.Fatalf("ffmpeg = %q, errs %v", a.pipeline.(media.Tools).FFmpeg, a.kits.errs)
	}
}

func TestOnboardingShowsTheDownloads(t *testing.T) {
	a, tt, updates := newTestApp(t, media.Tools{})
	release := make(chan struct{})
	fakeKits(t, a, release)
	a.kitsInstallWanted()
	waitUntil(t, tt, a, updates, func() bool {
		p, ok := a.kitsProgress("yt-dlp")
		return ok && p == 0.5
	})
	for _, row := range a.onboardingTools(a.cx) {
		if row.State != onboarding.ToolDownloading {
			t.Fatalf("%s is %v, want downloading", row.Name, row.State)
		}
	}
	close(release)
	waitUntil(t, tt, a, updates, func() bool { return len(a.kitsDownloading()) == 0 })
	for _, row := range a.onboardingTools(a.cx) {
		if row.State != onboarding.ToolReady {
			t.Fatalf("%s is %v once downloaded", row.Name, row.State)
		}
	}
}

func TestOnboardingShowsAFailedDownload(t *testing.T) {
	a, _, _ := newTestApp(t, media.Tools{YTDLP: "/usr/local/bin/yt-dlp", FFmpeg: "/usr/local/bin/ffmpeg"})
	fakeKits(t, a, make(chan struct{}))
	a.kits.errs["whisper-cli"] = rejected(503)
	for _, row := range a.onboardingTools(a.cx) {
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
	// Where Kando cannot download, the guide says how to install it instead: from its download page, or on Windows with Scoop.
	media.OwnToolsDir = ""
	rows := a.onboardingTools(a.cx)
	if rows[2].State != onboarding.ToolUnavailable {
		t.Fatalf("without a folder of its own, whisper-cli is %v", rows[2].State)
	}
	src, _ := media.SourceFor("whisper-cli")
	install := ""
	if src.Scoop != "" {
		install = "scoop install " + src.Scoop
	}
	if src.Page == "" || rows[2].Page != src.Page || rows[2].Install != install {
		t.Fatalf("whisper-cli's page %q and command %q, want %+v", rows[2].Page, rows[2].Install, src)
	}
}

// TestKitsDownloadDenoQuietlyWhenTheComputerHasNone checks that Kando downloads its own Deno, which yt-dlp solves YouTube's challenges with, only when the computer has none, and quietly: Deno is not one of the programs a job needs, so neither the sidebar nor the guide show it.
func TestKitsDownloadDenoQuietlyWhenTheComputerHasNone(t *testing.T) {
	a, _, _ := newTestApp(t, media.Tools{})
	fakeKits(t, a, make(chan struct{}))
	a.kitsHooks().ready()
	if job := a.kits.running["deno"]; job == nil || !job.quiet {
		t.Fatalf("Deno's download is %+v, want a quiet one", job)
	}
	if got := a.kitsDownloading(); strings.Join(got, ",") != "yt-dlp,ffmpeg,whisper-cli" {
		t.Fatalf("downloading %v, want the three programs alone", got)
	}
	a.kitsCancel()

	b, _, _ := newTestApp(t, media.Tools{})
	fakeKits(t, b, make(chan struct{}))
	b.tools.candidates = func(program string) []string {
		if program == "deno" {
			return []string{"/opt/homebrew/bin/deno"}
		}
		return nil
	}
	b.kitsHooks().ready()
	if job := b.kits.running["deno"]; job != nil {
		t.Fatalf("Deno downloads although the computer has one: %+v", job)
	}
	b.kitsCancel()
}
