//go:build darwin

package brand

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"kando/packages/media"
)

const libraryIconAttr = "com.lingocut.libraryIcon"
const libraryIconVersion = "2"

// Ensure gives a .lingocut bundle a Finder icon: the system folder with the app mark on its face. A bundle that already has this version is left untouched.
func Ensure(dir string, png []byte) {
	if filepath.Ext(dir) != media.LibraryExt {
		return
	}
	if libraryIconCurrent(dir) && hasCustomIcon(dir) {
		return
	}
	file, err := os.CreateTemp("", "lingocut-icon-*.png")
	if err != nil {
		return
	}
	path := file.Name()
	if _, err := file.Write(png); err != nil {
		file.Close()
		os.Remove(path)
		return
	}
	file.Close()
	defer os.Remove(path)
	// The mark sits where a folder badge does: 346px on a 1024 canvas, centered, bottom edge 250px up.
	script := `use framework "AppKit"
on run argv
  set badge to current application's NSImage's alloc()'s initWithContentsOfFile:(item 1 of argv)
  if badge is missing value then error "badge"
  set ws to current application's NSWorkspace's sharedWorkspace()
  set folder to ws's iconForFileType:"public.folder"
  set canvas to current application's NSImage's alloc()'s initWithSize:{1024, 1024}
  canvas's lockFocus()
  set whole to {{0, 0}, {1024, 1024}}
  folder's drawInRect:whole fromRect:{{0, 0}, {0, 0}} operation:(current application's NSCompositingOperationSourceOver) fraction:1
  set mark to {{339, 250}, {346, 346}}
  badge's drawInRect:mark fromRect:{{0, 0}, {0, 0}} operation:(current application's NSCompositingOperationSourceOver) fraction:1
  canvas's unlockFocus()
  set ok to ws's setIcon:canvas forFile:(item 2 of argv) options:0
  if ok is false then error "icon"
end run`
	if err := exec.Command("osascript", "-e", script, path, dir).Run(); err != nil {
		return
	}
	_ = exec.Command("xattr", "-w", libraryIconAttr, libraryIconVersion, dir).Run()
}

func libraryIconCurrent(dir string) bool {
	out, err := exec.Command("xattr", "-p", libraryIconAttr, dir).Output()
	return err == nil && strings.TrimSpace(string(out)) == libraryIconVersion
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
