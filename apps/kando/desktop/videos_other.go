//go:build !linux && !windows

package desktop

// VideosDir is the user's videos folder, which a new library goes in by default; "" on macOS, where it is ~/Movies whatever the language (media.MoviesDir).
func VideosDir() string { return "" }
