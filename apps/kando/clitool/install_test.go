package clitool

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// cliToolRoot is a folder for one test's executables and install folders, with its links resolved, as Look resolves the executable's.
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

// cliToolUnixOnly skips a test of the links Install makes in a folder on PATH, and of Look finding them, on Windows, which has no folders to install in (EnvHere): there Kando.exe in a folder on PATH is the command, which TestCLIToolLookOnWindows covers.
func cliToolUnixOnly(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Windows installs no kando link; TestCLIToolLookOnWindows covers its command")
	}
}

func TestCLIToolInstallLinksIntoTheFirstWritableFolder(t *testing.T) {
	cliToolUnixOnly(t)
	root := cliToolRoot(t)
	exe := cliToolFakeApp(t, root)
	missing := filepath.Join(root, "opt", "homebrew", "bin")
	locked := cliToolMkdir(t, filepath.Join(root, "usr", "local", "bin"), 0o555)
	own := filepath.Join(root, "home", ".local", "bin")
	env := Env{Exe: exe, InstallTo: []string{locked, missing, own}, Own: own}
	if os.Geteuid() == 0 {
		t.Skip("root writes into a read-only folder")
	}
	link, err := Install(env, exe)
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
	link, err = Install(env, exe)
	if err != nil || link != filepath.Join(locked, "kando") {
		t.Fatalf("installed at %s (%v), want %s", link, err, filepath.Join(locked, "kando"))
	}
	wantLink(t, link, exe)
}

func TestCLIToolInstallRefusesAnotherProgram(t *testing.T) {
	cliToolUnixOnly(t)
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
	_, err := Install(Env{Exe: exe, InstallTo: []string{bin, other}}, exe)
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
	cliToolUnixOnly(t)
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
	_, err := Install(Env{Exe: exe, InstallTo: dirs}, exe)
	var blocked *BlockedError
	if !errors.As(err, &blocked) || len(blocked.Paths) != len(dirs) {
		t.Fatalf("err = %v, want one naming every other program's kando", err)
	}
	for name, target := range links {
		wantLink(t, filepath.Join(root, name, "bin", "kando"), target)
	}
}

func TestCLIToolInstallReplacesAnotherKando(t *testing.T) {
	cliToolUnixOnly(t)
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
			link, err := Install(Env{Exe: exe, InstallTo: []string{bin}, Own: own}, exe)
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
	saved := ReadOnly
	ReadOnly = func(string) bool { return readOnly }
	t.Cleanup(func() { ReadOnly = saved })
}

