package main

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/settings"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
)

// Settings › 语音识别 and 通用: the whisper model, its download, where it downloads from and its file, the language spoken in the videos, and the tools' versions.

// savedSpeech is the speech model's part of settings.json: the model's file, the language spoken in the videos as whisper's code, empty for whisper to detect it, and where the models download from, empty for Hugging Face (media.SpeechSources).
type savedSpeech struct {
	WhisperModel   string `json:"whisperModel,omitempty"`
	SpeechLanguage string `json:"speechLanguage,omitempty"`
	SpeechSource   string `json:"speechSource,omitempty"`
}

// clean trims the model's path, the language and the download source, drops a language whisper is not offered for (media.SpeechLanguages), which leaves whisper to detect it, and a source Kando does not know, which leaves the models to download from Hugging Face.
func (s *savedSpeech) clean() {
	s.WhisperModel = strings.TrimSpace(s.WhisperModel)
	s.SpeechLanguage = strings.TrimSpace(s.SpeechLanguage)
	if !media.KnownSpeechLanguage(s.SpeechLanguage) {
		s.SpeechLanguage = ""
	}
	if s.SpeechSource = strings.TrimSpace(s.SpeechSource); !slices.Contains(media.SpeechSources(), s.SpeechSource) {
		s.SpeechSource = ""
	}
}

func (a *app) saveSpeech(saved *savedSettings) {
	saved.SpeechSource = a.prefs.speechSource
	if tools, ok := a.pipeline.(media.Tools); ok {
		saved.WhisperModel = tools.WhisperModel
		saved.SpeechLanguage = tools.SpeechLanguage
	}
}

// restoreSpeech uses the model saved last time unless KANDO_WHISPER_MODEL names one or the saved file is gone, and the language and the download source saved last time.
func (a *app) restoreSpeech(saved savedSettings) {
	a.prefs.speechSource = saved.SpeechSource
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	if os.Getenv("KANDO_WHISPER_MODEL") == "" && saved.WhisperModel != "" {
		if info, err := os.Stat(saved.WhisperModel); err == nil && !info.IsDir() && info.Size() > 0 {
			tools.WhisperModel = saved.WhisperModel
		}
	}
	tools.SpeechLanguage = saved.SpeechLanguage
	a.pipeline = tools
}

// useSpeechLanguage tells whisper the language spoken in the videos from the next job on (a job already running keeps the language it started with): code is one of media.SpeechLanguages, or "" for whisper to detect it. Any other code is ignored.
func (a *app) useSpeechLanguage(code string) {
	tools, ok := a.pipeline.(media.Tools)
	if !ok || (code != "" && !media.KnownSpeechLanguage(code)) || tools.SpeechLanguage == code {
		return
	}
	tools.SpeechLanguage = code
	a.pipeline = tools
	if err := a.saveSettings(); err != nil {
		a.saveErr = a.messages().SettingsSpeechLanguageSaveFailed(err.Error())
	}
}

// useSpeechSource downloads the whisper models from source from the next download on (one already running keeps its source): source is one of media.SpeechSources. Any other source is ignored. Why the last download failed goes, as its hint to try the other source no longer applies.
func (a *app) useSpeechSource(source string) {
	if !slices.Contains(media.SpeechSources(), source) || a.prefs.speechSource == source {
		return
	}
	a.prefs.speechSource = source
	if errors.As(a.prefs.downloadErr, new(settings.DownloadError)) {
		a.prefs.downloadErr = nil
	}
	if err := a.saveSettings(); err != nil {
		a.saveErr = a.messages().SettingsSpeechSourceSaveFailed(err.Error())
	}
}

// speechDownloadFromMirror downloads the whisper catalog model id again from hf-mirror.com, which becomes the download source, as the button next to an error of a download from Hugging Face does (settings.MirrorHelps).
func (a *app) speechDownloadFromMirror(id string) {
	a.useSpeechSource(media.SpeechSourceMirror)
	a.downloadSpeechModel(id)
}

func (a *app) speechSearchDirs() []string {
	dirs := []string{a.prefs.modelsDir}
	if tools, ok := a.pipeline.(media.Tools); ok && tools.WhisperModel != "" {
		dirs = append(dirs, filepath.Dir(tools.WhisperModel))
	}
	return append(dirs, a.prefs.extraModelDirs...)
}

// findSpeechModels looks up which whisper catalog models are on disk, so the 语音识别 page reads them from memory. It runs when the settings window opens, and when the tools change (settingsHooks.toolsChanged): another model is chosen, a finished download included, or 重新检测.
func (a *app) findSpeechModels() {
	tools, _ := a.pipeline.(media.Tools)
	dirs := a.speechSearchDirs()
	found := map[string]string{}
	for _, model := range media.SpeechModels() {
		if path := media.ResolveSpeechModel(tools.WhisperModel, dirs, model.File); path != "" {
			found[model.ID] = path
		}
	}
	a.prefs.speechModels = found
}

