// Package sourcebrowser is the 媒体来源 page (docs/media-sources.md 4.1): the media sources the user added, from this page or in Settings › 媒体来源, on its left, and the folder of the one chosen on its right, its videos and folders, where videos are chosen and imported into the library through the dialog it opens over the window (ImportDialog). It draws and hands back presses; the listing, the choosing and the import are the app's (App).
package sourcebrowser

import (
	"time"

	"kando/apps/kando/live"
)

// App is what the page and its dialog read and do. They read the getters in their bindings and call the actions from their handlers, which tell the revision (package live), so none of the actions calls Changed itself. What they say is in the interface's language.
type App interface {
	live.State

	// MediaBrowseSources are the sources added, in the order they were; MediaBrowseSource is the one shown, "" while there is none, and ChooseMediaBrowseSource shows another at its root.
	MediaBrowseSources() []Source
	MediaBrowseSource() string
	ChooseMediaBrowseSource(id string)
	// MediaBrowseCrumbs are the folder shown, the source's name first and then each folder down to it; OpenMediaBrowseCrumb goes up to the one at index.
	MediaBrowseCrumbs() []string
	OpenMediaBrowseCrumb(index int)
	// MediaBrowseEntries are the folders and videos of the folder shown, the folders first, each in the order of their names; MediaBrowseLoading is whether it is being listed, and MediaBrowseErr why it could not be, "" when it was. RetryMediaBrowse lists it again, and EditMediaBrowseSource opens the source's settings.
	MediaBrowseEntries() []Entry
	MediaBrowseLoading() bool
	MediaBrowseErr() string
	RetryMediaBrowse()
	EditMediaBrowseSource()
	// OpenMediaBrowseEntry opens the folder at path, or the film imported from the video at path; ToggleMediaBrowseEntry chooses the video at path to import, or no more.
	OpenMediaBrowseEntry(path string)
	ToggleMediaBrowseEntry(path string)
	// MediaBrowseChosen is how many videos are chosen; ImportMediaBrowseChosen opens the dialog that imports them (MediaBrowseImport). MediaBrowseNote is what the page says about the last import, "" for nothing.
	MediaBrowseChosen() int
	ImportMediaBrowseChosen()
	MediaBrowseNote() string
	// AddMediaBrowseSource opens Settings › 媒体来源 with the dialog that adds a source; once the source is saved, the settings close and this page shows it.
	AddMediaBrowseSource()

	// MediaBrowseImport is the dialog importing the videos chosen, Open false while it is closed; ChooseMediaBrowseImportLink picks how they are imported, indexed and played from the source (true) or copied into the library, and RememberMediaBrowseImport whether that way is kept for the next import; StartMediaBrowseImport imports them and closes it, CancelMediaBrowseImport closes it.
	MediaBrowseImport() Import
	ChooseMediaBrowseImportLink(link bool)
	RememberMediaBrowseImport(on bool)
	StartMediaBrowseImport()
	CancelMediaBrowseImport()
}

// Source is a source as the page lists it: its name and its kind and where it is (Summary).
type Source struct {
	ID, Name, Summary string
}

// Entry is a folder or a video of the folder shown, as its row shows it. Path is where it is on the source, which the actions take. Share is a share of a server, listed at the root of a source without one. Size and Modified are a video's, which the row writes; a folder's are not shown. Subtitles is a subtitle file beside the video, which is copied with it; InLibrary a video already imported, which is opened rather than chosen; Chosen a video chosen to import.
type Entry struct {
	Path, Name string
	Dir, Share bool
	Size       int64
	Modified   time.Time
	Subtitles  bool
	InLibrary  bool
	Chosen     bool
}

// Import is the dialog importing the videos chosen: how many, from which source, how many bytes their copies take, whether they are only indexed and played from the source (Link) rather than copied, and whether that choice is remembered for the next import.
type Import struct {
	Open     bool
	Count    int
	Source   string
	Size     int64
	Link     bool
	Remember bool
}
