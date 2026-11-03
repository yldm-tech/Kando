//go:build darwin

package main

import (
	_ "embed"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

//go:embed resources/icon.png
var appIconPNG []byte

// ensureLibraryLogo gives a .vimpo bundle a Finder icon. A bundle that already has one, such as an existing 影视库.vimpo, is left untouched.
func ensureLibraryLogo(dir string) {
	if filepath.Ext(dir) != ".vimpo" || hasCustomIcon(dir) {
		return
	}
	file, err := os.CreateTemp("", "lingocut-icon-*.png")
	if err != nil {
		return
	}
	path := file.Name()
	if _, err := file.Write(appIconPNG); err != nil {
		file.Close()
		os.Remove(path)
		return
	}
	file.Close()
	defer os.Remove(path)
	script := `use framework "AppKit"
on run argv
  set image to current application's NSImage's alloc()'s initWithContentsOfFile:(item 1 of argv)
  if image is missing value then error "icon"
  current application's NSWorkspace's sharedWorkspace()'s setIcon:image forFile:(item 2 of argv) options:0
end run`
	_ = exec.Command("osascript", "-e", script, path, dir).Run()
}

func hasCustomIcon(dir string) bool {
	out, err := exec.Command("xattr", "-px", "com.apple.FinderInfo", dir).Output()
	if err != nil {
		return false
	}
	fields := strings.Fields(string(out))
	if len(fields) < 9 {
		return false
	}
	flags, err := strconv.ParseUint(fields[8], 16, 8)
	if err != nil {
		return false
	}
	return flags&0x04 != 0
}
