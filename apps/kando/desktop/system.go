package desktop

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
)

// Reveal and Trash run the system's own tools where NSGo has no service for them yet (nsgo#117): each platform's revealItem and trashItem, in system_<os>.go, build the command lines here and run them through runCommand. A path reaches a tool as an argument of its own, never through a shell, and absolute, so it never reads as one of the tool's options.

// runCommand runs cmd and waits for it, failing with what it wrote to its standard error when it exits with an error. Tests replace it to see the command lines without starting anything.
var runCommand = func(cmd *exec.Cmd) error {
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("%s: %w: %s", filepath.Base(cmd.Path), err, msg)
		}
		return fmt.Errorf("%s: %w", filepath.Base(cmd.Path), err)
	}
	return nil
}

// lookPath finds a tool as exec.LookPath does. Tests replace it to say which tools the system has.
var lookPath = exec.LookPath

// errNoTool is the error of a reveal or a trash that needs a tool the system lacks.
var errNoTool = errors.New("the system has no tool for it")

// openRevealCommand is macOS's open -R, which has Finder show path selected.
func openRevealCommand(path string) *exec.Cmd {
	return exec.Command("/usr/bin/open", "-R", path)
}

// macTrash is the trash command macOS has had since macOS 14, which moves its files to the Trash as Finder does and fails for a file on a volume without one.
const macTrash = "/usr/bin/trash"

// macTrashCommand moves path to the Trash with macTrash.
func macTrashCommand(path string) *exec.Cmd {
	return exec.Command(macTrash, path)
}

// finderTrashCommand has Finder move path to the Trash, for a macOS older than macTrash. The path is the script's argument, not part of its text, so nothing in it needs escaping; a path that is not there fails as it is made an alias.
func finderTrashCommand(path string) *exec.Cmd {
	return exec.Command("/usr/bin/osascript",
		"-e", "on run argv",
		"-e", "set target to POSIX file (item 1 of argv) as alias",
		"-e", `tell application "Finder" to delete target`,
		"-e", "end run",
		path)
}

// showItemsCommand has the file manager that answers org.freedesktop.FileManager1 show path selected, through gdbus, which the gio tools of GLib bring along. Its arguments are GVariant text: the array of the item's file: URI, and an empty startup ID.
func showItemsCommand(gdbus, path string) *exec.Cmd {
	return exec.Command(gdbus, "call", "--session",
		"--dest", "org.freedesktop.FileManager1",
		"--object-path", "/org/freedesktop/FileManager1",
		"--method", "org.freedesktop.FileManager1.ShowItems",
		"["+gvariantString(fileURI(path))+"]", gvariantString(""))
}

// gioTrashCommand moves path to the Trash with gio trash, which fails for a file on a volume without one rather than deleting it.
func gioTrashCommand(gio, path string) *exec.Cmd {
	return exec.Command(gio, "trash", path)
}

// explorerSelectLine is the command line that has File Explorer show path selected: explorer.exe takes /select, and the path in quotes as one argument, which Go's quoting of arguments would split, so the line is written whole. A Windows path holds no quotes.
func explorerSelectLine(path string) string {
	return `explorer.exe /select,"` + path + `"`
}

// fileURI is path as a file: URI, its special characters escaped.
func fileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

// gvariantString is s as a GVariant text string: in single quotes, with its backslashes and single quotes escaped.
func gvariantString(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
}
