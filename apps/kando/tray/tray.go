// Package tray is what Kando's status icon tells and offers: in the menu bar on macOS, the notification area on Windows and the system tray on Linux. It works out the icon's text, tooltip, image and menu from the queue's state; the app (state_tray.go) shows them through NSGo's desktop.NewTray and follows the queue with them.
package tray

import (
	"math"
	"unicode/utf8"

	"kando/apps/kando/locales"
	"kando/packages/core"

	ns "github.com/nsgo-dev/nsgo/core"
)

// Job is the job the queue runs now, as the icon tells of it.
type Job struct {
	Title    string
	Stage    core.Stage
	Progress core.Progress
}

// Status is the queue as the icon tells of it: the job running, if any, how many wait behind it, whether the queue is paused, and whether the main window is put away.
type Status struct {
	Running *Job
	Waiting int
	Paused  bool
	Hidden  bool
}

// stageSpans are the parts of a whole job each stage takes, in percent, as the design's tray counts them: the download a fifth, the transcription and the translation most of the rest, and the export the last tenth.
var stageSpans = map[core.Stage][2]float64{
	core.StageDownloading:  {0, 20},
	core.StageTranscribing: {20, 50},
	core.StageTranslating:  {50, 90},
	core.StageExporting:    {90, 100},
}

// Overall is how far a job in stage, its stage at p, has gone of the whole job, in whole percent from 0 to 100. A stage that has not said how far it is counts from its start; a job done is 100, one waiting or failed 0.
func Overall(stage core.Stage, p core.Progress) int {
	if stage == core.StageComplete {
		return 100
	}
	span, ok := stageSpans[stage]
	if !ok {
		return 0
	}
	f := 0.0
	if p.Known {
		f = min(max(p.Fraction, 0), 1)
	}
	return int(math.Round(span[0] + (span[1]-span[0])*f))
}

// percent is how far the running job has gone of the whole job.
func (s Status) percent() int {
	if s.Running == nil {
		return 0
	}
	return Overall(s.Running.Stage, s.Running.Progress)
}

// Title is the text beside the icon, which macOS shows: the running job's percentage, 已暂停 while the queue is paused, and none while nothing runs.
func Title(m locales.Messages, s Status) string {
	switch {
	case s.Paused:
		return m.TrayPaused()
	case s.Running != nil:
		return m.TrayPercent(s.percent())
	}
	return ""
}

// titleRunes is how much of a film's title the menu and the tooltip show, so a long one does not stretch the menu across the screen.
const titleRunes = 40

// short cuts title to titleRunes, with an ellipsis where it was cut.
func short(title string) string {
	if utf8.RuneCountInString(title) <= titleRunes {
		return title
	}
	return string([]rune(title)[:titleRunes-1]) + "…"
}

// Tooltip is what shows as the pointer rests on the icon: the app's name, or the running job and how far it has gone, or that the queue is paused.
func Tooltip(m locales.Messages, s Status) string {
	switch {
	case s.Paused:
		return m.TrayTooltipPaused()
	case s.Running != nil:
		return m.TrayTooltipJob(short(s.Running.Title), s.percent())
	}
	return m.AppName()
}

// state is the menu's first line: what Kando is doing.
func state(m locales.Messages, s Status) string {
	switch {
	case s.Paused:
		return m.TrayStatePaused()
	case s.Running != nil && s.Hidden:
		return m.TrayStateBackground()
	case s.Running != nil:
		return m.TrayStateRunning()
	}
	return m.TrayStateIdle()
}

// StageName is a stage as the menu names it, in the language of m.
func StageName(m locales.Messages, job Job) string {
	switch job.Stage {
	case core.StageDownloading:
		return m.FilmsStageDownloading()
	case core.StageTranscribing:
		return m.FilmsStageTranscribing()
	case core.StageTranslating:
		return m.FilmsStageTranslating()
	case core.StageExporting:
		return m.FilmsStageExporting()
	}
	return m.FilmsStageQueued()
}

// Actions are what the menu's commands do.
type Actions struct {
	// Pause pauses the queue, or resumes it while it is paused.
	Pause func()
	// Open brings the main window to the front; Settings opens the settings; Quit quits, asking first while a job runs.
	Open, Settings, Quit func()
}

// Menu is the icon's menu: what Kando is doing, the job running and how many wait, then 暂停处理队列, and 打开看懂, 设置… and 退出看懂. The lines that only tell are disabled.
func Menu(m locales.Messages, s Status, act Actions) []ns.MenuItem {
	items := []ns.MenuItem{{Label: state(m, s), Disabled: true}}
	if j := s.Running; j != nil {
		items = append(items, ns.MenuItem{Label: m.TrayJob(short(j.Title), StageName(m, *j), s.percent()), Disabled: true})
	}
	items = append(items,
		ns.MenuItem{Label: m.TrayQueue(s.Waiting), Disabled: true},
		ns.MenuItem{Separator: true},
		ns.MenuItem{Label: m.QueuePause(), Checkable: true, Checked: s.Paused, OnSelect: act.Pause},
		ns.MenuItem{Separator: true},
		ns.MenuItem{Label: m.TrayOpen(), OnSelect: act.Open},
		ns.MenuItem{Label: m.AppSettings(), OnSelect: act.Settings},
		ns.MenuItem{Label: m.MenuQuit(), OnSelect: act.Quit},
	)
	return items
}
