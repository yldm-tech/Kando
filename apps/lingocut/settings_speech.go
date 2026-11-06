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

func (a *app) speechSearchDirs() []string {
	dirs := []string{a.modelsDir}
	if tools, ok := a.pipeline.(media.Tools); ok && tools.WhisperModel != "" {
		dirs = append(dirs, filepath.Dir(tools.WhisperModel))
	}
	return append(dirs, a.extraModelDirs...)
}

func (a *app) useSpeechModel(path string) {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	tools.WhisperModel = path
	a.pipeline = tools
	if err := a.saveSettings(); err != nil {
		a.saveErr = "模型无法保存：" + err.Error()
	}
}

func (a *app) downloadSpeechModel(id string) {
	model, ok := media.SpeechModelByID(id)
	if !ok || a.downloading != "" {
		return
	}
	if a.modelsDir == "" {
		a.downloadErr = "还没有模型目录"
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.downloading = id
	a.downloadWritten = 0
	a.downloadTotal = 0
	a.downloadErr = ""
	a.downloadCancel = cancel
	dir := a.modelsDir
	update := a.update
	go func() {
		var last time.Time
		path, err := media.DownloadSpeechModel(ctx, media.WhisperModelBaseURL, model.File, dir, func(written, total int64) {
			if time.Since(last) < 200*time.Millisecond && written != total {
				return
			}
			last = time.Now()
			update(func() {
				if a.downloading != id {
					return
				}
				a.downloadWritten = written
				a.downloadTotal = total
			})
		})
		update(func() {
			if a.downloading != id {
				return
			}
			a.downloading = ""
			a.downloadCancel = nil
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					a.downloadErr = err.Error()
				}
				return
			}
			a.useSpeechModel(path)
		})
	}()
}

func (a *app) cancelDownload() {
	if a.downloadCancel != nil {
		a.downloadCancel()
	}
}

func (a *app) redetectTools() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	a.pipeline = tools.Relook()
	a.loadToolVersions()
}

// loadToolVersions asks each installed tool for its version off the UI thread.
func (a *app) loadToolVersions() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	paths := []string{tools.YTDLP, tools.FFmpeg, tools.Whisper}
	a.versionGen++
	gen := a.versionGen
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
			if a.versionGen != gen {
				return
			}
			a.toolVersions = versions
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
