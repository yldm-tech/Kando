package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"lingocut/packages/media"
)

// Settings › 语音识别 and 通用: the whisper model, its download, and the tools' versions.

// savedSpeech is the speech model's part of settings.json.
type savedSpeech struct {
	WhisperModel string `json:"whisperModel,omitempty"`
}

func (s *savedSpeech) clean() { s.WhisperModel = strings.TrimSpace(s.WhisperModel) }

func (a *app) saveSpeech(saved *savedSettings) {
	if tools, ok := a.pipeline.(media.Tools); ok {
		saved.WhisperModel = tools.WhisperModel
	}
}

// restoreSpeech uses the model saved last time unless LINGOCUT_WHISPER_MODEL names one or the saved file is gone.
func (a *app) restoreSpeech(saved savedSettings) {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	if os.Getenv("LINGOCUT_WHISPER_MODEL") == "" && saved.WhisperModel != "" {
		if info, err := os.Stat(saved.WhisperModel); err == nil && !info.IsDir() && info.Size() > 0 {
			tools.WhisperModel = saved.WhisperModel
			a.pipeline = tools
		}
	}
}

func (a *app) speechSearchDirs() []string {
	dirs := []string{a.prefs.modelsDir}
	if tools, ok := a.pipeline.(media.Tools); ok && tools.WhisperModel != "" {
		dirs = append(dirs, filepath.Dir(tools.WhisperModel))
	}
	return append(dirs, a.prefs.extraModelDirs...)
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
		a.saveErr = "模型无法保存：" + err.Error()
	}
}

func (a *app) downloadSpeechModel(id string) {
	model, ok := media.SpeechModelByID(id)
	if !ok || a.prefs.downloading != "" {
		return
	}
	if a.prefs.modelsDir == "" {
		a.prefs.downloadErr = "还没有模型目录"
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.prefs.downloading = id
	a.prefs.downloadWritten = 0
	a.prefs.downloadTotal = 0
	a.prefs.downloadErr = ""
	a.prefs.downloadCancel = cancel
	dir := a.prefs.modelsDir
	update := a.update
	go func() {
		var last time.Time
		path, err := media.DownloadSpeechModel(ctx, media.WhisperModelBaseURL, model.File, dir, func(written, total int64) {
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
					a.prefs.downloadErr = err.Error()
				}
				return
			}
			a.useSpeechModel(path)
		})
	}()
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
