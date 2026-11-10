package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kando/apps/kando/clitool"
	"kando/apps/kando/settings"
)

// cliToolRoot is a folder for one test's executables and install folders, with its links resolved, as clitool.Look resolves the executable's.
func cliToolRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// cliToolFakeApp writes an executable named Kando in root/app, as the one inside Kando.app, and returns its path.
func cliToolFakeApp(t *testing.T, root string) string {
	t.Helper()
	exe := filepath.Join(root, "app", "Kando")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe
}

func cliToolMkdir(t *testing.T, dir string, mode os.FileMode) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, mode); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	return dir
}

func wantLink(t *testing.T, link, target string) {
	t.Helper()
	got, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	if got != target {
		t.Fatalf("%s points at %s, want %s", link, got, target)
	}
}

// cliToolDiskImage makes every volume read only, as the disk image Kando comes in is, for the test.
func cliToolDiskImage(t *testing.T, readOnly bool) {
	t.Helper()
	saved := clitool.ReadOnly
	clitool.ReadOnly = func(string) bool { return readOnly }
	t.Cleanup(func() { clitool.ReadOnly = saved })
}

func TestSettingsGeneralAsksToMoveATemporaryApp(t *testing.T) {
	cliToolDiskImage(t, true)
	root := cliToolRoot(t)
	bin := cliToolMkdir(t, filepath.Join(root, "bin"), 0o755)
	exe := "/Volumes/Kando/Kando.app/Contents/MacOS/Kando"
	a, tt, updates := newTestApp(&fakePipeline{})
	a.cliTool.env = func() clitool.Env { return clitool.Env{Exe: exe, Searched: []string{bin}, InstallTo: []string{bin}} }
	tt = settingsOpenAt(a, tt, settings.SectionGeneral)
	tt.SetSize(880, 1600)
	waitUntil(t, updates, func() bool { return a.cliTool.known })
	tt.Frame()
	if !tt.HasText("看懂正从磁盘映像或临时位置运行。请先把看懂移到「应用程序」文件夹，再从那里打开。") {
		t.Fatalf("no hint to move the app: %q", tt.Texts())
	}
	for _, text := range tt.Texts() {
		if strings.Contains(text, "/Volumes/") {
			t.Fatalf("the card gives out the temporary path: %q", text)
		}
		if text == "安装" || text == "重新安装" {
			t.Fatalf("the card offers to install: %q", tt.Texts())
		}
	}
	tt.Click("拷贝配置")
	tt.Click("拷贝命令")
	tt.Frame()
	if tt.Clipboard() != "" {
		t.Fatalf("copied %q", tt.Clipboard())
	}
}

func TestSettingsGeneralInstallsTheCommandAndCopiesTheMCPConfig(t *testing.T) {
	root := cliToolRoot(t)
	exe := cliToolFakeApp(t, root)
	bin := cliToolMkdir(t, filepath.Join(root, "bin"), 0o755)
	a, tt, updates := newTestApp(&fakePipeline{})
	a.cliTool.env = func() clitool.Env {
		return clitool.Env{Exe: exe, Searched: []string{bin}, InstallTo: []string{bin}}
	}
	tt = settingsOpenAt(a, tt, settings.SectionGeneral)
	tt.SetSize(880, 1600)
	waitUntil(t, updates, func() bool { return a.cliTool.known })
	tt.Frame()
	for _, text := range []string{"命令行与 MCP", "命令行工具", "在终端里用 kando 命令添加视频、查看任务和字幕。", "MCP", "Claude Code", "claude mcp add kando -- " + exe + " mcp"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}

	if err := tt.Click("安装"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, updates, func() bool { return !a.cliTool.installing })
	tt.Frame()
	link := filepath.Join(bin, "kando")
	wantLink(t, link, exe)
	if !tt.HasText("已安装") || !tt.HasText("在终端里输入 kando 即可使用，命令在 "+link+"。") {
		t.Fatalf("after 安装: %q", tt.Texts())
	}

	if err := tt.Click("拷贝配置"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.Clipboard() != clitool.MCPConfig(link) || !strings.Contains(tt.Clipboard(), `"command": "`+link+`"`) {
		t.Fatalf("clipboard = %s", tt.Clipboard())
	}
	if !tt.HasText("已拷贝") {
		t.Fatalf("拷贝配置 does not say it copied: %q", tt.Texts())
	}
	if err := tt.Click("拷贝命令"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.Clipboard() != "claude mcp add kando -- "+link+" mcp" {
		t.Fatalf("clipboard = %s", tt.Clipboard())
	}
}

func TestSettingsGeneralSaysWhenTheCommandIsNotOnPath(t *testing.T) {
	root := cliToolRoot(t)
	exe := cliToolFakeApp(t, root)
	own := filepath.Join(root, "own", "bin")
	a, tt, updates := newTestApp(&fakePipeline{})
	a.cliTool.env = func() clitool.Env {
		return clitool.Env{Exe: exe, InstallTo: []string{own}, Own: own}
	}
	tt = settingsOpenAt(a, tt, settings.SectionGeneral)
	tt.SetSize(880, 1600)
	waitUntil(t, updates, func() bool { return a.cliTool.known })
	tt.Frame()
	if err := tt.Click("安装"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, updates, func() bool { return !a.cliTool.installing })
	tt.Frame()
	link := filepath.Join(own, "kando")
	wantLink(t, link, exe)
	if want := "已安装到 " + link + "，但终端的 PATH 里还没有 " + own + "。把它加进 PATH，再打开一个新的终端窗口。"; !tt.HasText(want) {
		t.Fatalf("missing %q in %q", want, tt.Texts())
	}
	if !tt.HasText("重新安装") || a.CLI().Installed {
		t.Fatalf("a command a terminal does not find shows as installed: %q", tt.Texts())
	}
}

func TestSettingsGeneralShowsWhyInstallFailed(t *testing.T) {
	root := cliToolRoot(t)
	exe := cliToolFakeApp(t, root)
	bin := cliToolMkdir(t, filepath.Join(root, "bin"), 0o755)
	if err := os.WriteFile(filepath.Join(bin, "kando"), []byte("other"), 0o755); err != nil {
		t.Fatal(err)
	}
	a, tt, updates := newTestApp(&fakePipeline{})
	a.cliTool.env = func() clitool.Env { return clitool.Env{Exe: exe, Searched: []string{bin}, InstallTo: []string{bin}} }
	tt = settingsOpenAt(a, tt, settings.SectionGeneral)
	tt.SetSize(880, 1600)
	waitUntil(t, updates, func() bool { return a.cliTool.known })
	tt.Frame()
	if !tt.HasText("终端里的 kando 是 " + filepath.Join(bin, "kando") + "，不是这个看懂。") {
		t.Fatalf("the other kando is not named: %q", tt.Texts())
	}
	if err := tt.Click("重新安装"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, updates, func() bool { return !a.cliTool.installing })
	tt.Frame()
	if !tt.HasText("没能安装：" + filepath.Join(bin, "kando") + " 是别的程序的 kando，看懂不会替换它。") {
		t.Fatalf("no reason shown: %q", tt.Texts())
	}
}
