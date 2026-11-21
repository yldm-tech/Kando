package mediasource

import (
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

// sourceNameMax is the most characters of a source's name its folder in the library takes.
const sourceNameMax = 80

// placeholderTries is how many numbered names Placeholder tries for a path that is taken.
const placeholderTries = 1000

// Placeholder is where an indexed film's video is said to be in the library, though no video is there: library/<source name>/<path on the source>, which its subtitles and the files kept beside a video (media.Companions) are named after. Each name is made one this system can hold as a file's (fileName), and the result stays in the library's folder (filepath.IsLocal). A path that taken reports in use, as another film's, gets a number before its extension, "Heat (2).mkv", and so on. p is a file's path on the source (CleanPath); the root and one that leaves it are ErrInvalidPath.
func Placeholder(library, sourceName, p string, taken func(string) bool) (string, error) {
	clean, err := CleanPath(p)
	if err != nil {
		return "", err
	}
	if clean == "" {
		return "", fmt.Errorf("the root: %w", ErrInvalidPath)
	}
	if utf8.RuneCountInString(sourceName) > sourceNameMax {
		sourceName = string([]rune(sourceName)[:sourceNameMax])
	}
	names := append([]string{fileName(sourceName)}, strings.Split(clean, "/")...)
	for i := 1; i < len(names); i++ {
		names[i] = fileName(names[i])
	}
	last := names[len(names)-1]
	ext := path.Ext(last)
	if ext == last {
		ext = ""
	}
	stem := strings.TrimSuffix(last, ext)
	for n := 1; n <= placeholderTries; n++ {
		if n > 1 {
			names[len(names)-1] = stem + " (" + strconv.Itoa(n) + ")" + ext
		}
		rel := filepath.Join(names...)
		if !filepath.IsLocal(rel) {
			return "", fmt.Errorf("%q: %w", p, ErrInvalidPath)
		}
		full := filepath.Join(library, rel)
		if taken == nil || !taken(full) {
			return full, nil
		}
	}
	return "", fmt.Errorf("mediasource: no free name for %q in %s", p, library)
}

// fileName is name made one a file can have on every system Kando runs on: the characters Windows refuses in a name (<>:"/\|?* and control characters) are _, the spaces and dots it drops from a name's end go, a name Windows keeps for a device (CON, NUL, COM1 and the like, with any extension) starts with _, and nothing left is _.
func fileName(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.TrimRight(name, " .")
	if name == "" {
		return "_"
	}
	base, _, _ := strings.Cut(name, ".")
	if reservedName(strings.TrimRight(base, " ")) {
		name = "_" + name
	}
	return name
}

// reservedName reports whether base is a name Windows keeps for a device, in any case.
func reservedName(base string) bool {
	switch strings.ToUpper(base) {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return true
	}
	upper := strings.ToUpper(base)
	if len(upper) == 4 && (strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) {
		c := upper[3]
		return c >= '0' && c <= '9'
	}
	// Windows also keeps COM and LPT with a superscript digit.
	for _, prefix := range []string{"COM", "LPT"} {
		if rest, ok := strings.CutPrefix(upper, prefix); ok && (rest == "¹" || rest == "²" || rest == "³") {
			return true
		}
	}
	return false
}
