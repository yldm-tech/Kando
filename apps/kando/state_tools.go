package main

import (
	"context"
	"maps"
	"slices"

	"kando/apps/kando/jobfailure"
	"kando/apps/kando/toolhealth"
	"kando/packages/core"
	"kando/packages/media"
)

// toolsUI is what the app knows of the copies of the programs it runs (toolhealth.Programs) beyond where they are: whether each copy starts and answers, which the app finds by running it once (media.Probe) off the UI thread when the window is ready, after 重新检测 and once a download of Kando's own copy lands (redetectTools), and after a job failed at a stage one of them runs; and the copy of a program the user chose to run, Kando's own or the computer's, which settings.json keeps. The copy the pipeline runs is the first that works in the order Kando prefers them (toolhealth.Pick), so a copy that cannot start, as a Homebrew ffmpeg whose libraries Homebrew upgraded under it, gives way to one that can, and the settings, the sidebar, the first-run guide, a failed job's card, the control socket and the feedback say so rather than calling it installed.
type toolsUI struct {
	// health is how each copy answered when it was last run, by path; a copy not in it has not been run yet.
	health map[string]media.Health
	// others is, by program, a copy on the computer besides Kando's own that works, which the program's row lets the user choose instead (toolhealth.Pick).
	others map[string]string
	// choice is, by program, the copy the user chose to run: toolhealth.Own or toolhealth.Computer.
	choice map[string]string
	// gen numbers the checks, so the answer of one started before another is dropped. ready is set once the window is ready, before which a failed job does not start a check.
	gen   int
	ready bool
	// failed is the failed jobs a check has been started for, by ID, so a job that fails starts one check and no more (toolsAfterFailure).
	failed map[string]bool
	// retry is, by program, the failed jobs to run again once the program works, which 下载 … 并重试 on their cards asked for (toolsRetryWhenFixed).
	retry map[string][]string
	// candidates and probe find the copies of a program and run one; tests replace them, as the copies on the machine running them are not the test's.
	candidates func(program string) []string
	probe      func(ctx context.Context, program, path string) media.Health
}

func newToolsUI() toolsUI {
	return toolsUI{health: map[string]media.Health{}, others: map[string]string{}, choice: map[string]string{}, failed: map[string]bool{}, retry: map[string][]string{},
		candidates: media.Candidates, probe: media.Probe}
}

// savedTools is the tools' part of settings.json: the copy the user chose to run of each program they chose one of, Kando's own or the computer's.
type savedTools struct {
	ToolCopies map[string]string `json:"tool_copies,omitempty"`
}

func (s *savedTools) clean() {
	maps.DeleteFunc(s.ToolCopies, func(program, copy string) bool {
		return !slices.Contains(toolhealth.Programs, program) || (copy != toolhealth.Own && copy != toolhealth.Computer)
	})
}

// toolsHooks: the copies the user chose are saved with the settings and applied to the pipeline at startup; the copies are run once the window is ready; and a job that failed at a stage a program runs has the copies run again, as the program may have broken since.
func (a *app) toolsHooks() areaHooks {
	return areaHooks{
		save: func(saved *savedSettings) {
			saved.savedTools = savedTools{}
			if len(a.tools.choice) > 0 {
				saved.ToolCopies = maps.Clone(a.tools.choice)
			}
		},
		restore: func(saved savedSettings) {
			maps.Copy(a.tools.choice, saved.ToolCopies)
			if tools, ok := a.pipeline.(media.Tools); ok {
				a.pipeline = a.toolsChoose(tools)
			}
		},
		ready: func() {
			a.tools.ready = true
			a.toolsCheck()
		},
		shelfChanged: a.toolsAfterFailure,
	}
}

// redetectTools looks for the programs again, 重新检测 and once a download of Kando's own copy landed, keeps the copies the user chose, has the areas follow, and runs the copies to check them.
func (a *app) redetectTools() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	a.pipeline = a.toolsChoose(tools.Relook())
	a.areasToolsChanged(true)
	a.toolsCheck()
}

// toolsChoose puts in tools, for each program the user chose a copy of, that copy: Kando's own, when it has one, or the first on the computer, until the copies are run and checked (toolsCheck).
func (a *app) toolsChoose(tools media.Tools) media.Tools {
	for _, program := range toolhealth.Programs {
		own := media.OwnTool(program)
		switch a.tools.choice[program] {
		case toolhealth.Own:
			if own != "" {
				*toolPath(&tools, program) = own
			}
		case toolhealth.Computer:
			for _, path := range a.tools.candidates(program) {
				if path != own {
					*toolPath(&tools, program) = path
					break
				}
			}
		}
	}
	return tools
}

// toolPath is where tools keeps the path of program.
func toolPath(tools *media.Tools, program string) *string {
	switch program {
	case "yt-dlp":
		return &tools.YTDLP
	case "ffmpeg":
		return &tools.FFmpeg
	}
	return &tools.Whisper
}

