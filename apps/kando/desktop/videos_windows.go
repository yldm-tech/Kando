package desktop

import "golang.org/x/sys/windows"

// VideosDir is the user's videos folder, which a new library goes in by default: on Windows the Videos known folder, wherever the user or OneDrive moved it; "" when Windows does not say.
func VideosDir() string {
	dir, err := windows.KnownFolderPath(windows.FOLDERID_Videos, 0)
	if err != nil {
		return ""
	}
	return dir
}
