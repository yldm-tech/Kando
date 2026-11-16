package desktop

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// xdgVideosDir is the videos folder a Linux desktop names for the user whose home is home: XDG_VIDEOS_DIR from the environment, else from user-dirs.dirs in the configuration folder (XDG_CONFIG_HOME, else ~/.config), which xdg-user-dirs writes in the user's language, as ~/Vidéos. A value starts with $HOME/ or is absolute; "" when neither says, or when it is home itself, which is how the file turns the folder off.
func xdgVideosDir(home string, getenv func(string) string) string {
	value := getenv("XDG_VIDEOS_DIR")
	if value == "" {
		config := getenv("XDG_CONFIG_HOME")
		if !filepath.IsAbs(config) {
			config = filepath.Join(home, ".config")
		}
		value = userDirsValue(filepath.Join(config, "user-dirs.dirs"), "XDG_VIDEOS_DIR")
	}
	switch {
	case value == "$HOME" || value == "$HOME/":
		return ""
	case strings.HasPrefix(value, "$HOME/"):
		return filepath.Join(home, filepath.FromSlash(strings.TrimPrefix(value, "$HOME/")))
	case filepath.IsAbs(value) && filepath.Clean(value) != filepath.Clean(home):
		return filepath.Clean(value)
	}
	return ""
}

// userDirsValue is what the user-dirs.dirs at path sets key to, its quotes taken off: a line of key="value", which the format allows only with $HOME and no other expansion, and comments after #.
func userDirsValue(path, key string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	lines := bufio.NewScanner(f)
	value := ""
	for lines.Scan() {
		line := strings.TrimSpace(lines.Text())
		if rest, ok := strings.CutPrefix(line, key+"="); ok {
			value = strings.Trim(strings.TrimSpace(rest), `"`)
		}
	}
	return value
}
