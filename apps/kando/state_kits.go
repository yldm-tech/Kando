package main

import (
	"context"
	"errors"
	"time"

	"kando/packages/media"
)

// kitsUI is the state of the programs Kando downloads for itself (media.Kit): yt-dlp, ffmpeg and whisper-cli, each the moment the window is ready when the computer has none, into media.OwnToolsDir (main sets it; empty, as in tests that leave it, nothing is downloaded), so a first launch needs nothing installed. The first-run guide and Settings › 通用 show their progress, and 下载 or 重试 there starts one again.
type kitsUI struct {
	// running is the download of each program under way, by program.
	running map[string]*kitDownload
	// errs is why the last download of each program failed, by program, until it is started again.
	errs map[string]string
	// install downloads a kit; tests replace it.
	install func(ctx context.Context, kit media.Kit, dir string, progress func(written, total int64)) (string, error)
}

// kitDownload is one download under way: how far it has come, and how to stop it.
type kitDownload struct {
	written, total int64
	cancel         context.CancelFunc
}

func newKitsUI() kitsUI {
	return kitsUI{running: map[string]*kitDownload{}, errs: map[string]string{}, install: media.InstallKit}
}

func (a *app) kitsHooks() areaHooks {
	return areaHooks{
		ready:    a.kitsInstallMissing,
		quitting: a.kitsCancel,
	}
}

// kitsInstallMissing starts downloading every program the pipeline has not found that Kando can download for this platform.
func (a *app) kitsInstallMissing() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	for _, tool := range []struct{ program, path string }{{"yt-dlp", tools.YTDLP}, {"ffmpeg", tools.FFmpeg}, {"whisper-cli", tools.Whisper}} {
		if tool.path == "" {
			a.kitsInstall(tool.program)
		}
	}
}

// kitsCanInstall reports whether Kando can download program here.
func (a *app) kitsCanInstall(program string) bool {
	_, ok := media.KitFor(program)
	return ok && media.OwnToolsDir != ""
}

// kitsInstall downloads program off the UI thread, unless it is already downloading. Once it is in, the tools are looked up again, which finds it.
func (a *app) kitsInstall(program string) {
	kit, ok := media.KitFor(program)
	if !ok || media.OwnToolsDir == "" || a.kits.running[program] != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	job := &kitDownload{total: kit.Size, cancel: cancel}
	a.kits.running[program] = job
	delete(a.kits.errs, program)
	dir, install, update := media.OwnToolsDir, a.kits.install, a.update
	go func() {
		var last time.Time
		_, err := install(ctx, kit, dir, func(written, total int64) {
			if time.Since(last) < 200*time.Millisecond && written != total {
				return
			}
			last = time.Now()
			update(func() {
				if a.kits.running[program] != job {
					return
				}
				job.written, job.total = written, total
			})
		})
		update(func() {
			if a.kits.running[program] != job {
				return
			}
			delete(a.kits.running, program)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					a.kits.errs[program] = err.Error()
				}
				return
			}
			a.redetectTools()
		})
	}()
}

// kitsCancel stops every download under way.
func (a *app) kitsCancel() {
	for _, job := range a.kits.running {
		job.cancel()
	}
}

// kitsProgress is how far the download of program has come, from 0 to 1, or -1 before its size is known; ok is false when it is not downloading.
func (a *app) kitsProgress(program string) (progress float64, ok bool) {
	job := a.kits.running[program]
	if job == nil {
		return 0, false
	}
	if job.total <= 0 {
		return -1, true
	}
	return min(float64(job.written)/float64(job.total), 1), true
}

// kitsDownloading is the programs downloading now, in the order the pipeline runs them.
func (a *app) kitsDownloading() []string {
	var names []string
	for _, program := range []string{"yt-dlp", "ffmpeg", "whisper-cli"} {
		if a.kits.running[program] != nil {
			names = append(names, program)
		}
	}
	return names
}
