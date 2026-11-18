package main

import (
	"strings"
	"testing"
	"time"

	"kando/apps/mobile/store"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// reconnect is how long a step waits for the phone to find a computer it could not reach answering again: its tries back off to a few seconds apart.
const reconnect = 30 * time.Second

// sendLink sends link from the Add sheet the Library's + opens, typed into its field.
func (p *phone) sendLink(link string) {
	p.t.Helper()
	p.app.WaitFor("id:films-add", wait)
	p.app.Press("id:films-add")
	p.app.WaitFor("id:add-link", wait)
	p.app.Type("id:add-link", link)
	p.app.Press("id:add-send")
	p.app.WaitFor("id:jobs-page", wait)
}

// moved has d move job id along: its state, the step at, of the three, running with progress, those before it done.
func moved(d interface{ SetJob(sync.Job) }, id, link, state string, at int, progress float64) sync.Job {
	job := sync.Job{ID: id, Link: link, Title: "Morning Talk", Library: "lib", Device: "Test Phone", State: state}
	for i, name := range []string{sync.StepDownload, sync.StepTranscribe, sync.StepTranslate} {
		st := sync.Step{Name: name, State: sync.StepWaiting}
		switch {
		case i < at:
			st.State, st.Progress = sync.StepDone, 1
		case i == at:
			st.State, st.Progress = sync.StepRunning, progress
		}
		job.Steps = append(job.Steps, st)
	}
	d.SetJob(job)
	return job
}

// A link pasted into the Add sheet goes to the computer, which queues it with the phone's name; the Jobs page follows it step by step as the computer moves it along, has the computer update its downloader when it is too old for the site, and once it is done the banner says so and the job opens its film.
func TestALinkSentFromThePhoneIsFollowedToItsFilm(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.app.WaitFor("id:films-add", wait)
	p.app.Press("id:films-add")
	p.app.WaitFor("id:add-link", wait)
	p.app.Clipboard = "Look at this https://youtu.be/dQw4w9WgXcQ"
	p.app.Press("id:add-paste")
	if got := p.app.WaitFor("id:add-site", wait).Text; got != "youtube.com" {
		t.Fatalf("the sheet names the site %q, want youtube.com", got)
	}
	p.app.Press("id:add-send")
	p.app.WaitFor("id:jobs-page", wait)
	p.app.WaitFor("id:jobs-job-job1", wait)
	if jobs := d.Jobs(); len(jobs) != 1 || jobs[0].Link != "https://youtu.be/dQw4w9WgXcQ" || jobs[0].Device != "Test Phone" {
		t.Fatalf("the computer's queue is %+v", jobs)
	}
	p.app.WaitUntil(wait, func() bool { links, _ := store.Open(dir).Links(); return len(links) == 0 })

	moved(d, "job1", "https://youtu.be/dQw4w9WgXcQ", sync.JobRunning, 0, 0.42)
	p.app.WaitUntil(wait, func() bool {
		return p.app.Exists("id:jobs-step-detail-job1-download") && p.app.Find("id:jobs-step-detail-job1-download").Text == msgs.JobsPercent(42)
	})
	if got, want := p.app.Find("id:jobs-status-job1").Text, msgs.JobsFrom(msgs.JobsStepPercent(msgs.JobsStepDownload(), 42), "Test Phone"); got != want {
		t.Fatalf("the job says %q, want %q", got, want)
	}

	job := moved(d, "job1", "https://youtu.be/dQw4w9WgXcQ", sync.JobFailed, 3, 0)
	job.Steps[0].State, job.Failure = sync.StepFailed, &sync.Failure{Step: sync.StepDownload, Code: sync.FailOutdated, Message: "yt-dlp is too old", Remedy: sync.RemedyUpdateYTDLP}
	d.SetJob(job)
	p.app.WaitFor("id:jobs-update-job1", wait)
	if got := p.app.Find("id:jobs-failure-job1").Text; got != msgs.JobsFailOutdated() {
		t.Fatalf("the failure says %q, want %q", got, msgs.JobsFailOutdated())
	}
	p.app.Press("id:jobs-update-job1")
	p.app.WaitUntil(wait, func() bool { return d.YTDLPUpdates() == 1 })

	done := moved(d, "job1", "https://youtu.be/dQw4w9WgXcQ", sync.JobDone, 3, 0)
	done.Film = "f1"
	d.SetJob(done)
	p.app.WaitFor("id:job-done-job1", wait)
	if got := p.texts("id:job-done-job1"); !strings.Contains(got, msgs.JobsDoneTitle("Studio")) {
		t.Fatalf("the banner says %q, want %q", got, msgs.JobsDoneTitle("Studio"))
	}
	p.app.Press("id:jobs-open-job1")
	p.app.WaitFor("id:film-page", wait)
	if got := p.app.Find("id:film-title").Text; got != "Morning Talk" {
		t.Fatalf("the job opened %q", got)
	}
}

// A link sent while the computer is off waits on the phone, kept on its disk, and goes once the computer answers again; the computer takes it once, though it is sent again.
func TestALinkSentWhileTheComputerIsOffGoesOnceItIsBack(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	d.SetOff(true)
	p := startPhone(t, dir, secrets)
	p.sendLink("https://youtu.be/dQw4w9WgXcQ")
	id := store.DesktopID(d.Fingerprint)
	p.app.WaitUntil(wait, func() bool { return p.app.Find("id:jobs-summary-"+id).Text == msgs.JobsOfflineSummary() })
	var links []store.Link
	p.app.WaitUntil(wait, func() bool { links, _ = store.Open(dir).Links(); return len(links) == 1 })
	p.app.WaitFor("id:jobs-link-"+links[0].ID, wait)
	if got := p.app.Find("id:jobs-link-status-" + links[0].ID).Text; got != msgs.JobsWaitingFor("Studio") {
		t.Fatalf("the link waiting says %q", got)
	}

	d.SetOff(false)
	p.app.WaitFor("id:jobs-job-job1", reconnect)
	p.app.WaitForGone("id:jobs-link-"+links[0].ID, wait)
	p.app.WaitUntil(wait, func() bool { links, _ := store.Open(dir).Links(); return len(links) == 0 })
	if jobs := d.Jobs(); len(jobs) != 1 {
		t.Fatalf("the computer made %d jobs of the link", len(jobs))
	}
	p.app.WaitUntil(wait, func() bool { return p.app.Find("id:jobs-summary-"+id).Text == msgs.JobsActive(1) })
}

// A computer that does not take links from phones refuses the link, which stays with why until it is removed; one that asks its user first takes it without a job, and the phone says so.
func TestARefusedLinkSaysWhyAndAnAskedOneIsPutBeforeTheUser(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	d.SetReceive(sync.ReceiveOff)
	p.sendLink("https://youtu.be/dQw4w9WgXcQ")
	var links []store.Link
	p.app.WaitUntil(wait, func() bool { links, _ = store.Open(dir).Links(); return len(links) == 1 })
	status := "id:jobs-link-status-" + links[0].ID
	p.app.WaitUntil(wait, func() bool { return p.app.Exists(status) && p.app.Find(status).Text == msgs.JobsLinksOff("Studio") })
	p.app.Press("id:jobs-link-forget-" + links[0].ID)
	p.app.WaitForGone("id:jobs-link-"+links[0].ID, wait)

	d.SetReceive(sync.ReceiveAsk)
	p.app.Press("id:jobs-add")
	p.app.WaitFor("id:add-link", wait)
	p.app.Type("id:add-link", "https://youtu.be/dQw4w9WgXcQ")
	p.app.Press("id:add-send")
	p.app.WaitUntil(wait, func() bool { return len(d.Pending()) == 1 })
	p.app.WaitUntil(wait, func() bool { return strings.Contains(p.texts("id:toasts"), msgs.JobsSentPending("Studio")) })
	if len(d.Jobs()) != 0 {
		t.Fatalf("a computer that asks first made jobs %+v", d.Jobs())
	}
}

// A job is canceled on the computer only once the user confirms it, and a canceled job can be started again; pausing is offered while the computer answers it.
func TestAJobIsCanceledAfterAskingAndStartedAgain(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	moved(d, "job1", "https://youtu.be/dQw4w9WgXcQ", sync.JobRunning, 1, -1)
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.app.WaitFor("id:films-jobs", wait)
	p.app.Press("id:films-jobs")
	p.app.WaitFor("id:jobs-job-job1", wait)
	p.app.WaitFor("id:jobs-pause-job1", wait)
	p.app.Press("id:jobs-pause-job1")
	p.app.WaitFor("id:jobs-resume-job1", wait)
	p.app.Press("id:jobs-resume-job1")
	p.app.WaitUntil(wait, func() bool { return d.Jobs()[0].State == sync.JobWaiting })

	p.app.WaitFor("id:jobs-cancel-job1", wait)
	p.app.Press("id:jobs-cancel-job1")
	p.app.Answer(0)
	if d.Jobs()[0].State == sync.JobCanceled {
		t.Fatal("a job was canceled though the user kept it")
	}
	p.app.Press("id:jobs-cancel-job1")
	p.app.Answer(1)
	p.app.WaitFor("id:jobs-retry-job1", wait)
	if d.Jobs()[0].State != sync.JobCanceled {
		t.Fatalf("the job is %s on the computer after canceling", d.Jobs()[0].State)
	}
	p.app.Press("id:jobs-retry-job1")
	p.app.WaitUntil(wait, func() bool { return d.Jobs()[0].State == sync.JobWaiting })
	p.app.WaitForGone("id:jobs-retry-job1", wait)
}

// On a tablet the Jobs page is a section of the sidebar rather than a page the Library opens.
func TestATabletShowsJobsInItsSidebar(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("the idiom is the platform's on a target")
	}
	d := newDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir), nsgotest.Size(1180, 820), nsgotest.Environment(core.Environment{Locale: lang, Idiom: core.IdiomTablet}))
	p.app.WaitFor("id:films-add", wait)
	if p.app.Exists("id:films-jobs") {
		t.Fatal("a tablet's Library opens the Jobs page, which its sidebar shows")
	}
	p.app.Press("id:sidebar-tab-jobs")
	p.app.WaitFor("id:jobs-page", wait)
	p.app.WaitUntil(wait, func() bool {
		return p.app.Find("id:jobs-summary-"+store.DesktopID(d.Fingerprint)).Text == msgs.JobsIdle()
	})
	p.app.Press("id:jobs-add")
	p.app.WaitFor("id:add-link", wait)
}
