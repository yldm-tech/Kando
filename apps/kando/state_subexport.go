package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/locales"
	"kando/apps/kando/subexport"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
)

// Exporting the open film's subtitles (File › 导出字幕… ⌘E): the dialog in package subexport chooses how, and the export is the film's lines as the player shows them, the user's corrections laid over the machine's and the lines marked as no speech left out (core.Correct), written to the film's folder or where the user saves it.

// subexportUI is the state of the 导出字幕 dialog: whether it is open, release, which gives back the window's modal flag it holds (holdModal), and what the user chose, kept for the next export and in settings.json.
type subexportUI struct {
	open    bool
	release func()
	choice  subexport.Choice
}

func newSubexportUI() subexportUI { return subexportUI{choice: subexport.Default()} }

// savedSubexport is the export dialog's part of settings.json: what the user chose last. SubexportNoDelay keeps the delay out of the times, as the dialog writes it in unless told not to.
type savedSubexport struct {
	SubexportFormat  string `json:"subtitleExportFormat,omitempty"`
	SubexportContent string `json:"subtitleExportContent,omitempty"`
	SubexportOrder   string `json:"subtitleExportOrder,omitempty"`
	SubexportMissing string `json:"subtitleExportMissing,omitempty"`
	SubexportNoDelay bool   `json:"subtitleExportNoDelay,omitempty"`
}

func (s *savedSubexport) clean() {}

// subexportHooks: the choice is saved with the settings and taken back at startup, a value the dialog does not offer replaced by its default.
func (a *app) subexportHooks() areaHooks {
	return areaHooks{
		save: func(saved *savedSettings) {
			c := a.export.choice
			saved.savedSubexport = savedSubexport{SubexportFormat: c.Format, SubexportContent: c.Content, SubexportOrder: c.Order, SubexportMissing: c.Missing, SubexportNoDelay: !c.Delay}
		},
		restore: func(saved savedSettings) {
			s := saved.savedSubexport
			a.export.choice = subexport.Choice{Format: s.SubexportFormat, Content: s.SubexportContent, Order: s.SubexportOrder, Missing: s.SubexportMissing, Delay: !s.SubexportNoDelay}.Clean()
		},
	}
}

// subexportKeys is File › 导出字幕… ⌘E, while a film is open with its subtitles read.
func (a *app) subexportKeys(cx *ns.Scope) []shortcut {
	return []shortcut{{Group: groupFile, Label: locales.SubexportMenu(cx), Key: "mod+e", When: a.subexportCan, Run: a.OpenSubexport}}
}

// subexportOverlay is the export dialog over the window while it is open (overlays).
func (a *app) subexportOverlay(*ns.Scope) ns.Node { return subexport.Dialog{A: a} }

// subexportCan reports whether the open film's subtitles can be exported: they are read, with the user's corrections, and hold a line.
func (a *app) subexportCan() bool {
	return a.openKey != "" && a.correctLoaded() && !a.player.subs.editsErr && len(a.player.subs.machine) > 0
}

// subexportLines are the open film's lines with the user's corrections laid over them, those marked as no speech among them, in the order they start; none while they cannot be exported (subexportCan).
func (a *app) subexportLines() []core.Corrected {
	if !a.subexportCan() {
		return nil
	}
	subs := a.player.subs
	lines := make([]core.Line, len(subs.machine))
	for i, c := range subs.machine {
		translation, original := core.SplitCue(c.Text)
		lines[i] = core.Line{Start: c.Start, End: c.End, Original: original, Translation: translation}
	}
	return core.Correct(lines, subs.edits)
}

// subexportVideo is the video of the film open, "" while none is.
func (a *app) subexportVideo() string {
	item, ok := a.openFilm()
	if !ok {
		return ""
	}
	return item.VideoPath
}

// subexportAs is the export the user chose, of the open film with its subtitle delay, a placeholder in the interface's language standing in for a missing translation.
func (a *app) subexportAs() core.Export {
	return a.export.choice.Export(a.player.film.delay, a.messages().SubexportPlaceholder())
}

// OpenSubexport opens the export dialog (subexport.App).
func (a *app) OpenSubexport() {
	if a.export.open || !a.subexportCan() {
		return
	}
	a.export.open, a.export.release = true, a.holdModal()
}

func (a *app) SubexportOpen() bool { return a.export.open }

