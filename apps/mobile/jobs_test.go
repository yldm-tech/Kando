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

// A link whose call drops after the computer took it, while the computer keeps answering, goes again under the same call ID rather than waiting until the computer drops off; the computer makes one job of it.
func TestALinkWhoseCallDroppedGoesAgainWhileTheComputerAnswers(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	p.app.WaitFor("id:films-jobs", wait)
	p.app.Press("id:films-jobs")
	p.app.WaitUntil(wait, func() bool { return p.app.Find("id:jobs-summary-"+id).Text == msgs.JobsIdle() })
	d.DropCreates(1)
	p.app.Press("id:jobs-add")
	p.app.WaitFor("id:add-link", wait)
	p.app.Type("id:add-link", "https://youtu.be/dQw4w9WgXcQ")
	p.app.Press("id:add-send")
	p.app.WaitFor("id:jobs-job-job1", reconnect)
	p.app.WaitUntil(wait, func() bool { links, _ := store.Open(dir).Links(); return len(links) == 0 })
	if jobs := d.Jobs(); len(jobs) != 1 {
		t.Fatalf("the computer made %d jobs of the link", len(jobs))
	}
}

// The links kept on the phone outlive the app: one waiting for a computer that is off goes once it answers after the app started again, and one the computer refused still says why and is not sent again on its own.
func TestLinksWaitAndKeepTheirRefusalsAcrossARestart(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	var refused, waiting string
	t.Run("before", func(t *testing.T) {
		p := startPhone(t, dir, secrets)
		d.SetReceive(sync.ReceiveOff)
		p.sendLink("https://youtu.be/refused0001")
		var links []store.Link
		p.app.WaitUntil(wait, func() bool { links, _ = store.Open(dir).Links(); return len(links) == 1 && links[0].Refusal != nil })
		refused = links[0].ID
		d.SetOff(true)
		p.app.Press("id:jobs-add")
		p.app.WaitFor("id:add-link", wait)
		p.app.Type("id:add-link", "https://youtu.be/dQw4w9WgXcQ")
		p.app.Press("id:add-send")
		p.app.WaitUntil(wait, func() bool { links, _ = store.Open(dir).Links(); return len(links) == 2 })
		waiting = links[1].ID
	})

	p := startPhone(t, dir, secrets)
	p.app.WaitFor("id:films-jobs", wait)
	p.app.Press("id:films-jobs")
	p.app.WaitFor("id:jobs-link-"+waiting, wait)
	if got := p.app.Find("id:jobs-link-status-" + refused).Text; got != msgs.JobsLinksOff("Studio") {
		t.Fatalf("the refused link says %q after a restart, want %q", got, msgs.JobsLinksOff("Studio"))
	}
	if got := p.app.Find("id:jobs-link-status-" + waiting).Text; got != msgs.JobsWaitingFor("Studio") {
		t.Fatalf("the waiting link says %q after a restart", got)
	}
	p.app.WaitFor("id:jobs-link-resend-"+refused, wait)

	d.SetReceive(sync.ReceiveQueue)
	d.SetOff(false)
	p.app.WaitFor("id:jobs-job-job1", reconnect)
	p.app.WaitForGone("id:jobs-link-"+waiting, wait)
	if jobs := d.Jobs(); len(jobs) != 1 || jobs[0].Link != "https://youtu.be/dQw4w9WgXcQ" {
		t.Fatalf("the computer's queue is %+v", jobs)
	}
	if got := p.app.Find("id:jobs-link-status-" + refused).Text; got != msgs.JobsLinksOff("Studio") {
		t.Fatalf("the refused link says %q once the computer is back", got)
	}
}

// A computer that speaks protocol v1 alone takes no links: the ones waiting for it say so rather than waiting for ever, and the Add sheet does not send it more.
func TestAnOlderComputerRefusesTheLinksWaitingForIt(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	d.SetOff(true)
	p := startPhone(t, dir, secrets)
	p.sendLink("https://youtu.be/dQw4w9WgXcQ")
	var links []store.Link
	p.app.WaitUntil(wait, func() bool { links, _ = store.Open(dir).Links(); return len(links) == 1 })
	d.SetOlder(true)
	d.SetOff(false)
	id := store.DesktopID(d.Fingerprint)
	status := "id:jobs-link-status-" + links[0].ID
	p.app.WaitUntil(reconnect, func() bool { return p.app.Exists(status) && p.app.Find(status).Text == msgs.JobsOlder() })
	if got := p.app.Find("id:jobs-summary-" + id).Text; got != msgs.JobsOlder() {
		t.Fatalf("the computer's card says %q, want %q", got, msgs.JobsOlder())
	}
	p.app.WaitUntil(wait, func() bool { kept, _ := store.Open(dir).Links(); return len(kept) == 1 && kept[0].Refusal != nil })

	p.app.Press("id:jobs-add")
	p.app.WaitFor("id:add-link", wait)
	p.app.Type("id:add-link", "https://youtu.be/dQw4w9WgXcQ")
	if p.app.Enabled("id:add-send") {
		t.Fatal("the Add sheet sends links to a computer too old to take them")
	}
	if got := p.app.Find("id:add-note").Text; got != msgs.JobsOlder() {
		t.Fatalf("the Add sheet says %q, want %q", got, msgs.JobsOlder())
	}
}

