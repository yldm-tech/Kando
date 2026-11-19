package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/locales"
	"kando/apps/kando/plugins"
	"kando/apps/kando/subexport"
	"kando/packages/core"
	"kando/packages/media"
	"kando/packages/track"

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

// subexportCan reports whether the open film's subtitles can be exported: they are read, with the user's corrections, and hold a line. A read of them again does not stop it, as the lines read before show until it lands.
func (a *app) subexportCan() bool {
	return a.openKey != "" && a.correctShown() && !a.player.subs.editsErr && len(a.player.subs.machine) > 0
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

// SubexportChoice is what the user chose, with SubRip in place of a plugin's format that is not running now.
func (a *app) SubexportChoice() subexport.Choice {
	c := a.export.choice
	if _, ok := a.pluginsSubtitleFormat(c.Format); strings.HasPrefix(c.Format, subexport.PluginPrefix) && !ok {
		c.Format = core.FormatSRT
	}
	return c
}

// SubexportFormats are the subtitle formats of the script plugins running, each labelled with its plugin's label in the interface's language (subexport.App).
func (a *app) SubexportFormats() []subexport.Format {
	m := a.messages()
	var out []subexport.Format
	for _, f := range a.pluginsSubtitleFormats() {
		out = append(out, subexport.Format{Value: pluginsContribID(f.id, f.of.ID), Label: f.of.Label.In(m.Language())})
	}
	return out
}

// subexportData makes the export the user chose and has done hear it on the engine's goroutine: Kando's own formats at once (core.Export.Write), a plugin's from its exportSubtitles(formatID, cues), given the cues Kando's own would write (plugins.CueValues); err when the plugin failed, is not running, or returned something that is not a file's text.
func (a *app) subexportData(done func(data []byte, err error)) {
	e, lines := a.subexportAs(), a.subexportLines()
	f, ok := a.pluginsSubtitleFormat(a.SubexportChoice().Format)
	if !ok {
		done(e.Write(lines), nil)
		return
	}
	if a.plugins.script.contrib.formatting {
		return
	}
	a.plugins.script.contrib.formatting = true
	a.pluginsContribCall(f.id, f.of.Export, []any{f.of.ID, plugins.CueValues(e.Cues(lines))}, func(v any, err error) {
		a.plugins.script.contrib.formatting = false
		defer a.Changed()
		text, ok := plugins.SubtitlesText(v)
		if err == nil && !ok {
			err = errPluginsBadSubtitles
		}
		if err != nil {
			done(nil, err)
			return
		}
		done([]byte(text), nil)
	})
}

// errPluginsBadSubtitles is a subtitle format of a plugin that returned something other than a file's text.
var errPluginsBadSubtitles = errors.New("the plugin's subtitle format returned no text")

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
	c := a.SubexportChoice()
	if f, ok := a.pluginsSubtitleFormat(c.Format); ok {
		return filepath.Join(filepath.Dir(video), c.FileNameExt(video, "."+f.of.Extension))
	}
	return filepath.Join(filepath.Dir(video), c.FileName(video))
}

// CloseSubexport closes the dialog and gives back the window's modal flag.
func (a *app) CloseSubexport() {
	if a.export.release != nil {
		a.export.release()
	}
	a.export.open, a.export.release = false, nil
}

// SubexportToFolder writes the export beside the film's video, closes the dialog and says on the picture what it wrote, or why it could not; a plugin's format waits for the plugin first (subexportData).
func (a *app) SubexportToFolder() {
	path := a.SubexportPath()
	if path == "" {
		return
	}
	a.subexportData(func(data []byte, err error) {
		if err == nil {
			err = subexport.Write(path, data)
		}
		if err != nil {
			a.flash(a.messages().SubexportFailed())
			return
		}
		a.CloseSubexport()
		a.flash(a.messages().SubexportDone(filepath.Base(path)))
	})
}

// SubexportSaveAs asks where to save the export, under the name it takes in the film's folder, and closes the dialog once it is saved. The system's dialog writes the file itself, so a save under one of the names Kando keeps the film's subtitles under, which would then lay the corrections over lines already holding them, is undone (subexportUndo).
func (a *app) SubexportSaveAs() {
	path := a.SubexportPath()
	if path == "" || a.cx == nil {
		return
	}
	a.subexportData(func(data []byte, err error) {
		if err != nil {
			a.flash(a.messages().SubexportFailed())
			return
		}
		a.subexportSave(path, data)
	})
}

