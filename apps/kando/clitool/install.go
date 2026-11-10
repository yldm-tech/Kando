// Package clitool puts the kando command where a terminal finds it, finds where it stands and gives MCP clients what to run, for Settings › 通用 › 命令行与 MCP.
package clitool

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

// cliToolName is the command a terminal runs Kando's command line by: the app's own executable under another name, which cli.Main recognizes by its arguments, not by the name.
const cliToolName = "kando"

// Env is what Look and Install see of the computer. EnvHere reads this one; tests give folders of their own.
type Env struct {
	// Exe is this app's executable, as os.Executable names it.
	Exe string
	// Searched are the folders a terminal looks for commands in, in its order.
	Searched []string
	// InstallTo are the folders Install may put the command in, in order of preference; Own is the one of them that belongs to the user, which it makes when it is missing.
	InstallTo []string
	Own       string
}

// Status is where the kando command stands, as Settings › 通用 › 命令行与 MCP shows it.
type Status struct {
	// Exe is this app's executable with its links resolved, which the command should run.
	Exe string
	// Command is the kando a terminal finds first, "" for none; Ours reports whether it runs Exe.
	Command string
	Ours    bool
	// Link is a kando in one of the install folders that runs Exe, "" for none. With Ours false it is a command a terminal does not find, as one installed in a ~/.local/bin that is not on PATH.
	Link string
	// Installable reports whether this system has folders to install the command in; Windows has none, as making a link there needs administrator rights or developer mode.
	Installable bool
	// Temporary reports whether Exe is in a place that goes away (cliToolTemporary), where no command or MCP configuration should point.
	Temporary bool
}

// MCPCommand is what an MCP client should run: the kando a terminal finds when it is this app, else the link the card installed, else the executable itself, each a full path, as MCP clients start their servers without a shell's PATH. It is "" while Kando runs from a place that goes away (cliToolTemporary), where every one of them leads to a path that soon stops working, so the card asks to move Kando first instead of giving out a configuration that breaks.
func (s Status) MCPCommand() string {
	switch {
	case s.Temporary:
		return ""
	case s.Ours:
		return s.Command
	case s.Link != "":
		return s.Link
	}
	return s.Exe
}

// EnvHere is this computer: the running executable, a terminal's PATH as far as the app can know it, and /usr/local/bin, /opt/homebrew/bin on macOS and ~/.local/bin to install in. It reads /etc/paths, so it runs off the UI thread.
func EnvHere() Env {
	exe, _ := os.Executable()
	env := Env{Exe: exe, Searched: cliToolTerminalPath()}
	if runtime.GOOS == "windows" {
		return env
	}
	env.InstallTo = []string{"/usr/local/bin"}
	if runtime.GOOS == "darwin" {
		env.InstallTo = append(env.InstallTo, "/opt/homebrew/bin")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		env.Own = filepath.Join(home, ".local", "bin")
		env.InstallTo = append(env.InstallTo, env.Own)
	}
	return env
}

// cliToolTerminalPath is where a terminal looks for commands. An app opened from Finder or a desktop launcher has a shorter PATH than a terminal, so on macOS it adds what path_helper gives every login shell, /etc/paths and the files of /etc/paths.d, and Homebrew's /opt/homebrew/bin, which its installer puts on the shell's PATH. Folders a user adds in a shell's startup file are known only when the app was started from that shell.
func cliToolTerminalPath() []string {
	var dirs []string
	seen := map[string]bool{}
	add := func(dir string) {
		dir = strings.TrimSpace(dir)
		if dir == "" || !filepath.IsAbs(dir) || seen[filepath.Clean(dir)] {
			return
		}
		seen[filepath.Clean(dir)] = true
		dirs = append(dirs, dir)
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		add(dir)
	}
	if runtime.GOOS != "darwin" {
		return dirs
	}
	files := []string{"/etc/paths"}
	if more, err := filepath.Glob("/etc/paths.d/*"); err == nil {
		files = append(files, more...)
	}
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			continue
		}
		lines := bufio.NewScanner(f)
		for lines.Scan() {
			add(lines.Text())
		}
		f.Close()
	}
	if info, err := os.Stat("/opt/homebrew/bin"); err == nil && info.IsDir() {
		add("/opt/homebrew/bin")
	}
	return dirs
}

// Look finds where the kando command stands in env. It reads the disk, so it runs off the UI thread.
func Look(env Env) Status {
	st := Status{Exe: env.Exe, Installable: len(env.InstallTo) > 0}
	if resolved, err := filepath.EvalSymlinks(env.Exe); err == nil {
		st.Exe = resolved
	}
	st.Temporary = cliToolTemporary(st.Exe)
	for _, dir := range env.Searched {
		path := filepath.Join(dir, cliToolFile())
		if info, err := os.Stat(path); err == nil && !info.IsDir() && (runtime.GOOS == "windows" || info.Mode()&0o111 != 0) {
			st.Command = path
			st.Ours = cliToolRuns(path, st.Exe)
			break
		}
	}
	for _, dir := range env.InstallTo {
		if path := filepath.Join(dir, cliToolName); cliToolRuns(path, st.Exe) {
			st.Link = path
			break
		}
	}
	return st
}

// cliToolFile is the command's file name on this system: kando, or kando.exe on Windows, where it is Kando.exe itself in a folder on PATH, as the file system ignores case.
func cliToolFile() string {
	if runtime.GOOS == "windows" {
		return cliToolName + ".exe"
	}
	return cliToolName
}

// cliToolRuns reports whether path, following its links, is the executable exe. A Linux package that installs its own kando, a link or a copy, counts as the same file.
func cliToolRuns(path, exe string) bool {
	a, err := os.Stat(path)
	if err != nil {
		return false
	}
	b, err := os.Stat(exe)
	return err == nil && os.SameFile(a, b)
}

