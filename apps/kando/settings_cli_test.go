package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"kando/apps/kando/settings"
)

// cliToolRoot is a folder for one test's executables and install folders, with its links resolved, as cliToolLook resolves the executable's.
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

func TestCLIToolInstallLinksIntoTheFirstWritableFolder(t *testing.T) {
	root := cliToolRoot(t)
	exe := cliToolFakeApp(t, root)
	missing := filepath.Join(root, "opt", "homebrew", "bin")
	locked := cliToolMkdir(t, filepath.Join(root, "usr", "local", "bin"), 0o555)
	own := filepath.Join(root, "home", ".local", "bin")
	env := cliToolEnv{exe: exe, installTo: []string{locked, missing, own}, own: own}
	if os.Geteuid() == 0 {
		t.Skip("root writes into a read-only folder")
	}
	link, err := cliToolInstall(env, exe)
	if err != nil {
		t.Fatal(err)
	}
	if link != filepath.Join(own, "kando") {
		t.Fatalf("installed at %s, want in the user's own folder %s, made for it", link, own)
	}
	wantLink(t, link, exe)
	if _, err := os.Stat(missing); err == nil {
		t.Fatalf("%s was made; only the user's own folder may be", missing)
	}
	entries, _ := os.ReadDir(own)
	if len(entries) != 1 {
		t.Fatalf("%s has %d entries, want the link alone", own, len(entries))
	}

	// With the system folder writable it goes there, first.
	os.Chmod(locked, 0o755)
	link, err = cliToolInstall(env, exe)
	if err != nil || link != filepath.Join(locked, "kando") {
		t.Fatalf("installed at %s (%v), want %s", link, err, filepath.Join(locked, "kando"))
	}
	wantLink(t, link, exe)
}

func TestCLIToolInstallRefusesAnotherProgram(t *testing.T) {
	root := cliToolRoot(t)
	exe := cliToolFakeApp(t, root)
	bin := cliToolMkdir(t, filepath.Join(root, "bin"), 0o755)
	file := filepath.Join(bin, "kando")
	if err := os.WriteFile(file, []byte("someone else's kando"), 0o755); err != nil {
		t.Fatal(err)
	}
	other := cliToolMkdir(t, filepath.Join(root, "other"), 0o755)
	if err := os.Symlink("/usr/bin/true", filepath.Join(other, "kando")); err != nil {
		t.Fatal(err)
	}
	_, err := cliToolInstall(cliToolEnv{exe: exe, installTo: []string{bin, other}}, exe)
	if err == nil || !strings.Contains(err.Error(), file) || !strings.Contains(err.Error(), filepath.Join(other, "kando")) {
		t.Fatalf("err = %v, want one naming both commands that are not Kando's", err)
	}
	if data, _ := os.ReadFile(file); string(data) != "someone else's kando" {
		t.Fatalf("the other program's file now holds %q", data)
	}
	wantLink(t, filepath.Join(other, "kando"), "/usr/bin/true")
}

// cliToolFakeFile writes an executable at path, making its folders.
func cliToolFakeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestCLIToolInstallRefusesAPackageManagersKandoLink(t *testing.T) {
	root := cliToolRoot(t)
	exe := cliToolFakeApp(t, root)
	// Links named kando to files named kando that belong to other programs: npm's into its global node_modules, Homebrew's into its Cellar, each relative to the link as they make them, and one of them dangling.
	links := map[string]string{
		"npm":      "../lib/node_modules/kando-cli/bin/kando",
		"homebrew": "../Cellar/kando/1.0/bin/kando",
		"gone":     "../lib/node_modules/gone/bin/kando",
	}
	var dirs []string
	for name, target := range links {
		bin := cliToolMkdir(t, filepath.Join(root, name, "bin"), 0o755)
		if name != "gone" {
			cliToolFakeFile(t, filepath.Join(bin, target))
		}
		if err := os.Symlink(target, filepath.Join(bin, "kando")); err != nil {
			t.Fatal(err)
		}
		dirs = append(dirs, bin)
	}
	_, err := cliToolInstall(cliToolEnv{exe: exe, installTo: dirs}, exe)
	var blocked *cliToolBlockedError
	if !errors.As(err, &blocked) || len(blocked.paths) != len(dirs) {
		t.Fatalf("err = %v, want one naming every other program's kando", err)
	}
	for name, target := range links {
		wantLink(t, filepath.Join(root, name, "bin", "kando"), target)
	}
}