func (a *app) useSpeechModel(path string) {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	tools.WhisperModel = path
	a.pipeline = tools
	a.areasToolsChanged(false)
	if err := a.saveSettings(); err != nil {
		a.saveErr = a.messages().SettingsSpeechSaveFailed(err.Error())
	}
}

// pickSpeechModel asks for a whisper.cpp model file already on the computer, as 选择模型文件… on the 语音识别 page does, holding the window's modal flag until the panel answers, and uses the one picked from the next job on. A file that is not a model, or one Kando cannot read, is not used; the page says why under the models.
func (a *app) pickSpeechModel() {
	if a.cx == nil {
		return
	}
	release := a.holdModal()
	// TODO(nsgo#199): give core.PickFilesOptions a title, a button label and a name for the filter, so the panel says 选择模型文件 over whisper 模型 as the button does; it shows the system's own words https://github.com/nsgo-dev/nsgo/issues/199
	a.cx.Run(func() {
		a.cx.PickFiles(ns.PickFilesOptions{Types: []string{".bin"}}, func(files []ns.File, err error) {
			release()
			defer a.Changed()
			if err != nil {
				a.prefs.downloadErr = err
				return
			}
			if len(files) == 0 || files[0].Path == "" {
				return
			}
			path := files[0].Path
			if err := media.CheckSpeechModelFile(path); err != nil {
				if !errors.Is(err, media.ErrNotSpeechModel) {
					log.Printf("settings: cannot read the speech model %s: %v", path, err)
				}
				a.prefs.downloadErr = settings.ModelFileError{Path: path, Err: err}
				return
			}
			a.prefs.downloadErr = nil
			a.useSpeechModel(path)
		})
	})
}

// whisperModelBaseURL is where downloadSpeechModel fetches the whisper models from, given the download source chosen. Tests point it at a server of their own, and speechModelFor at the files that server makes up.
var whisperModelBaseURL = media.SpeechSourceBaseURL

// speechModelFor is the whisper catalog model id, whose pinned size and checksum its download must match.
var speechModelFor = media.SpeechModelByID

// downloadSpeechModel downloads the whisper catalog model id from the download source chosen, checks it against the catalog's checksum and uses it. Why it failed is kept with the source it came from (settings.DownloadError), so the page can suggest the mirror when Hugging Face could not be reached.
func (a *app) downloadSpeechModel(id string) {
	model, ok := speechModelFor(id)
	if !ok || a.prefs.downloading != "" {
		return
	}
	if a.prefs.modelsDir == "" {
		a.prefs.downloadErr = settings.ErrNoModelsDir
		return
	}
	if a.privacy.offline {
		a.prefs.downloadErr = media.ErrOffline
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.prefs.downloading = id
	a.prefs.downloadWritten = 0
	a.prefs.downloadTotal = 0
	a.prefs.downloadErr = nil
	a.prefs.downloadCancel = cancel
	dir := a.prefs.modelsDir
	source := a.prefs.speechSource
	base := whisperModelBaseURL(source)
	update := a.update
	go func() {
		var last time.Time
		path, err := media.DownloadSpeechModel(ctx, base, model, dir, func(written, total int64) {
			if time.Since(last) < 200*time.Millisecond && written != total {
				return
			}
			last = time.Now()
			update(func() {
				if a.prefs.downloading != id {
					return
				}
				a.prefs.downloadWritten = written
				a.prefs.downloadTotal = total
			})
		})
		update(func() {
			if a.prefs.downloading != id {
				return
			}
			a.prefs.downloading = ""
			a.prefs.downloadCancel = nil
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					a.prefs.downloadErr = settings.DownloadError{ID: id, Source: source, Err: err}
				}
				return
			}
			a.useSpeechModel(path)
		})
	}()
}

// speechModelOwned reports whether the file of the whisper catalog model id is one Kando downloaded: a file right in its models folder, where downloadSpeechModel puts them. A model in another app's folder (defaultExtraModelDirs) or one KANDO_WHISPER_MODEL names elsewhere is not Kando's to move to the Trash.
func (a *app) speechModelOwned(id string) bool {
	path := a.prefs.speechModels[id]
	return path != "" && a.prefs.modelsDir != "" && filepath.Dir(filepath.Clean(path)) == filepath.Clean(a.prefs.modelsDir)
}

// speechModelInUse reports whether path is a model transcriptions use: the one chosen for the next job, or the one the running job took when it started (jobQueue.whisperModel), which it still needs until it returns.
func (a *app) speechModelInUse(path string) bool {
	same := func(model string) bool { return model != "" && filepath.Clean(model) == filepath.Clean(path) }
	tools, ok := a.pipeline.(media.Tools)
	return (ok && same(tools.WhisperModel)) || (a.queue.active != "" && same(a.queue.whisperModel))
}

