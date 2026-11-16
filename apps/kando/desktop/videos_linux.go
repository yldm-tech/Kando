package desktop

import "os"

// VideosDir is the user's videos folder, which a new library goes in by default: on Linux the one the desktop names (xdgVideosDir), in the user's language; "" when it names none.
func VideosDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return xdgVideosDir(home, os.Getenv)
}
