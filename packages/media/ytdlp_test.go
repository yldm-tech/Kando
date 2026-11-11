package media

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeExec(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestFindYTDLPInstallTellsHowItWasInstalled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the install layouts are Unix ones")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Homebrew: bin/yt-dlp links into the Cellar, and bin/brew updates it.
	cellar := filepath.Join(root, "brew", "Cellar", "yt-dlp", "2026.8.19", "bin", "yt-dlp")
	writeExec(t, cellar, "#!/opt/homebrew/opt/python/bin/python3\n")
	writeExec(t, filepath.Join(root, "brew", "bin", "brew"), "#!/bin/sh\n")
	link := filepath.Join(root, "brew", "bin", "yt-dlp")
	if err := os.Symlink(cellar, link); err != nil {
		t.Fatal(err)
	}
	if got := FindYTDLPInstall(link); got.Kind != "homebrew" || strings.Join(got.Update, " ") != filepath.Join(root, "brew", "bin", "brew")+" upgrade yt-dlp" {
		t.Fatalf("homebrew: %+v", got)
	}
	// pip: a Python script outside a Cellar.
	pip := filepath.Join(root, "venv", "bin", "yt-dlp")
	writeExec(t, pip, "#!"+filepath.Join(root, "venv", "bin", "python3")+"\nimport yt_dlp\n")
	if got := FindYTDLPInstall(pip); got.Kind != "pip" || got.Update[0] != filepath.Join(root, "venv", "bin", "python3") || got.Update[len(got.Update)-1] != "yt-dlp" {
		t.Fatalf("pip: %+v", got)
	}
	// The standalone build updates itself.
	standalone := filepath.Join(root, "bin", "yt-dlp")
	writeExec(t, standalone, "\xcf\xfa\xed\xfe binary")
	if got := FindYTDLPInstall(standalone); got.Kind != "standalone" || strings.Join(got.Update, " ") != standalone+" -U" {
		t.Fatalf("standalone: %+v", got)
	}
	// A Python script Kando may not write, as the one a Linux distribution's packages put in /usr/bin, is the system's to update: pip would leave it as it is.
	system := filepath.Join(root, "usr", "bin", "yt-dlp")
	writeExec(t, system, "#!/usr/bin/python3\nimport yt_dlp\n")
	if err := os.Chmod(system, 0o555); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Dir(system), 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(filepath.Dir(system), 0o755) })
	if os.Geteuid() != 0 {
		if got := FindYTDLPInstall(system); got.Kind != "system" || got.Update != nil || got.Dir != filepath.Dir(system) {
			t.Fatalf("system: %+v", got)
		}
		if _, err := UpdateYTDLP(context.Background(), system); !errors.Is(err, ErrCannotUpdate) {
			t.Fatalf("system update: %v", err)
		}
	}
	// One Kando may not replace says how instead.
	if err := os.Chmod(standalone, 0o555); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() != 0 {
		if got := FindYTDLPInstall(standalone); got.Update != nil || got.Dir != filepath.Dir(standalone) {
			t.Fatalf("read-only: %+v", got)
		}
		if _, err := UpdateYTDLP(context.Background(), standalone); !errors.Is(err, ErrCannotUpdate) {
			t.Fatalf("read-only update: %v", err)
		}
	}
}

func TestUpdateYTDLPRunsTheUpdateAndReadsTheNewVersion(t *testing.T) {
	// A standalone yt-dlp that, given -U, replaces its version as the real one replaces itself. It is a file of its own, as FindYTDLPInstall updates the program a link leads to.
	tool := writeFakeTool(t, t.TempDir(), "yt-dlp", fakeTool{Act: "update", Standalone: true})
	if v, err := YTDLPVersion(context.Background(), tool); err != nil || v != "2024.03.10" {
		t.Fatalf("version %q, %v", v, err)
	}
	if v, err := UpdateYTDLP(context.Background(), tool); err != nil || v != "2026.11.03" {
		t.Fatalf("updated to %q, %v", v, err)
	}
}

func TestUpdateYTDLPSaysWhyItCannot(t *testing.T) {
	if _, err := UpdateYTDLP(context.Background(), ""); err == nil {
		t.Fatal("updated a missing yt-dlp")
	}
}

func TestBrowserName(t *testing.T) {
	if BrowserName("chrome") != "Chrome" || BrowserName("safari") != "Safari" || BrowserName("vivaldi") != "vivaldi" {
		t.Fatal("wrong names")
	}
}
