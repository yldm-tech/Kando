package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"kando/apps/kando/locales"
	"kando/apps/kando/settings"
	"kando/packages/media"
)

// Settings › 语音识别 and 通用: the whisper model, its download and its file, the language spoken in the videos, and the tools' versions.

// savedSpeech is the speech model's part of settings.json: the model's file, and the language spoken in the videos as whisper's code, empty for whisper to detect it.
type savedSpeech struct {
	WhisperModel   string `json:"whisperModel,omitempty"`
	SpeechLanguage string `json:"speechLanguage,omitempty"`
}

// clean trims the model's path and the language, and drops a language whisper is not offered for (media.SpeechLanguages), which leaves whisper to detect it.
func (s *savedSpeech) clean() {
	s.WhisperModel = strings.TrimSpace(s.WhisperModel)
	s.SpeechLanguage = strings.TrimSpace(s.SpeechLanguage)
	if !media.KnownSpeechLanguage(s.SpeechLanguage) {
		s.SpeechLanguage = ""
	}
}

func (a *app) saveSpeech(saved *savedSettings) {
	if tools, ok := a.pipeline.(media.Tools); ok {
		saved.WhisperModel = tools.WhisperModel
		saved.SpeechLanguage = tools.SpeechLanguage
	}
}

// restoreSpeech uses the model saved last time unless KANDO_WHISPER_MODEL names one or the saved file is gone, and the language saved last time.
func (a *app) restoreSpeech(saved savedSettings) {
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
		a.saveErr = locales.SettingsSpeechLanguageSaveFailed(err.Error())
	}
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
		a.saveErr = locales.SettingsSpeechSaveFailed(err.Error())
	}
}

// whisperModelBaseURL is where downloadSpeechModel fetches the whisper models from. Tests point it at a server of their own.
var whisperModelBaseURL = media.WhisperModelBaseURL

func (a *app) downloadSpeechModel(id string) {
	model, ok := media.SpeechModelByID(id)
	if !ok || a.prefs.downloading != "" {
		return
	}
	if a.prefs.modelsDir == "" {
		a.prefs.downloadErr = errSpeechNoModelsDir
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.prefs.downloading = id
	a.prefs.downloadWritten = 0
	a.prefs.downloadTotal = 0
	a.prefs.downloadErr = nil
	a.prefs.downloadCancel = cancel
	dir := a.prefs.modelsDir
	update := a.update
	go func() {
		var last time.Time
		path, err := media.DownloadSpeechModel(ctx, whisperModelBaseURL, model.File, dir, func(written, total int64) {
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
					a.prefs.downloadErr = err
				}
				return
			}
			a.useSpeechModel(path)
		})
	}()
}

// speechTrashError is why trashSpeechModel left the whisper catalog model id where it was: err, or the model being in use when err is nil. speechDownloadError words it in the language it is shown in.
type speechTrashError struct {
	id  string
	err error
}

func (e speechTrashError) Error() string {
	if e.err == nil {
		return e.id + " is in use"
	}
	return e.id + ": " + e.err.Error()
}

// errSpeechNoModelsDir is downloadSpeechModel's error when Kando has no folder to put the models in.
var errSpeechNoModelsDir = errors.New("no models folder")

// speechDownloadError says in plain words, in the interface's language, why a speech model's download failed, or "" when it did not. A stalled or dropped download kept what it got, so these say that downloading again picks up from there.
func speechDownloadError(err error) string {
	var status interface{ StatusCode() int }
	var trash speechTrashError
	switch {
	case err == nil:
		return ""
	case errors.As(err, &trash) && trash.err == nil:
		return locales.SettingsSpeechTrashInUse(settings.SpeechModelName(trash.id))
	case errors.As(err, &trash):
		return locales.SettingsSpeechTrashFailed(settings.SpeechModelName(trash.id), trash.err.Error())
	case errors.Is(err, errSpeechNoModelsDir):
		return locales.SettingsSpeechNoModelsDir()
	case errors.Is(err, media.ErrStalled):
		return locales.SettingsSpeechDownloadStalled()
	case errors.As(err, &status):
		return locales.SettingsSpeechDownloadFailedStatus(status.StatusCode())
	case errors.Is(err, media.ErrDisconnected):
		return locales.SettingsSpeechDownloadDisconnected()
	}
	return err.Error()
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

// askTrashSpeechModel opens the confirmation of moving the model id to the Trash, holding the window's modal flag until it closes (speechModals). A model Kando did not download is not asked about.
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

// speechModals gives back the modal flag the trash confirmation held once it has closed. The settings window calls it after each of its frames, and as it closes. The confirmation is drawn by the 语音识别 page, so it also closes when the window shows another section, as the main window can ask for (OpenSettings) while it is open; otherwise the flag would stay held with no confirmation on screen, and the menu bar would drop every command.
func (a *app) speechModals() {
	if a.prefs.trashOpen && (!a.prefs.open || a.SettingsSection() != settings.SectionSpeech) {
		a.prefs.trashOpen = false
	}
	if !a.prefs.trashOpen && a.prefs.trashRelease != nil {
		a.prefs.trashRelease()
		a.prefs.trashRelease = nil
	}
}

// trashSpeechModel moves the file of the whisper catalog model id to the Trash, as the confirmation asks, and looks the models up again, so another copy, such as one another app keeps, shows in its place. It leaves alone a model Kando did not download and any while a download runs, and refuses the model in use, saying so under the models.
func (a *app) trashSpeechModel(id string) {
	path := a.prefs.speechModels[id]
	if !a.speechModelOwned(id) || a.prefs.downloading != "" {
		return
	}
	if a.speechModelInUse(path) {
		a.prefs.downloadErr = speechTrashError{id: id}
		return
	}
	if err := trashItem(path); err != nil {
		a.prefs.downloadErr = speechTrashError{id: id, err: err}
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
	out, err := exec.CommandContext(ctx, path, args...).CombinedOutput()
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