// askTrashSpeechModel opens the confirmation of moving the model id to the Trash, holding the window's modal flag until it closes (closeTrashSpeechModel, or speechModals when its page goes away). A model Kando did not download is not asked about.
func (a *app) askTrashSpeechModel(id string) {
	if !a.speechModelOwned(id) {
		return
	}
	a.prefs.trashModel = id
	a.prefs.trashOpen = true
	if a.prefs.trashRelease == nil {
		a.prefs.trashRelease = a.holdModal()
	}
}

// closeTrashSpeechModel closes the trash confirmation as its 取消 or 移到废纸篓 button, Enter or Escape answers it, and gives back the modal flag it held right away, so the Changed after the answer already finds the menu bar's commands free.
func (a *app) closeTrashSpeechModel() {
	a.prefs.trashOpen = false
	if a.prefs.trashRelease != nil {
		a.prefs.trashRelease()
		a.prefs.trashRelease = nil
	}
}

// speechModals gives back the modal flag the trash confirmation held once it has closed, and reports whether it closed the confirmation or gave the flag back. It runs after every change (settingsHooks' afterChange), and as the settings close, for the confirmation that closes without being answered: the confirmation is drawn by the 语音识别 page, so it also closes when the window shows another section, as the main window can ask for (OpenSettings) while it is open; otherwise the flag would stay held with no confirmation on screen, and the menu bar would drop every command.
func (a *app) speechModals() bool {
	changed := false
	if a.prefs.trashOpen && (!a.prefs.open || a.SettingsSection() != settings.SectionSpeech) {
		a.prefs.trashOpen = false
		changed = true
	}
	if !a.prefs.trashOpen && a.prefs.trashRelease != nil {
		a.prefs.trashRelease()
		a.prefs.trashRelease = nil
		changed = true
	}
	return changed
}

// trashSpeechModel moves the file of the whisper catalog model id to the Trash, as the confirmation asks, and looks the models up again, so another copy, such as one another app keeps, shows in its place. It leaves alone a model Kando did not download and any while a download runs, and refuses the model in use, saying so under the models.
func (a *app) trashSpeechModel(id string) {
	path := a.prefs.speechModels[id]
	if !a.speechModelOwned(id) || a.prefs.downloading != "" {
		return
	}
	if a.speechModelInUse(path) {
		a.prefs.downloadErr = settings.TrashError{ID: id}
		return
	}
	if err := trashItem(a.cx, path); err != nil {
		a.prefs.downloadErr = settings.TrashError{ID: id, Err: err}
		return
	}
	a.prefs.downloadErr = nil
	a.findSpeechModels()
}

func (a *app) cancelDownload() {
	if a.prefs.downloadCancel != nil {
		a.prefs.downloadCancel()
	}
}

func (a *app) redetectTools() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	a.pipeline = tools.Relook()
	a.areasToolsChanged(true)
	a.loadToolVersions()
}

// loadToolVersions asks each installed tool for its version off the UI thread.
func (a *app) loadToolVersions() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	paths := []string{tools.YTDLP, tools.FFmpeg, tools.Whisper}
	a.prefs.versionGen++
	gen := a.prefs.versionGen
	update := a.update
	go func() {
		versions := map[string]string{}
		for _, path := range paths {
			if path == "" {
				continue
			}
			if version := toolVersion(path); version != "" {
				versions[path] = version
			}
		}
		update(func() {
			if a.prefs.versionGen != gen {
				return
			}
			a.prefs.toolVersions = versions
		})
	}()
}

func toolVersion(path string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	args := []string{"--version"}
	if strings.Contains(strings.ToLower(filepath.Base(path)), "ffmpeg") {
		args = []string{"-version"}
	}
	out, err := media.Command(ctx, path, args...).CombinedOutput()
	if len(out) == 0 || (err != nil && ctx.Err() != nil) {
		return ""
	}
	line := strings.TrimSpace(string(out))
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	return shortVersion(line)
}

func shortVersion(line string) string {
	fields := strings.Fields(line)
	if len(fields) >= 3 && strings.EqualFold(fields[0], "ffmpeg") && strings.EqualFold(fields[1], "version") {
		return fields[2]
	}
	lower := strings.ToLower(line)
	if i := strings.Index(lower, "version"); i >= 0 {
		rest := strings.Trim(line[i+len("version"):], " :\t")
		if parts := strings.Fields(rest); len(parts) > 0 {
			return strings.Trim(parts[0], ":")
		}
	}
	if len(line) > 48 {
		return line[:48]
	}
	return line
}

// defaultExtraModelDirs lists places other local apps keep whisper.cpp models, so one already on disk can be selected without copying it.
func defaultExtraModelDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(home, "Library", "Application Support", "Vimpo", "models"),
		filepath.Join(home, ".cache", "hyperframes", "whisper", "models"),
	}
}
