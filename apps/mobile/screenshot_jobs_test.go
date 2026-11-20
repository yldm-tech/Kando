package main

import (
	"net/http"
	"testing"

	"kando/apps/mobile/store"
	"kando/packages/sync"
	"kando/packages/sync/synctest"
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
	p.openTab(jobsTab, msgs.JobsTitle())
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

// TestScreenshotsJobsAway renders the Jobs page away from the computer's network, following its queue through the storage relay (jobs-relay), with a link left in the storage that the computer, gone to sleep, has not taken yet (jobs-relay-off), and once the storage no longer takes the computer's key (jobs-relay-storage).
func TestScreenshotsJobsAway(t *testing.T) {
	shot := screenshots(t)
	d := newDesktop(t)
	bucket := synctest.NewBucket(t, now)
	d.StartRelay(t, bucket, now, relayTurn)
	moved(d, "job1", "https://youtu.be/dQw4w9WgXcQ", sync.JobRunning, 1, 0.64)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	p.app.WaitUntil(wait, func() bool { return d.RelayJoins() == 1 })
	p.openJobs()
	d.SetOff(true)
	p.app.WaitUntil(reconnect, func() bool { return p.reach(id) == msgs.JobsViaStorage() })
	shot(p, "jobs-relay")
	d.StopRelay()
	p.app.Press("id:jobs-add")
	p.app.WaitFor("id:add-link", wait)
	p.app.Type("id:add-link", "https://www.bilibili.com/video/BV1GJ411x7h7")
	p.app.Press("id:add-send")
	p.app.WaitUntil(wait, func() bool { links, _ := store.Open(dir).Links(); return len(links) == 1 && links[0].Relayed })
	shot(p, "jobs-relay-off")
	bucket.Refuse(http.StatusForbidden, "SignatureDoesNotMatch")
	p.app.WaitUntil(wait, func() bool { return p.texts("id:jobs-summary-"+id) == msgs.JobsStorageKeys("Studio") })
	shot(p, "jobs-relay-storage")
}