func TestCLIToolRefusesATemporaryApp(t *testing.T) {
	cliToolDiskImage(t, true)
	root := cliToolRoot(t)
	for _, exe := range []string{
		"/Volumes/Kando 0.1.0/Kando.app/Contents/MacOS/Kando",
		"/private/var/folders/xy/abc/T/AppTranslocation/0F2E-41A7/d/Kando.app/Contents/MacOS/Kando",
	} {
		bin := cliToolMkdir(t, filepath.Join(root, strconv.Itoa(len(exe))), 0o755)
		env := Env{Exe: exe, Searched: []string{bin}, InstallTo: []string{bin}}
		st := Look(env)
		if !st.Temporary || st.MCPCommand() != "" || MCPConfig(st.MCPCommand()) != "" || ClaudeCommand(st.MCPCommand()) != "" {
			t.Fatalf("%s: %+v gives MCP clients %q", exe, st, st.MCPCommand())
		}
		if _, err := Install(env, st.Exe); !errors.Is(err, ErrTemporary) {
			t.Fatalf("%s: err = %v, want ErrTemporary", exe, err)
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

func TestCLIToolInstallReplacesAStaleKandoLink(t *testing.T) {
	cliToolUnixOnly(t)
	root := cliToolRoot(t)
	exe := cliToolFakeApp(t, root)
	bin := cliToolMkdir(t, filepath.Join(root, "bin"), 0o755)
	stale := "/Users/someone/.Trash/Kando.app/Contents/MacOS/Kando"
	if err := os.Symlink(stale, filepath.Join(bin, "kando")); err != nil {
		t.Fatal(err)
	}
	env := Env{Exe: exe, Searched: []string{bin}, InstallTo: []string{bin}}
	if st := Look(env); st.Command != "" || st.Ours {
		t.Fatalf("a dangling link counts as a command: %+v", st)
	}
	link, err := Install(env, exe)
	if err != nil {
		t.Fatal(err)
	}
	wantLink(t, link, exe)
	if st := Look(env); !st.Ours || st.Command != link || st.Link != link {
		t.Fatalf("after installing: %+v", st)
	}
}

func TestCLIToolLookFindsWhatATerminalRuns(t *testing.T) {
	cliToolUnixOnly(t)
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
	env := Env{Exe: filepath.Join(through, "Kando"), Searched: []string{first, second}, InstallTo: []string{own}, Own: own}

	st := Look(env)
	if st.Exe != exe || st.Command != "" || st.Ours || st.Link != "" || !st.Installable {
		t.Fatalf("with no kando: %+v", st)
	}

	if err := os.Symlink(exe, filepath.Join(second, "kando")); err != nil {
		t.Fatal(err)
	}
	if st := Look(env); !st.Ours || st.Command != filepath.Join(second, "kando") {
		t.Fatalf("with this app's kando on PATH: %+v", st)
	}

	// Another program earlier on PATH wins, and a Kando link that a terminal does not search is still found.
	if err := os.WriteFile(filepath.Join(first, "kando"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(exe, filepath.Join(own, "kando")); err != nil {
		t.Fatal(err)
	}
	st = Look(env)
	if st.Ours || st.Command != filepath.Join(first, "kando") || st.Link != filepath.Join(own, "kando") {
		t.Fatalf("with another kando first on PATH: %+v", st)
	}
	if st.MCPCommand() != filepath.Join(own, "kando") {
		t.Fatalf("MCP clients would run %s", st.MCPCommand())
	}

	// A file that is not executable is not a command.
	os.Chmod(filepath.Join(first, "kando"), 0o644)
	if st := Look(env); !st.Ours {
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
	if err := json.Unmarshal([]byte(MCPConfig("/usr/local/bin/kando")), &config); err != nil {
		t.Fatal(err)
	}
	server := config.MCPServers["kando"]
	if len(config.MCPServers) != 1 || server.Command != "/usr/local/bin/kando" || strings.Join(server.Args, " ") != "mcp" {
		t.Fatalf("config = %+v", config)
	}
	cases := map[string]string{
		"/usr/local/bin/kando":                            "claude mcp add kando -- /usr/local/bin/kando mcp",
		"/Volumes/My Apps/Kando.app/Contents/MacOS/Kando": "claude mcp add kando -- '/Volumes/My Apps/Kando.app/Contents/MacOS/Kando' mcp",
		"/tmp/it's/kando":                                 `claude mcp add kando -- '/tmp/it'\''s/kando' mcp`,
	}
	if runtime.GOOS == "windows" {
		// The command is pasted into PowerShell, cmd.exe or Git Bash: all three take a double-quoted path, and cmd.exe no single-quoted one.
		cases = map[string]string{
			`C:\Users\me\AppData\Local\Programs\Kando\Kando.exe`: `claude mcp add kando -- "C:\Users\me\AppData\Local\Programs\Kando\Kando.exe" mcp`,
			`C:\Users\Jo Smith\it's\Kando.exe`:                   `claude mcp add kando -- "C:\Users\Jo Smith\it's\Kando.exe" mcp`,
		}
	}
	for path, want := range cases {
		if got := ClaudeCommand(path); got != want {
			t.Fatalf("command for %s = %s, want %s", path, got, want)
		}
	}
}

func TestCLIToolLookOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows finds commands by PATHEXT; Unix's are the links TestCLIToolLookFindsWhatATerminalRuns covers")
	}
	if env := EnvHere(); len(env.InstallTo) != 0 || Look(env).Installable {
		t.Fatalf("Windows offers folders to install in: %+v", env)
	}
	t.Setenv("PATHEXT", ".COM;.EXE;.BAT;.CMD")
	root := cliToolRoot(t)
	exe := filepath.Join(root, "app", "Kando.exe")
	cliToolFakeFile(t, exe)
	first := cliToolMkdir(t, filepath.Join(root, "first"), 0o755)
	env := Env{Exe: exe, Searched: []string{first}}
	if _, err := Install(env, exe); err == nil {
		t.Fatal("Install made a command on Windows")
	}

	st := Look(env)
	if st.Exe != exe || st.Command != "" || st.Ours || st.Link != "" || st.Installable || st.MCPCommand() != exe {
		t.Fatalf("with no kando: %+v", st)
	}

	// The app's own folder on PATH makes Kando.exe the kando command, as Windows ignores case.
	env.Searched = append(env.Searched, filepath.Dir(exe))
	st = Look(env)
	if !st.Ours || st.Command != filepath.Join(filepath.Dir(exe), "kando.exe") || st.MCPCommand() != st.Command {
		t.Fatalf("with the app's folder on PATH: %+v", st)
	}

	// A kando without an extension is not a command there, but another program's kando.cmd earlier on PATH, as npm makes, wins.
	cliToolFakeFile(t, filepath.Join(first, "kando"))
	if st := Look(env); !st.Ours {
		t.Fatalf("a kando without an extension hides this app's: %+v", st)
	}
	cliToolFakeFile(t, filepath.Join(first, "kando.cmd"))
	st = Look(env)
	if st.Ours || st.Command != filepath.Join(first, "kando.cmd") || st.MCPCommand() != exe {
		t.Fatalf("with another kando first on PATH: %+v", st)
	}
}