func (a *app) SubexportChoice() subexport.Choice { return a.export.choice }

// SetSubexportChoice keeps c as the choice and saves it.
func (a *app) SetSubexportChoice(c subexport.Choice) {
	a.export.choice = c.Clean()
	a.Save()
}

func (a *app) SubexportCount() core.ExportCount { return a.subexportAs().Count(a.subexportLines()) }

func (a *app) SubexportDelay() time.Duration {
	if a.openKey == "" {
		return 0
	}
	return a.player.film.delay
}

// SubexportPath is the file 导出到影片文件夹 writes, beside the film's video (subexport.Choice.FileName).
func (a *app) SubexportPath() string {
	video := a.subexportVideo()
	if video == "" || !a.subexportCan() {
		return ""
	}
	return filepath.Join(filepath.Dir(video), a.export.choice.FileName(video))
}

// CloseSubexport closes the dialog and gives back the window's modal flag.
func (a *app) CloseSubexport() {
	if a.export.release != nil {
		a.export.release()
	}
	a.export.open, a.export.release = false, nil
}

// SubexportToFolder writes the export beside the film's video, closes the dialog and says on the picture what it wrote, or why it could not.
func (a *app) SubexportToFolder() {
	path := a.SubexportPath()
	if path == "" {
		return
	}
	if err := subexport.Write(path, a.subexportAs().Write(a.subexportLines())); err != nil {
		a.flash(a.messages().SubexportFailed())
		return
	}
	a.CloseSubexport()
	a.flash(a.messages().SubexportDone(filepath.Base(path)))
}

// SubexportSaveAs asks where to save the export, under the name it takes in the film's folder, and closes the dialog once it is saved. The system's dialog writes the file itself, so a save over one of the subtitle files Kando keeps for the film, which would then lay the corrections over lines already holding them, is put back as it was.
func (a *app) SubexportSaveAs() {
	path := a.SubexportPath()
	if path == "" || a.cx == nil {
		return
	}
	data := a.subexportAs().Write(a.subexportLines())
	kept := subexportKandoFiles(a.subexportVideo(), a.player.subs.path)
	release := a.holdModal()
	// TODO(nsgo): give core.SaveOptions a folder to start in, so the panel opens in the film's folder rather than where the system chooses; and a way to refuse a path before the file is written, so a save over Kando's own subtitles need not be put back afterwards (subexportPutBack)
	a.cx.Run(func() {
		a.cx.SaveFile(ns.SaveOptions{Name: filepath.Base(path), Data: data}, func(saved bool, to string, err error) {
			release()
			defer a.Changed()
			switch {
			case err != nil:
				a.flash(a.messages().SubexportFailed())
			case !saved:
			case subexportPutBack(kept, to):
				a.flash(a.messages().SubexportKandoFile())
			default:
				a.CloseSubexport()
				a.flash(a.messages().SubexportDone(filepath.Base(to)))
			}
		})
	})
}

// subexportKandoFiles are the subtitle files Kando keeps for the film whose video is video and whose subtitles the player reads from subtitles, with what they hold now: its transcript, the one it keeps beside subtitles of the user's, and its bilingual subtitles (media.Companions), those that exist.
func subexportKandoFiles(video, subtitles string) map[string][]byte {
	paths := []string{subtitles}
	if video != "" {
		paths = append(paths, slices.DeleteFunc(media.Companions(video), func(p string) bool { return !strings.EqualFold(filepath.Ext(p), ".srt") })...)
	}
	kept := map[string][]byte{}
	for _, p := range paths {
		if p == "" {
			continue
		}
		if data, err := os.ReadFile(p); err == nil {
			kept[subexportKey(p)] = data
		}
	}
	return kept
}

// subexportPutBack puts back the file at path as kept holds it, when it is one of them, and reports whether it was.
func subexportPutBack(kept map[string][]byte, path string) bool {
	data, ok := kept[subexportKey(path)]
	if !ok {
		return false
	}
	subexport.Write(path, data)
	return true
}

// subexportKey is path with its folder's symbolic links resolved, so a file is found among subexportKandoFiles under the path the save dialog gives for it, as /private/var for /var on macOS.
func subexportKey(path string) string {
	path = filepath.Clean(path)
	if dir, err := filepath.EvalSymlinks(filepath.Dir(path)); err == nil {
		return filepath.Join(dir, filepath.Base(path))
	}
	return path
}
