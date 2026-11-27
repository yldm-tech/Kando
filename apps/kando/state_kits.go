package main

import (
	"context"
	"errors"
	"time"

	"kando/apps/kando/toolhealth"
	"kando/packages/media"
)

// kitsUI is the state of the programs Kando downloads for itself (media.Kit): yt-dlp, ffmpeg and whisper-cli, into media.OwnToolsDir (main sets it; empty, as in tests that leave it, nothing is downloaded). Kando runs its own copy first (media.Candidates), so a package manager that upgrades what the computer's copy is built on cannot break it, and so it downloads each kit the moment the window is ready even when the computer has the program, except a kit that is only a fallback, which it downloads only when the computer has no copy that works (kitsInstallWanted). The first-run guide and Settings › 通用 show the downloads of programs the computer lacks, and 下载 or 重试 there starts one again; a download while the computer's copy works is quiet: it is not shown, and when it fails it is not tried again until Kando starts again or the user asks for it.
type kitsUI struct {
	// running is the download of each program under way, by program.
	running map[string]*kitDownload
	// errs is why the last download of each program failed, by program, until it is started again; settings.KitsError words it in the language it is shown in.
	errs map[string]error
	// quietFailed is the programs whose quiet download failed since Kando started, which are not tried again on their own.
	quietFailed map[string]bool
	// relook is set when a quiet download landed while another was still under way: the tools are looked up again once, when the last one ends, rather than after each.
	relook bool
	// install downloads a kit; tests replace it.
	install func(ctx context.Context, kit media.Kit, dir string, progress func(written, total int64)) (string, error)
}

// kitDownload is one download under way: how far it has come, and how to stop it. quiet is set while the computer's copy of the program works, so nothing shows the download.
type kitDownload struct {
	written, total int64
	cancel         context.CancelFunc
	quiet          bool
}

func newKitsUI() kitsUI {
	return kitsUI{running: map[string]*kitDownload{}, errs: map[string]error{}, quietFailed: map[string]bool{}, install: media.InstallKit}
}

func (a *app) kitsHooks() areaHooks {
	return areaHooks{
		ready:    a.kitsInstallWanted,
		quitting: a.kitsCancel,
	}
}

// kitsInstallWanted starts downloading every kit Kando wants and has not got: one it runs first, as long as it has none; a fallback (media.Kit.Fallback), when the computer has no copy that works; and one whose copy is of an earlier version than this Kando pins (media.KitStale). It runs when the window is ready and after each check of the tools (toolsCheck), which tells whether the computer's copies work. A download for a program whose current copy works is quiet, and one that failed quietly is not started again.
func (a *app) kitsInstallWanted() {
	if _, ok := a.pipeline.(media.Tools); !ok {
		return
	}
	for _, program := range toolhealth.Programs {
		kit, ok := media.KitFor(program)
		if !ok || a.kits.quietFailed[program] {
			continue
		}
		usable := a.toolUsable(program)
		own := media.OwnTool(program)
		switch {
		case own == "" && (!kit.Fallback || !usable):
			a.kitsStart(program, usable)
		case own != "" && media.KitStale(kit):
			a.kitsStart(program, true)
		}
	}
	a.kitsDenoWanted()
}

// kitsDenoWanted starts downloading Kando's own Deno, quietly, when the computer has none and Kando has not got it, or has an earlier version than this Kando pins: yt-dlp solves YouTube's challenges with it (media.Tools.Deno). Deno is not one of the programs a job needs (toolhealth.Programs), so it is not run to check it, and the computer's copy is taken as it is found.
func (a *app) kitsDenoWanted() {
	kit, ok := media.KitFor("deno")
	if !ok || a.kits.quietFailed["deno"] {
		return
	}
	own := media.OwnTool("deno")
	switch {
	case own == "" && len(a.tools.candidates("deno")) == 0:
		a.kitsStart("deno", true)
	case own != "" && media.KitStale(kit):
		a.kitsStart("deno", true)
	}
}

// kitsCanInstall reports whether Kando can download program here.
func (a *app) kitsCanInstall(program string) bool {
	_, ok := media.KitFor(program)
	return ok && media.OwnToolsDir != ""
}

// kitsInstall downloads program because the user asked for it, from a button: never quiet, and again after a quiet download failed. A quiet download of it under way is shown from now on.
func (a *app) kitsInstall(program string) {
	delete(a.kits.quietFailed, program)
	a.kitsStart(program, false)
}

// kitsStart downloads program off the UI thread, unless it is already downloading, quiet or not. Once it is in, the tools are looked up again, which finds it, and checked (redetectTools).
func (a *app) kitsStart(program string, quiet bool) {
	kit, ok := media.KitFor(program)
	if !ok || media.OwnToolsDir == "" {
		return
	}
	if job := a.kits.running[program]; job != nil {
		job.quiet = job.quiet && quiet
		return
	}
	if a.privacy.offline {
		if !quiet {
			a.kits.errs[program] = media.ErrOffline
		}
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	job := &kitDownload{total: kit.Size, cancel: cancel, quiet: quiet}
	a.kits.running[program] = job
	delete(a.kits.errs, program)
	dir, install, update := media.OwnToolsDir, a.kits.install, a.update
	a.tools.work.Go(func() {
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
			switch {
			case err != nil && job.quiet:
				a.kits.quietFailed[program] = true
			case err != nil && !errors.Is(err, context.Canceled):
				a.kits.errs[program] = err
			case err == nil && job.quiet:
				// The program works as it is: the tools are looked up once, when the last download ends.
				a.kits.relook = true
			case err == nil:
				// The user is waiting for this one: it is used at once.
				a.kits.relook = false
				a.redetectTools()
				return
			}
			if a.kits.relook && len(a.kits.running) == 0 {
				a.kits.relook = false
				a.redetectTools()
			}
		})
	})
}

// kitsCancel stops every download under way.
func (a *app) kitsCancel() {
	for _, job := range a.kits.running {
		job.cancel()
	}
}

// kitsProgress is how far the download of program has come, from 0 to 1, or -1 before its size is known; ok is false when it is not downloading, or downloading quietly.
func (a *app) kitsProgress(program string) (progress float64, ok bool) {
	job := a.kits.running[program]
	if job == nil || job.quiet {
		return 0, false
	}
	if job.total <= 0 {
		return -1, true
	}
	return min(float64(job.written)/float64(job.total), 1), true
}

// kitsDownloading is the programs downloading now, but quietly, in the order the pipeline runs them.
func (a *app) kitsDownloading() []string {
	var names []string
	for _, program := range toolhealth.Programs {
		if job := a.kits.running[program]; job != nil && !job.quiet {
			names = append(names, program)
		}
	}
	return names
}