// toolsCheck finds every copy of each program and runs them off the UI thread, one at a time (macOS stops a burst of new processes of one program started together), and then has the pipeline run the first copy of each that works (toolhealth.Pick), and the areas follow; it starts the downloads Kando wants now that it knows which copies work (kitsInstallWanted), and runs again the jobs waiting for a program that works now (toolsRetryWhenFixed). The copy the pipeline runs is run too when the lookup does not find it, as one a test or KANDO_ variable names.
func (a *app) toolsCheck() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok || a.update == nil {
		return
	}
	a.tools.gen++
	gen, choice, probe, update := a.tools.gen, maps.Clone(a.tools.choice), a.tools.probe, a.update
	// The copies are found here, as Relook finds them, which only looks at files; running them is what takes long.
	found := map[string][]string{}
	own := map[string]string{}
	for _, program := range toolhealth.Programs {
		list := a.tools.candidates(program)
		if path := *toolPath(&tools, program); path != "" && !slices.Contains(list, path) {
			list = append([]string{path}, list...)
		}
		found[program], own[program] = list, media.OwnTool(program)
	}
	go func() {
		health := map[string]media.Health{}
		for _, program := range toolhealth.Programs {
			for _, path := range found[program] {
				health[path] = probe(context.Background(), program, path)
			}
		}
		update(func() {
			if gen != a.tools.gen {
				return
			}
			a.tools.health = health
			chosen, ok := a.pipeline.(media.Tools)
			if !ok {
				return
			}
			before := chosen
			for _, program := range toolhealth.Programs {
				*toolPath(&chosen, program), a.tools.others[program] = toolhealth.Pick(found[program], own[program], choice[program], health)
			}
			a.pipeline = chosen
			if chosen.YTDLP != before.YTDLP || chosen.FFmpeg != before.FFmpeg || chosen.Whisper != before.Whisper {
				a.areasToolsChanged(false)
			} else {
				a.checkReadiness(false)
			}
			a.kitsInstallWanted()
			a.toolsRetryWhenFixed()
		})
	}()
}

// toolHealth is how the copy of program the pipeline runs answered, with its path; a zero Health when it has not been run yet, and an empty path when there is none.
func (a *app) toolHealth(program string) (string, media.Health) {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return "", media.Health{}
	}
	path := *toolPath(&tools, program)
	return path, a.tools.health[path]
}

// toolUsable reports whether program has a copy that is not known to be broken, which a job can run. Another pipeline, as a test's, runs no programs of its own and has all of them.
func (a *app) toolUsable(program string) bool {
	if _, ok := a.pipeline.(media.Tools); !ok {
		return true
	}
	path, health := a.toolHealth(program)
	return path != "" && health.State != media.HealthBroken
}

// toolsBroken is the programs whose copy is known not to run, in the order the pipeline runs them.
func (a *app) toolsBroken() []string {
	var names []string
	for _, program := range toolhealth.Programs {
		if path, health := a.toolHealth(program); path != "" && health.State == media.HealthBroken {
			names = append(names, program)
		}
	}
	return names
}

// toolsUse runs the copy of program the user chose, toolhealth.Own or toolhealth.Computer, saves the choice and checks the copies again.
func (a *app) toolsUse(program, choice string) {
	a.tools.choice[program] = choice
	a.Save()
	if tools, ok := a.pipeline.(media.Tools); ok {
		a.pipeline = a.toolsChoose(tools)
		a.areasToolsChanged(false)
	}
	a.toolsCheck()
}

// toolsUseOwn runs Kando's own copy of program from now on, downloading it again when it has none, or one that does not run: 改用看懂自带的 and 重新下载 in Settings › 通用, and 下载 … 并重试 on a failed job's card. The choice stays, so a computer's copy that works again later does not take over, as on Linux, where Kando's own ffmpeg otherwise comes after the computer's.
func (a *app) toolsUseOwn(program string) {
	a.toolsUse(program, toolhealth.Own)
	own := media.OwnTool(program)
	if own == "" || a.tools.health[own].State == media.HealthBroken {
		a.kitsInstall(program)
	}
}

// toolsAfterFailure checks the copies again when a job has failed at a stage one of the programs runs since the last check, as a program can break while Kando is open, when the package manager upgrades what it is built on. A job that had failed already when Kando started, or before the window was ready, starts none.
func (a *app) toolsAfterFailure() {
	check := false
	for _, job := range a.library.Jobs {
		if job.Stage != core.StageFailed || job.Err == nil || a.tools.failed[job.ID] {
			continue
		}
		a.tools.failed[job.ID] = true
		if len(toolhealth.StageTools(jobfailure.Stage(job))) > 0 {
			check = true
		}
	}
	if check && a.tools.ready {
		a.toolsCheck()
	}
}

// toolBrokenFor is the program that stopped job: the first program the stage it failed at runs whose copy is known not to run now. ok is false for a job that did not fail, or one whose programs all work.
func (a *app) toolBrokenFor(job core.Job) (program string, ok bool) {
	if job.Stage != core.StageFailed {
		return "", false
	}
	for _, program := range toolhealth.StageTools(jobfailure.Stage(job)) {
		if path, health := a.toolHealth(program); path != "" && health.State == media.HealthBroken {
			return program, true
		}
	}
	return "", false
}

// toolsRetryWhenFixed runs again the jobs waiting for a program, once its copy is known to work: one not run yet may still not.
func (a *app) toolsRetryWhenFixed() {
	for _, program := range toolhealth.Programs {
		ids := a.tools.retry[program]
		if _, health := a.toolHealth(program); len(ids) == 0 || health.State != media.HealthReady {
			continue
		}
		delete(a.tools.retry, program)
		for _, id := range ids {
			if job, ok := a.library.Find(id); ok && job.Stage == core.StageFailed {
				a.Retry(id)
			}
		}
	}
}
