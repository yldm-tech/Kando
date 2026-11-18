package main

import (
	"testing"

	"kando/packages/sync"
)

func TestScreenshotsJobs(t *testing.T) {
	shot := screenshots(t)
	d := newDesktop(t)
	d.SetOptions(sync.OptionQuality, sync.OptionSubtitles)
	moved(d, "job1", "https://youtu.be/dQw4w9WgXcQ", sync.JobRunning, 1, 0.64)
	waiting := moved(d, "job2", "https://www.bilibili.com/video/BV1GJ411x7h7", sync.JobWaiting, -1, 0)
	waiting.Title, waiting.Device = "How Rivers Shape Landscapes", ""
	d.SetJob(waiting)
	failed := moved(d, "job3", "https://youtu.be/ScMzIvxBSi4", sync.JobFailed, 3, 0)
	failed.Title = "Street Food in Osaka"
	failed.Steps[0].State, failed.Failure = sync.StepFailed, &sync.Failure{Step: sync.StepDownload, Code: sync.FailOutdated, Message: "yt-dlp is too old", Remedy: sync.RemedyUpdateYTDLP}
	d.SetJob(failed)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.app.WaitFor("id:films-jobs", wait)
	p.app.Press("id:films-jobs")
	p.app.WaitFor("id:jobs-update-job3", wait)
	shot(p, "jobs")
	p.app.Press("id:jobs-add")
	p.app.WaitFor("id:add-link", wait)
	p.app.Type("id:add-link", "https://youtu.be/dQw4w9WgXcQ")
	p.app.WaitFor("id:add-site", wait)
	p.app.WaitFor("id:add-quality-best", wait)
	p.app.Press("id:add-quality-best")
	shot(p, "jobs-add")
}
