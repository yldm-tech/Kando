package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"kando/apps/kando/feedback"
	"kando/apps/kando/locales"
	"kando/packages/core"

	"github.com/egoist/mygo"
)

// logsUI is the state of the app's log file (feedback/logfile.go): the file the standard logger writes to, and the stage each job of the open shelf was in when the log last looked at it, so a job that fails is logged once.
type logsUI struct {
	// file is the log file once the window is ready; nil before, in tests that do not open one, and when the logs folder could not be written.
	file   *feedback.LogFile
	stages map[string]core.Stage
}

func newLogsUI() logsUI { return logsUI{} }

// logsDir is the folder the log file goes in: the system's place for the app's logs, ~/Library/Logs/Kando on macOS. Tests point it at a folder of their own.
var logsDir = func() (string, error) { return mygo.App.Path(mygo.PathLogs) }

// logsReveal shows a file in Finder or the file manager. Tests replace it.
var logsReveal = mygo.Shell.ShowItemInFolder

// logsHooks: the log file opens once the window is ready, which is only in main, so a test's app does not write the user's log; its area runs first, so what the other areas log as they get ready reaches the file. At quitting it only says so and stays open: its area also runs first there, and what the other areas log as they stop, such as remote's control socket, must reach the file too; the process's exit closes it, and nothing is buffered. Each time the shelf changes, a job that has just failed is logged with the stage it failed in and its whole error; the jobs the shelf was read with are only looked at, as they failed in an earlier run.
func (a *app) logsHooks() areaHooks {
	return areaHooks{
		ready:        a.logsOpen,
		quitting:     a.logsQuitting,
		shelfLoaded:  func() { a.logsJobs(false) },
		shelfChanged: func() { a.logsJobs(true) },
	}
}

// logsKeys are the log's rows of the shortcut table: 显示日志文件 in the 帮助 menu, without a key, while there is a log file.
func (a *app) logsKeys() []shortcut {
	return []shortcut{
		{Group: groupHelp, Label: locales.LogsShowLogFile(), When: func() bool { return a.logs.file != nil }, Run: a.logsShow},
	}
}

// logsOpen opens the log file in logsDir and sends the standard logger's lines to it as well as to stderr. A folder that cannot be written leaves the log on stderr alone, and says why there.
func (a *app) logsOpen() {
	if a.logs.file != nil {
		return
	}
	dir, err := logsDir()
	if err != nil {
		log.Printf("logs: no folder for the log file: %v", err)
		return
	}
	file, err := feedback.OpenLogFile(filepath.Join(dir, feedback.LogName), feedback.LogLimit, os.Stderr)
	if err != nil {
		log.Printf("logs: cannot open the log file: %v", err)
		return
	}
	a.logs.file = file
	log.SetOutput(file)
	log.Printf("Kando %s started on %s/%s", appVersion, runtime.GOOS, runtime.GOARCH)
}

// logsQuitting notes in the log that the app is quitting, leaving the file open for what the other areas log as they stop.
func (a *app) logsQuitting() {
	if a.logs.file != nil {
		log.Print("Kando is quitting")
	}
}

// logsClose closes the log file; the standard logger writes to stderr alone again. A test closes its log with it; the app leaves its own open until the process ends (logsQuitting).
func (a *app) logsClose() {
	if a.logs.file == nil {
		return
	}
	log.SetOutput(os.Stderr)
	a.logs.file.Close()
	a.logs.file = nil
}

// logsShow reveals the log file in Finder or the file manager, so it can be attached to a report.
func (a *app) logsShow() {
	if a.logs.file != nil {
		logsReveal(a.logs.file.Path())
	}
}

// logsJobs records the stage of each job on the shelf. With report set, a job that is failed now and was not when the log last looked is logged: its ID, the stage it was in, and its whole error, which its card shortens.
func (a *app) logsJobs(report bool) {
	stages := make(map[string]core.Stage, len(a.library.Jobs))
	for _, job := range a.library.Jobs {
		stages[job.ID] = job.Stage
		before, seen := a.logs.stages[job.ID]
		if !report || job.Stage != core.StageFailed || before == core.StageFailed {
			continue
		}
		if !seen {
			before = "added"
		}
		a.logsJobFailed(job, before)
	}
	a.logs.stages = stages
}

// logsJobFailed logs that job failed while it was in stage before. Its error goes through the credentials the app holds first (feedbackSecrets): a translation's error can carry the AI service's address, which the log must not keep.
func (a *app) logsJobFailed(job core.Job, before core.Stage) {
	reason := "no error recorded"
	if job.Err != nil {
		reason = job.Err.Error()
	}
	reason = feedback.Scrubber{Secrets: a.feedbackSecrets()}.NoSecrets(reason)
	log.Printf("job %s failed while %s: %s", job.ID, before, reason)
}