// cliToolTemporary reports whether exe runs from a place that goes away, where a link to it soon leads nowhere and an MCP client could not start it: the read-only copy macOS runs an app from when it was opened where it was downloaded (App Translocation), whose path changes each time the app opens, and a read-only volume under /Volumes, as the disk image Kando comes in, which is ejected. An app kept on an external disk, which is writable, is where its owner put it and counts as installed. The path is the one Look resolved, so /Volumes/Macintosh HD, a link to the startup disk, is not taken for one.
func cliToolTemporary(exe string) bool {
	return strings.Contains(exe, "/AppTranslocation/") || strings.HasPrefix(exe, "/Volumes/") && ReadOnly(exe)
}

// ReadOnly reports whether the volume holding path cannot be written; a test replaces it.
var ReadOnly = readOnlyVolume

// ErrTemporary is why Install refuses to link to an executable in a place that goes away (cliToolTemporary).
var ErrTemporary = errors.New("Kando runs from a disk image or a temporary copy; move it to the Applications folder and open it from there first")

// BlockedError is why Install did not install the command: every folder it may use has a kando that is not Kando's, which it never replaces. Paths are those commands.
type BlockedError struct{ Paths []string }

func (e *BlockedError) Error() string {
	return strings.Join(e.Paths, ", ") + " belongs to another program"
}

// cliToolIsKando reports whether link, a symbolic link to target as os.Readlink gives it, runs a Kando that Install may replace with a link to exe: this app itself, a copy inside a Kando.app bundle (Contents/MacOS), an app MyGo's Linux packages install (/opt/kando/kando from the .deb and the PKGBUILD, ~/.local/kando.app/kando from the install script, where local is the folder above own, ~/.local/bin), or a link to nothing that named a Kando.app, as one to an app since deleted from the Trash. A kando of another program, even a link to a file named kando, as a package manager's (npm's ../lib/node_modules/x/bin/kando, Homebrew's ../Cellar/kando/1.0/bin/kando), is not, so it stays.
func cliToolIsKando(link, target, exe, own string) bool {
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(link), target)
	}
	target = filepath.Clean(target)
	known := []string{"/opt/kando/kando"}
	if own != "" {
		known = append(known, filepath.Join(filepath.Dir(own), "kando.app", "kando"))
	}
	resolved, err := filepath.EvalSymlinks(link)
	if err != nil {
		// A dangling link: what it named is gone, so only its path says whose it was.
		return errors.Is(err, fs.ErrNotExist) && (strings.Contains(target, "Kando.app") || slices.Contains(known, target))
	}
	if cliToolRuns(resolved, exe) {
		return true
	}
	for _, path := range []string{target, resolved} {
		if strings.Contains(path, "/Kando.app/Contents/MacOS/") || slices.Contains(known, path) {
			return true
		}
	}
	return false
}

// Install links kando to exe in the first folder of env.InstallTo that takes it, and returns the link. A folder takes it when it is writable and has no kando, or a kando that is a link to a Kando executable (cliToolIsKando), which it replaces; it never overwrites a file or a link to another program (BlockedError), and never asks for administrator rights. It makes env.Own when it is missing. It refuses an exe in a place that goes away (ErrTemporary), as the link would soon lead nowhere.
func Install(env Env, exe string) (string, error) {
	if len(env.InstallTo) == 0 {
		return "", errors.New("no folder to install the command in on this system")
	}
	if cliToolTemporary(exe) {
		return "", ErrTemporary
	}
	var blocked, failed []string
	for _, dir := range env.InstallTo {
		link := filepath.Join(dir, cliToolName)
		info, err := os.Lstat(link)
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			failed = append(failed, err.Error())
			continue
		case info.Mode()&fs.ModeSymlink == 0:
			blocked = append(blocked, link)
			continue
		default:
			if target, err := os.Readlink(link); err != nil || !cliToolIsKando(link, target, exe, env.Own) {
				blocked = append(blocked, link)
				continue
			}
		}
		if dir == env.Own {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				failed = append(failed, err.Error())
				continue
			}
		}
		// The link is made beside its place and renamed over it, so a kando that was there runs the old app or the new one, never nothing.
		tmp := filepath.Join(dir, "."+cliToolName+"-"+strconv.FormatInt(time.Now().UnixNano(), 36))
		if err := os.Symlink(exe, tmp); err != nil {
			failed = append(failed, err.Error())
			continue
		}
		if err := os.Rename(tmp, link); err != nil {
			os.Remove(tmp)
			failed = append(failed, err.Error())
			continue
		}
		return link, nil
	}
	if len(blocked) > 0 {
		return "", &BlockedError{Paths: blocked}
	}
	return "", errors.New(strings.Join(failed, "; "))
}

// MCPConfig is the mcpServers entry that runs command as Kando's MCP server, as Claude Desktop, Cursor and other MCP clients read it from their configuration file; "" when there is no command, as when Kando runs from a place that goes away (Status.MCPCommand).
func MCPConfig(command string) string {
	if command == "" {
		return ""
	}
	type server struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	config := map[string]map[string]server{"mcpServers": {cliToolName: {Command: command, Args: []string{"mcp"}}}}
	out, _ := json.MarshalIndent(config, "", "  ")
	return string(out)
}

// ClaudeCommand is the command that adds Kando's MCP server to Claude Code; "" when there is no command, as MCPConfig.
func ClaudeCommand(command string) string {
	if command == "" {
		return ""
	}
	return "claude mcp add " + cliToolName + " -- " + cliToolShellQuote(command) + " mcp"
}

// cliToolShellQuote quotes s for a POSIX shell when it has characters the shell would split or expand.
func cliToolShellQuote(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("/._-+:@%,=", r))
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