func TestCLIToolInstallReplacesAnotherKando(t *testing.T) {
	root := cliToolRoot(t)
	exe := cliToolFakeApp(t, root)
	own := filepath.Join(root, "home", ".local", "bin")
	installed := filepath.Join(root, "home", ".local", "kando.app", "kando")
	cliToolFakeFile(t, installed)
	bundle := filepath.Join(root, "Old", "Kando.app", "Contents", "MacOS", "Kando")
	cliToolFakeFile(t, bundle)
	for name, target := range map[string]string{
		// Another copy of the app, as an older one left in Downloads.
		"bundle": bundle,
		// The app MyGo's install script puts in ~/.local/kando.app.
		"script": installed,
		// The app of MyGo's .deb and PKGBUILD, which need not be there.
		"package": "/opt/kando/kando",
		// This app itself, through a relative link.
		"self": "../app/Kando",
	} {
		t.Run(name, func(t *testing.T) {
			bin := cliToolMkdir(t, filepath.Join(root, name), 0o755)
			if err := os.Symlink(target, filepath.Join(bin, "kando")); err != nil {
				t.Fatal(err)
			}
			link, err := cliToolInstall(cliToolEnv{exe: exe, installTo: []string{bin}, own: own}, exe)
			if err != nil {
				t.Fatal(err)
			}
			wantLink(t, link, exe)
		})
	}
}

// cliToolDiskImage makes every volume read only, as the disk image Kando comes in is, for the test.
func cliToolDiskImage(t *testing.T, readOnly bool) {
	t.Helper()
	saved := cliToolReadOnly
	cliToolReadOnly = func(string) bool { return readOnly }
	t.Cleanup(func() { cliToolReadOnly = saved })
}

func TestCLIToolRefusesATemporaryApp(t *testing.T) {
	cliToolDiskImage(t, true)
	root := cliToolRoot(t)
	for _, exe := range []string{
		"/Volumes/Kando 0.1.0/Kando.app/Contents/MacOS/Kando",
		"/private/var/folders/xy/abc/T/AppTranslocation/0F2E-41A7/d/Kando.app/Contents/MacOS/Kando",
	} {
		bin := cliToolMkdir(t, filepath.Join(root, strconv.Itoa(len(exe))), 0o755)
		env := cliToolEnv{exe: exe, searched: []string{bin}, installTo: []string{bin}}
		st := cliToolLook(env)
		if !st.temporary || st.cliToolCommand() != "" || cliToolMCPConfig(st.cliToolCommand()) != "" || cliToolClaudeCommand(st.cliToolCommand()) != "" {
			t.Fatalf("%s: %+v gives MCP clients %q", exe, st, st.cliToolCommand())
		}
		if _, err := cliToolInstall(env, st.exe); !errors.Is(err, errCLIToolTemporary) {
			t.Fatalf("%s: err = %v, want errCLIToolTemporary", exe, err)
		}
		if entries, _ := os.ReadDir(bin); len(entries) != 0 {
			t.Fatalf("%s: installing left %d entries in %s", exe, len(entries), bin)
		}
	}
	// An app in /Applications, or any folder that stays, is not temporary.
	if cliToolTemporary("/Applications/Kando.app/Contents/MacOS/Kando") || cliToolTemporary("/opt/kando/kando") {
		t.Fatal("a lasting place counts as temporary")
	}
	// Nor is one kept on an external disk, which can be written, unlike a disk image.
	cliToolDiskImage(t, false)
	if cliToolTemporary("/Volumes/External/Apps/Kando.app/Contents/MacOS/Kando") {
		t.Fatal("an app on a writable external disk counts as temporary")
	}
}