// A computer that revoked the phone says so, its links waiting meanwhile; unpaired in Settings, it takes its waiting links from the phone.
func TestARevokedComputerSaysSoAndUnpairingDropsItsLinks(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	d.SetOff(true)
	p := startPhone(t, dir, secrets)
	p.sendLink("https://youtu.be/dQw4w9WgXcQ")
	p.app.WaitUntil(wait, func() bool { links, _ := store.Open(dir).Links(); return len(links) == 1 })
	d.RevokeAll()
	d.SetOff(false)
	id := store.DesktopID(d.Fingerprint)
	p.app.WaitUntil(reconnect, func() bool { return p.app.Find("id:jobs-summary-"+id).Text == msgs.ComputersUnpaired("Studio") })
	if links, _ := store.Open(dir).Links(); len(links) != 1 || links[0].Refusal != nil {
		t.Fatalf("a revoked computer's link is %+v, want it waiting", links)
	}

	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.Press("id:settings-computer-options-" + id)
	p.app.Answer(1)
	p.app.WaitForGone("id:settings-computer-"+id, wait)
	p.app.WaitUntil(wait, func() bool { links, _ := store.Open(dir).Links(); return len(links) == 0 })
}

// A done job's Open waits for its film to be found on the computer, taking no second press meanwhile, and says so when the computer no longer has it.
func TestOpeningADoneJobWaitsForItsFilmAndSaysWhenItIsGone(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	done := moved(d, "job1", "https://youtu.be/dQw4w9WgXcQ", sync.JobDone, 3, 0)
	done.Film = "f1"
	d.SetJob(done)
	gone := moved(d, "job2", "https://youtu.be/gone0000001", sync.JobDone, 3, 0)
	gone.Film = "removed"
	d.SetJob(gone)
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.app.WaitFor("id:films-jobs", wait)
	p.app.Press("id:films-jobs")
	p.app.WaitFor("id:jobs-open-job1", wait)

	release := d.Hold()
	p.app.Press("id:jobs-open-job1")
	p.app.WaitUntil(wait, func() bool { return !p.app.Enabled("id:jobs-open-job1") })
	release()
	p.app.WaitFor("id:film-page", wait)
	p.app.HeaderBack("id:film-page")
	p.app.WaitForGone("id:film-page", wait)

	p.app.WaitFor("id:jobs-open-job2", wait)
	p.app.Press("id:jobs-open-job2")
	p.app.WaitUntil(wait, func() bool { return strings.Contains(p.texts("id:toasts"), msgs.JobsFilmGone("Studio")) })
	if p.app.Exists("id:film-page") {
		t.Fatal("a job whose film is gone opened a page")
	}
}

// An app that ends while a call to a computer is under way ends at once rather than waiting for the computer to answer or the call to time out.
func TestTheAppEndsWithoutWaitingForACallUnderWay(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	done := moved(d, "job1", "https://youtu.be/dQw4w9WgXcQ", sync.JobDone, 3, 0)
	done.Film = "f1"
	d.SetJob(done)
	secrets := pairedWith(t, d, dir)
	release := d.Hold()
	defer release()
	var ended time.Time
	t.Run("app", func(t *testing.T) {
		p := startPhone(t, dir, secrets)
		p.app.WaitFor("id:films-jobs", wait)
		p.app.Press("id:films-jobs")
		p.app.WaitFor("id:jobs-open-job1", wait)
		p.app.Press("id:jobs-open-job1")
		p.app.WaitUntil(wait, func() bool { return !p.app.Enabled("id:jobs-open-job1") })
		t.Cleanup(func() { ended = time.Now() })
	})
	if took := time.Since(ended); took > 5*time.Second {
		t.Fatalf("the app took %v to end with a call under way", took)
	}
}