// subexportSave asks where to save data, the export, under the name of path, as SubexportSaveAs does.
func (a *app) subexportSave(path string, data []byte) {
	video := a.subexportVideo()
	kept := subexportKeep(video, a.player.subs.path)
	busy := a.subexportJobRunning()
	release := a.holdModal()
	// TODO(nsgo#682): give core.SaveOptions a folder to start in, so the panel opens in the film's folder rather than where the system chooses; and a way to refuse a path before the file is written, so a save over Kando's own subtitles need not be undone afterwards (subexportUndo) https://github.com/nsgo-dev/nsgo/issues/682
	a.cx.Run(func() {
		a.cx.SaveFile(ns.SaveOptions{Name: filepath.Base(path), Data: data}, func(saved bool, to string, err error) {
			release()
			defer a.Changed()
			switch {
			case err != nil:
				a.flash(a.messages().SubexportFailed())
			case !saved:
			default:
				ours, changed, err := subexportUndo(kept, video, to, busy || a.subexportJobRunning())
				switch {
				case !ours:
					a.CloseSubexport()
					a.flash(a.messages().SubexportDone(filepath.Base(to)))
				case err != nil:
					a.flash(a.messages().SubexportKandoFileStuck())
				case changed:
					a.flash(a.messages().SubexportKandoFileChanged())
				default:
					a.flash(a.messages().SubexportKandoFile())
				}
			}
		})
	})
}

// subexportJobRunning reports whether the open film's job is running now, writing its subtitles as it goes.
func (a *app) subexportJobRunning() bool {
	item, ok := a.openFilm()
	return ok && item.JobID != "" && item.JobID == a.queue.active
}

// subexportKept is what the subtitle files of a film held as the save panel opened, by subexportKey: names, the subtitle files Kando keeps for the film and the one the player reads, each with what it held, nil for one that did not exist; companions, every file of the film's that existed then (media.Companions), with what it held, which tell whether Kando wrote any of them while the panel was open.
type subexportKept struct {
	names      map[string][]byte
	companions map[string][]byte
}

// subexportKeep is what the subtitle files of the film whose video is video and whose subtitles the player reads from subtitles hold now: its transcript, the one it keeps beside subtitles of the user's and its bilingual subtitles (<video>.srt, <video>.kando.srt, <video>.bilingual.srt), whether they exist or not, as a save under a name that is not there yet would make them the film's, and subtitles.
func subexportKeep(video, subtitles string) subexportKept {
	kept := subexportKept{names: map[string][]byte{}, companions: map[string][]byte{}}
	var paths []string
	if subtitles != "" {
		paths = append(paths, subtitles)
	}
	if video != "" {
		base := strings.TrimSuffix(video, filepath.Ext(video))
		paths = append(paths, base+".srt", base+".kando.srt", base+".bilingual.srt")
		for _, p := range subexportCompanions(video) {
			if data, err := os.ReadFile(p); err == nil {
				kept.companions[subexportKey(p)] = data
			}
		}
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			data = nil
		} else if data == nil {
			data = []byte{}
		}
		kept.names[subexportKey(p)] = data
	}
	return kept
}

// subexportUndo undoes a save of the export at path when path is one of kept's names, and reports whether it was. It puts the file back as kept holds it, or removes it when it did not exist then; but when Kando may have written the film's subtitles while the panel was open (busy, the film's job running as it opened or now, or another of its files changed since), what kept holds may be older than what the save wrote over, so it removes the file instead, reporting changed, rather than bring back subtitles that no longer match the rest. err is why the file could not be put back or removed; it then still holds the export.
func subexportUndo(kept subexportKept, video, path string, busy bool) (ours, changed bool, err error) {
	key := subexportKey(path)
	data, ours := kept.names[key]
	if !ours {
		return false, false, nil
	}
	changed = busy || subexportCompanionsChanged(kept, video, key)
	if data == nil || changed {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return true, changed, err
		}
		return true, changed, nil
	}
	return true, false, subexport.Write(path, data)
}

// subexportCompanionsChanged reports whether a file of the film whose video is video, other than the one at skip, is not now as kept holds it: one written, added or removed since.
func subexportCompanionsChanged(kept subexportKept, video, skip string) bool {
	if video == "" {
		return false
	}
	now := map[string]bool{}
	for _, p := range subexportCompanions(video) {
		key := subexportKey(p)
		if key == skip {
			continue
		}
		now[key] = true
		data, err := os.ReadFile(p)
		was, ok := kept.companions[key]
		if err != nil || !ok || !bytes.Equal(data, was) {
			return true
		}
	}
	for key := range kept.companions {
		if key != skip && !now[key] {
			return true
		}
	}
	return false
}

// subexportCompanions are the files of the film whose video is video (media.Companions) that tell whether Kando wrote its subtitles while the save panel was open: all but the logs of its subtitle document (track.Logs), which grow with records of no change to the lines, as the one taking in the very file the save wrote, and whose changes to the lines are written out to the subtitle files, which are compared.
func subexportCompanions(video string) []string {
	logs := map[string]bool{}
	for _, p := range track.Logs(video) {
		logs[filepath.Clean(p)] = true
	}
	return slices.DeleteFunc(media.Companions(video), func(p string) bool { return logs[filepath.Clean(p)] })
}

// subexportKey is path with its folder's symbolic links resolved, so a file is found among subexportKandoFiles under the path the save dialog gives for it, as /private/var for /var on macOS.
func subexportKey(path string) string {
	path = filepath.Clean(path)
	if dir, err := filepath.EvalSymlinks(filepath.Dir(path)); err == nil {
		return filepath.Join(dir, filepath.Base(path))
	}
	return path
}
