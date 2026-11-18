// Package subexport is the 导出字幕 dialog: it exports the open film's subtitles, with the user's corrections and without the lines marked as no speech, as SubRip, WebVTT or ASS (core.Export), to the film's folder or where the user saves them.
package subexport

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"kando/apps/kando/live"
	"kando/packages/core"
)

// App is what the dialog reads and changes. The dialog reads it inside its bindings through live.Func and calls its actions through live.Do and live.On, so what they change shows at once (package live).
type App interface {
	live.State
	// SubexportOpen reports whether the dialog shows.
	SubexportOpen() bool
	// SubexportChoice is what the user chose to export; SetSubexportChoice changes it.
	SubexportChoice() Choice
	SetSubexportChoice(c Choice)
	// SubexportCount is what the export as chosen holds of the open film's lines.
	SubexportCount() core.ExportCount
	// SubexportDelay is the open film's subtitle delay, which the export moves its times by while the choice says so.
	SubexportDelay() time.Duration
	// SubexportPath is the file 导出到影片文件夹 writes, in the film's folder, "" while no film is open.
	SubexportPath() string
	// SubexportToFolder writes the export to SubexportPath; SubexportSaveAs asks where to save it; CloseSubexport closes the dialog.
	SubexportToFolder()
	SubexportSaveAs()
	CloseSubexport()
}

// Choice is how the user chose to export: Format, one of core's Format values; Content, which lines of each cue (core.SubOriginal, core.SubTranslation or core.SubBilingual), Order, which of a bilingual cue's lines goes on top (core.SubTranslationFirst or core.SubOriginalFirst), and Missing, what a line without a translation becomes (one of core's Missing values); Delay moves the times by the film's subtitle delay, so the file shows the lines when the player does.
type Choice struct {
	Format  string
	Content string
	Order   string
	Missing string
	Delay   bool
}

// Default is the dialog's choice before the user changed it: bilingual SubRip, the translation on top, a line without a translation left out, the delay written into the times.
func Default() Choice {
	return Choice{Format: core.FormatSRT, Content: core.SubBilingual, Order: core.SubTranslationFirst, Missing: core.MissingSkip, Delay: true}
}

// Clean is c with each value one the dialog offers, Default's in place of one it does not, as a choice read back from settings.json may hold.
func (c Choice) Clean() Choice {
	d := Default()
	pick := func(v, def string, allowed ...string) string {
		for _, a := range allowed {
			if v == a {
				return v
			}
		}
		return def
	}
	c.Format = pick(c.Format, d.Format, core.FormatSRT, core.FormatVTT, core.FormatASS)
	c.Content = pick(c.Content, d.Content, core.SubOriginal, core.SubTranslation, core.SubBilingual)
	c.Order = pick(c.Order, d.Order, core.SubTranslationFirst, core.SubOriginalFirst)
	c.Missing = pick(c.Missing, d.Missing, core.MissingSkip, core.MissingPlaceholder, core.MissingOriginal)
	return c
}

// Export is how c exports a film whose subtitle delay is delay, placeholder standing in for a missing translation where c says so.
func (c Choice) Export(delay time.Duration, placeholder string) core.Export {
	e := core.Export{Format: c.Format, Content: c.Content, Order: c.Order, Missing: c.Missing, Placeholder: placeholder}
	if c.Delay {
		e.Delay = delay
	}
	return e
}

// FileName is the name of the file c exports of the video named video: the video's name without its extension, what the file holds (original, translation, or dual for both) and the format's extension, as "video.dual.srt". The names never take one of those Kando keeps a film's subtitles under (<video>.srt, <video>.kando.srt, <video>.bilingual.srt), which an export must not overwrite: the corrections would then be laid over lines that already hold them.
func (c Choice) FileName(video string) string {
	base := strings.TrimSuffix(filepath.Base(video), filepath.Ext(video))
	tag := "dual"
	switch c.Content {
	case core.SubOriginal:
		tag = "original"
	case core.SubTranslation:
		tag = "translation"
	}
	return base + "." + tag + core.Export{Format: c.Format}.Ext()
}

// Write writes data to path through a hidden file beside it, renamed over it once written, so path holds either what it held before or all of data.
func Write(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Chmod(0o644)
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}

// Display is path with the home folder written ~, as the library shows paths.
func Display(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || path == "" {
		return path
	}
	if prefix := home + string(filepath.Separator); strings.HasPrefix(path, prefix) {
		return "~" + string(filepath.Separator) + path[len(prefix):]
	}
	return path
}