func TestSettingsGeneralAsksToMoveATemporaryApp(t *testing.T) {
	cliToolDiskImage(t, true)
	root := cliToolRoot(t)
	bin := cliToolMkdir(t, filepath.Join(root, "bin"), 0o755)
	exe := "/Volumes/Kando/Kando.app/Contents/MacOS/Kando"
	a, tt, updates := newTestApp(&fakePipeline{})
	a.cliTool.env = func() cliToolEnv { return cliToolEnv{exe: exe, searched: []string{bin}, installTo: []string{bin}} }
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

func TestCLIToolInstallReplacesAStaleKandoLink(t *testing.T) {
	root := cliToolRoot(t)
	exe := cliToolFakeApp(t, root)
	bin := cliToolMkdir(t, filepath.Join(root, "bin"), 0o755)
	stale := "/Users/someone/.Trash/Kando.app/Contents/MacOS/Kando"
	if err := os.Symlink(stale, filepath.Join(bin, "kando")); err != nil {
		t.Fatal(err)
	}
	env := cliToolEnv{exe: exe, searched: []string{bin}, installTo: []string{bin}}
	if st := cliToolLook(env); st.command != "" || st.ours {
		t.Fatalf("a dangling link counts as a command: %+v", st)
	}
	link, err := cliToolInstall(env, exe)
	if err != nil {
		t.Fatal(err)
	}
	wantLink(t, link, exe)
	if st := cliToolLook(env); !st.ours || st.command != link || st.link != link {
		t.Fatalf("after installing: %+v", st)
	}
}

func TestCLIToolLookFindsWhatATerminalRuns(t *testing.T) {
	root := cliToolRoot(t)
	exe := cliToolFakeApp(t, root)
	// The app runs through a link of its own, as Kando.app's executable seen through /Applications on a moved volume.
	through := filepath.Join(root, "through")
	if err := os.Symlink(filepath.Dir(exe), through); err != nil {
		t.Fatal(err)
	}
	first := cliToolMkdir(t, filepath.Join(root, "first"), 0o755)
	second := cliToolMkdir(t, filepath.Join(root, "second"), 0o755)
	own := cliToolMkdir(t, filepath.Join(root, "own"), 0o755)
	env := cliToolEnv{exe: filepath.Join(through, "Kando"), searched: []string{first, second}, installTo: []string{own}, own: own}

	st := cliToolLook(env)
	if st.exe != exe || st.command != "" || st.ours || st.link != "" || !st.installable {
		t.Fatalf("with no kando: %+v", st)
	}

	if err := os.Symlink(exe, filepath.Join(second, "kando")); err != nil {
		t.Fatal(err)
	}
	if st := cliToolLook(env); !st.ours || st.command != filepath.Join(second, "kando") {
		t.Fatalf("with this app's kando on PATH: %+v", st)
	}

	// Another program earlier on PATH wins, and a Kando link that a terminal does not search is still found.
	if err := os.WriteFile(filepath.Join(first, "kando"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(exe, filepath.Join(own, "kando")); err != nil {
		t.Fatal(err)
	}
	st = cliToolLook(env)
	if st.ours || st.command != filepath.Join(first, "kando") || st.link != filepath.Join(own, "kando") {
		t.Fatalf("with another kando first on PATH: %+v", st)
	}
	if st.cliToolCommand() != filepath.Join(own, "kando") {
		t.Fatalf("MCP clients would run %s", st.cliToolCommand())
	}

	// A file that is not executable is not a command.
	os.Chmod(filepath.Join(first, "kando"), 0o644)
	if st := cliToolLook(env); !st.ours {
		t.Fatalf("a kando that cannot run hides this app's: %+v", st)
	}
}

func TestCLIToolSnippets(t *testing.T) {
	var config struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(cliToolMCPConfig("/usr/local/bin/kando")), &config); err != nil {
		t.Fatal(err)
	}
	server := config.MCPServers["kando"]
	if len(config.MCPServers) != 1 || server.Command != "/usr/local/bin/kando" || strings.Join(server.Args, " ") != "mcp" {
		t.Fatalf("config = %+v", config)
	}
	for path, want := range map[string]string{
		"/usr/local/bin/kando":                            "claude mcp add kando -- /usr/local/bin/kando mcp",
		"/Volumes/My Apps/Kando.app/Contents/MacOS/Kando": "claude mcp add kando -- '/Volumes/My Apps/Kando.app/Contents/MacOS/Kando' mcp",
		"/tmp/it's/kando":                                 `claude mcp add kando -- '/tmp/it'\''s/kando' mcp`,
	} {
		if got := cliToolClaudeCommand(path); got != want {
			t.Fatalf("command for %s = %s, want %s", path, got, want)
		}
	}
}

func TestSettingsGeneralInstallsTheCommandAndCopiesTheMCPConfig(t *testing.T) {
	root := cliToolRoot(t)
	exe := cliToolFakeApp(t, root)
	bin := cliToolMkdir(t, filepath.Join(root, "bin"), 0o755)
	a, tt, updates := newTestApp(&fakePipeline{})
	a.cliTool.env = func() cliToolEnv {
		return cliToolEnv{exe: exe, searched: []string{bin}, installTo: []string{bin}}
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
	if tt.Clipboard() != cliToolMCPConfig(link) || !strings.Contains(tt.Clipboard(), `"command": "`+link+`"`) {
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
	a.cliTool.env = func() cliToolEnv {
		return cliToolEnv{exe: exe, installTo: []string{own}, own: own}
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
	a.cliTool.env = func() cliToolEnv { return cliToolEnv{exe: exe, searched: []string{bin}, installTo: []string{bin}} }
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
