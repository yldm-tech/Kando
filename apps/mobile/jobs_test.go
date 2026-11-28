package main

import (
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kando/apps/mobile/films"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	"kando/apps/mobile/theme"
	"kando/packages/sync"
	"kando/packages/sync/synctest"

	"github.com/nsgo-dev/nsgo/base"
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
	if got := p.app.WaitFor("id:add-site", wait).Text; got != msgs.FilmsSourceYoutube() {
		t.Fatalf("the sheet names the site %q, want %q", got, msgs.FilmsSourceYoutube())
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
	// The job sent from this phone names its site alone; how far it is shows over its picture, the share of its three steps done.
	if got, want := p.app.Find("id:jobs-status-job1").Text, msgs.FilmsSourceYoutube(); got != want {
		t.Fatalf("the job says %q, want %q", got, want)
	}
	if got, want := p.texts("id:jobs-job-job1"), msgs.JobsPercent(14); !strings.Contains(got, want) {
		t.Fatalf("the job's card says %q, want %q over its picture", got, want)
	}

	job := moved(d, "job1", "https://youtu.be/dQw4w9WgXcQ", sync.JobFailed, 3, 0)
	job.Steps[0].State, job.Failure = sync.StepFailed, &sync.Failure{Step: sync.StepDownload, Code: sync.FailOutdated, Message: "yt-dlp is too old", Remedy: sync.RemedyUpdateYTDLP}
	d.SetJob(job)
	p.app.WaitFor("id:jobs-update-job1", wait)
	if got, want := p.app.Find("id:jobs-failure-job1").Text, msgs.JobsFailOutdatedSite(msgs.FilmsSourceYoutube()); got != want {
		t.Fatalf("the failure says %q, want %q", got, want)
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
	// The banner is frosted as the bars are.
	if banner := p.app.Find("id:job-done-job1"); banner.Style.CornerRadius != look.CardRadius || banner.Style.Effects == nil || len(banner.Style.Effects.BackdropFilters) != len(theme.BarBlur) {
		t.Fatalf("the banner is %v round over %+v, want %v round over the bars' blur", banner.Style.CornerRadius, banner.Style.Effects, look.CardRadius)
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
	p.app.WaitUntil(wait, func() bool { return p.app.Find("id:jobs-summary-"+id).Text == msgs.JobsLockPhone(msgs.JobsActive(1)) })
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
	p.app.WaitUntil(wait, func() bool {
		return p.app.Exists("id:toasts") && strings.Contains(p.texts("id:toasts"), msgs.JobsSentPending("Studio"))
	})
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
	p.openTab(jobsTab, msgs.JobsTitle())
	p.app.WaitFor("id:jobs-job-job1", wait)
	p.app.WaitUntil(wait, func() bool { return p.app.Exists("id:jobs-pause-job1") && p.app.Enabled("id:jobs-pause-job1") })
	p.app.Press("id:jobs-pause-job1")
	// The paused job's event can reach the phone before the computer's reply to Pause, so Resume shows while the row is still busy and takes its press only once the reply arrived.
	p.app.WaitUntil(wait, func() bool { return p.app.Exists("id:jobs-resume-job1") && p.app.Enabled("id:jobs-resume-job1") })
	p.app.Press("id:jobs-resume-job1")
	p.app.WaitUntil(wait, func() bool { return d.Jobs()[0].State == sync.JobWaiting })

	// The computer resumes the job before its reply reaches the phone, which keeps the row's buttons disabled until then.
	p.app.WaitUntil(wait, func() bool { return p.app.Exists("id:jobs-cancel-job1") && p.app.Enabled("id:jobs-cancel-job1") })
	p.app.Press("id:jobs-cancel-job1")
	p.app.Answer(0)
	if d.Jobs()[0].State == sync.JobCanceled {
		t.Fatal("a job was canceled though the user kept it")
	}
	p.app.Press("id:jobs-cancel-job1")
	p.app.Answer(1)
	// The row shows Retry once the job is canceled, and takes its press once the computer's reply to the cancel reached the phone.
	p.app.WaitUntil(wait, func() bool { return p.app.Exists("id:jobs-retry-job1") && p.app.Enabled("id:jobs-retry-job1") })
	if d.Jobs()[0].State != sync.JobCanceled {
		t.Fatalf("the job is %s on the computer after canceling", d.Jobs()[0].State)
	}
	p.app.Press("id:jobs-retry-job1")
	p.app.WaitUntil(wait, func() bool { return d.Jobs()[0].State == sync.JobWaiting })
	p.app.WaitForGone("id:jobs-retry-job1", wait)
}

// A failed job offers Remove beside Try Again, which takes it off the computer's queue and off the phone, as a canceled one does; a job going on does not, nor does any on a computer from before the operation.
func TestAStoppedJobIsRemovedFromThePhone(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	moved(d, "job1", "https://youtu.be/dQw4w9WgXcQ", sync.JobRunning, 1, 0.5)
	failed := moved(d, "job2", "https://youtu.be/M7lc1UVf-VE", sync.JobFailed, 3, 0)
	failed.Failure = &sync.Failure{Step: sync.StepDownload, Code: sync.FailNetwork, Remedy: sync.RemedyRetry}
	d.SetJob(failed)
	moved(d, "job3", "https://youtu.be/ScMzIvxBSi4", sync.JobCanceled, 1, 0)
	secrets := pairedWith(t, d, dir)

	t.Run("older computer", func(t *testing.T) {
		d.SetNoJobRemove(true)
		t.Cleanup(func() { d.SetNoJobRemove(false) })
		p := startPhone(t, dir, secrets)
		p.openTab(jobsTab, msgs.JobsTitle())
		p.app.WaitFor("id:jobs-retry-job2", wait)
		p.app.WaitFor("id:jobs-retry-job3", wait)
		for _, button := range []string{"id:jobs-remove-job2", "id:jobs-remove-job3"} {
			if p.app.Exists(button) {
				t.Fatalf("a computer that does not remove jobs is offered %s", button)
			}
		}
	})

	p := startPhone(t, dir, secrets)
	p.openTab(jobsTab, msgs.JobsTitle())
	p.app.WaitFor("id:jobs-retry-job2", wait)
	p.app.WaitUntil(wait, func() bool { return p.app.Exists("id:jobs-remove-job2") && p.app.Enabled("id:jobs-remove-job2") })
	if got := p.texts("id:jobs-remove-job2"); got != msgs.JobsRemove() {
		t.Fatalf("the failed job's button reads %q, want %q", got, msgs.JobsRemove())
	}
	if p.app.Exists("id:jobs-remove-job1") {
		t.Fatal("a running job is offered Remove")
	}
	p.app.Press("id:jobs-remove-job2")
	p.app.WaitForGone("id:jobs-job-job2", wait)
	gone := func(id string) bool {
		return !slices.ContainsFunc(d.Jobs(), func(j sync.Job) bool { return j.ID == id })
	}
	if !gone("job2") {
		t.Fatalf("the computer still has the removed job: %+v", d.Jobs())
	}
	p.app.WaitUntil(wait, func() bool { return p.app.Exists("id:jobs-remove-job3") && p.app.Enabled("id:jobs-remove-job3") })
	p.app.Press("id:jobs-remove-job3")
	p.app.WaitForGone("id:jobs-job-job3", wait)
	if !gone("job3") || gone("job1") {
		t.Fatalf("the computer's jobs after removing the canceled one: %+v", d.Jobs())
	}
	p.app.WaitFor("id:jobs-job-job1", wait)
}

// On a tablet Jobs is a section of the sidebar, as it is a tab of a phone's bar, with no button of its own on the Library.
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
	if p.app.Exists("id:sidebar-badge-tab-jobs") {
		t.Fatal("the sidebar counts jobs with nothing to do")
	}
	moved(d, "job1", "https://youtu.be/dQw4w9WgXcQ", sync.JobRunning, 0, 0.1)
	p.app.WaitUntil(wait, func() bool {
		return p.app.Exists("id:sidebar-badge-tab-jobs") && p.texts("id:sidebar-badge-tab-jobs") == "1"
	})
	// Out of reach, the sidebar's computers card says so and how many jobs wait, as the design's does.
	d.SetOff(true)
	p.app.WaitUntil(reconnect, func() bool { return p.texts("id:sidebar-computers-status") == msgs.JobsOfflineWaiting(1) })
	d.SetOff(false)
	// A tablet's Add sheet is a card in the middle of the screen, as the Pad artboards draw it, not a sheet across its width.
	p.app.Press("id:jobs-add")
	p.app.WaitFor("id:add-link", wait)
	field := p.app.Find("id:add-link").Bounds()
	if field.Size.Width > 560 || field.Origin.X < 300 {
		t.Fatalf("the Add sheet's field is at %+v on a 1180pt-wide tablet, want it in a card in the middle", field)
	}
}

// With two computers, a tablet's sidebar says the computers are offline only once neither answers: while one does, the card counts its running and failed jobs, not the other's as waiting.
func TestATabletSidebarSaysOfflineOnlyWhenNoComputerAnswers(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("the idiom is the platform's on a target")
	}
	studio := newDesktop(t)
	macbook := synctest.NewOwn(t, "MacBook")
	macbook.Offer("123456")
	moved(macbook, "job1", "https://youtu.be/dQw4w9WgXcQ", sync.JobRunning, 0, 0.1)
	failed := moved(macbook, "job2", "https://youtu.be/ScMzIvxBSi4", sync.JobFailed, 3, 0)
	failed.Steps[0].State, failed.Failure = sync.StepFailed, &sync.Failure{Step: sync.StepDownload, Code: sync.FailNetwork, Remedy: sync.RemedyRetry}
	macbook.SetJob(failed)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWithAll(t, dir, studio, macbook), nsgotest.Size(1180, 820), nsgotest.Environment(core.Environment{Locale: lang, Idiom: core.IdiomTablet}))
	p.app.WaitFor("id:sidebar-tab-jobs", wait)
	p.app.Press("id:sidebar-tab-jobs")
	p.app.WaitUntil(wait, func() bool { return p.texts("id:sidebar-computers-status") == msgs.JobsActiveFailed(1, 1) })
	studio.SetOff(true)
	p.app.WaitUntil(reconnect, func() bool { return p.reach(store.DesktopID(studio.Fingerprint)) == msgs.JobsOffline() })
	if got, want := p.texts("id:sidebar-computers-status"), msgs.JobsActiveFailed(1, 1); got != want {
		t.Fatalf("with Studio offline and the MacBook running its jobs, the sidebar says %q, want %q", got, want)
	}
	macbook.SetOff(true)
	p.app.WaitUntil(reconnect, func() bool { return p.texts("id:sidebar-computers-status") == msgs.JobsOfflineWaiting(1) })
	studio.SetOff(false)
	macbook.SetOff(false)
}

// A link whose call drops after the computer took it, while the computer keeps answering, goes again under the same call ID rather than waiting until the computer drops off; the computer makes one job of it.
func TestALinkWhoseCallDroppedGoesAgainWhileTheComputerAnswers(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	p.openTab(jobsTab, msgs.JobsTitle())
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
	p.openTab(jobsTab, msgs.JobsTitle())
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
	summary := "id:jobs-summary-" + id
	p.app.WaitUntil(wait, func() bool { return p.app.Exists(summary) && p.app.Find(summary).Text == msgs.JobsOlder() })
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
	// The learning state syncs as the phone starts: one that reached the computer after it revoked the phone would show the page that says so over every tab, so it is to have found the computer asleep first.
	p.openTab(learnTab, msgs.LearnTitle())
	p.app.WaitFor("id:learn-offline", wait)
	p.openTab(filmsTab, msgs.FilmsTitle())
	p.sendLink("https://youtu.be/dQw4w9WgXcQ")
	p.app.WaitUntil(wait, func() bool { links, _ := store.Open(dir).Links(); return len(links) == 1 })
	d.RevokeAll()
	d.SetOff(false)
	id := store.DesktopID(d.Fingerprint)
	p.app.WaitUntil(reconnect, func() bool { return p.app.Find("id:jobs-summary-"+id).Text == msgs.ComputersUnpaired("Studio") })
	if links, _ := store.Open(dir).Links(); len(links) != 1 || links[0].Refusal != nil {
		t.Fatalf("a revoked computer's link is %+v, want it waiting", links)
	}

	// Opening the computer's page syncs with it, which finds the phone revoked: the page that says so covers the tabs, and unpairs it.
	p.openComputers()
	p.app.WaitFor("id:revoked-page", wait)
	p.app.Press("id:revoked-unpair")
	p.app.Answer(1)
	p.app.WaitForGone("id:revoked-page", wait)
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
	p.openTab(jobsTab, msgs.JobsTitle())
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
	p.app.WaitUntil(wait, func() bool {
		return p.app.Exists("id:toasts") && strings.Contains(p.texts("id:toasts"), msgs.JobsFilmGone("Studio"))
	})
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
		p.openTab(jobsTab, msgs.JobsTitle())
		p.app.WaitFor("id:jobs-open-job1", wait)
		p.app.Press("id:jobs-open-job1")
		p.app.WaitUntil(wait, func() bool { return !p.app.Enabled("id:jobs-open-job1") })
		t.Cleanup(func() { ended = time.Now() })
	})
	if took := time.Since(ended); took > 5*time.Second {
		t.Fatalf("the app took %v to end with a call under way", took)
	}
}

// A computer that takes job options has the Add sheet offer them, and the link goes with the picture and the source of original lines picked; the computer's own setting sends none. One that takes none is offered none.
func TestTheAddSheetSendsTheOptionsTheComputerTakes(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	secrets := pairedWith(t, d, dir)
	t.Run("none", func(t *testing.T) {
		p := startPhone(t, dir, secrets)
		p.openTab(jobsTab, msgs.JobsTitle())
		p.app.WaitUntil(wait, func() bool {
			return p.app.Find("id:jobs-summary-"+store.DesktopID(d.Fingerprint)).Text == msgs.JobsIdle()
		})
		p.app.Press("id:jobs-add")
		p.app.WaitFor("id:add-link", wait)
		if p.app.Exists("id:add-quality-best") || p.app.Exists("id:add-subtitles-platform") {
			t.Fatal("the Add sheet offers options to a computer that takes none")
		}
	})

	d.SetOptions(sync.OptionQuality, sync.OptionSubtitles)
	p := startPhone(t, dir, secrets)
	p.app.WaitFor("id:films-add", wait)
	p.app.Press("id:films-add")
	p.app.WaitFor("id:add-quality-720", wait)
	p.app.Press("id:add-quality-720")
	p.app.Press("id:add-subtitles-recognize")
	p.app.Type("id:add-link", "https://youtu.be/dQw4w9WgXcQ")
	p.app.Press("id:add-send")
	p.app.WaitFor("id:jobs-job-job1", wait)
	if asked := d.Asked(); len(asked) != 1 || asked[0] != (sync.JobOptions{Quality: sync.Quality720, Subtitles: sync.SubtitlesRecognize}) {
		t.Fatalf("the computer was asked %+v", asked)
	}

	p.app.Press("id:jobs-add")
	p.app.WaitFor("id:add-quality-computer", wait)
	p.app.Press("id:add-quality-computer")
	p.app.Press("id:add-subtitles-computer")
	p.app.Type("id:add-link", "https://youtu.be/M7lc1UVf-VE")
	p.app.Press("id:add-send")
	p.app.WaitFor("id:jobs-job-job2", wait)
	if asked := d.Asked(); len(asked) != 2 || asked[1] != (sync.JobOptions{}) {
		t.Fatalf("the computer was asked %+v", asked)
	}
}

// A computer that made the phone read-only still shows its jobs, without the buttons that would change them, and says why; the Add sheet sends it nothing. One made read-only while the phone follows it refuses an action, and the phone drops its buttons.
func TestAReadOnlyPhoneFollowsJobsWithoutChangingThem(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	moved(d, "job1", "https://youtu.be/dQw4w9WgXcQ", sync.JobRunning, 1, 0.5)
	failed := moved(d, "job2", "https://youtu.be/M7lc1UVf-VE", sync.JobFailed, 3, 0)
	failed.Failure = &sync.Failure{Step: sync.StepDownload, Code: sync.FailNetwork, Remedy: sync.RemedyRetry}
	d.SetJob(failed)
	secrets := pairedWith(t, d, dir)
	id := store.DesktopID(d.Fingerprint)

	d.SetReadOnly(true)
	t.Run("read-only", func(t *testing.T) {
		p := startPhone(t, dir, secrets)
		p.openTab(jobsTab, msgs.JobsTitle())
		p.app.WaitFor("id:jobs-read-only-"+id, wait)
		if got := p.app.Find("id:jobs-read-only-" + id).Text; got != msgs.JobsReadOnly("Studio") {
			t.Fatalf("the page says %q, want %q", got, msgs.JobsReadOnly("Studio"))
		}
		p.app.WaitFor("id:jobs-failure-job2", wait)
		for _, button := range []string{"id:jobs-pause-job1", "id:jobs-cancel-job1", "id:jobs-retry-job2", "id:jobs-remove-job2"} {
			if p.app.Exists(button) {
				t.Fatalf("a read-only phone shows %s", button)
			}
		}
		p.app.Press("id:jobs-add")
		p.app.WaitFor("id:add-link", wait)
		p.app.Type("id:add-link", "https://youtu.be/abcdefghijk")
		if p.app.Enabled("id:add-send") {
			t.Fatal("a read-only phone sends links")
		}
		if got := p.app.Find("id:add-note").Text; got != msgs.JobsReadOnly("Studio") {
			t.Fatalf("the Add sheet says %q, want %q", got, msgs.JobsReadOnly("Studio"))
		}
	})

	d.SetReadOnly(false)
	p := startPhone(t, dir, secrets)
	p.openTab(jobsTab, msgs.JobsTitle())
	p.app.WaitFor("id:jobs-pause-job1", wait)
	if p.app.Exists("id:jobs-read-only-" + id) {
		t.Fatal("a phone that may change the jobs is told it is read-only")
	}
	d.SetReadOnly(true)
	p.app.Press("id:jobs-pause-job1")
	p.app.WaitUntil(wait, func() bool {
		return p.app.Exists("id:toasts") && strings.Contains(p.texts("id:toasts"), msgs.JobsReadOnly("Studio"))
	})
	p.app.WaitForGone("id:jobs-pause-job1", wait)
	p.app.WaitFor("id:jobs-read-only-"+id, wait)
	if jobs := d.Jobs(); jobs[0].State != sync.JobRunning {
		t.Fatalf("a read-only phone paused the job: %+v", jobs[0])
	}
}

// The tab bar's Jobs tab, the second, counts what is not done, the links waiting on the phone among it, in the badges' orange once something needs the user, and shows nothing once all is done. It covers the drawn tab bar, which the headless tests show: the system bar of an iPhone or an Android phone shows no badge, and the Library's header counts there instead (TestTheLibraryCountsJobsWhereTheTabBarIsTheSystems).
func TestTheJobsTabCountsWhatIsUnfinished(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.app.WaitFor(talkRow(store.DesktopID(d.Fingerprint)), wait)
	var names []string
	for _, tab := range p.app.QueryMounted("id:tab-bar >> role:tab") {
		names = append(names, tab.Props.Label)
	}
	if want := []string{msgs.FilmsTitle(), msgs.JobsTitle(), msgs.LearnTitle(), msgs.SettingsTitle()}; !slices.Equal(names, want) {
		t.Fatalf("the tab bar shows %q, want %q", names, want)
	}
	if p.app.Exists("id:films-jobs") {
		t.Fatal("the Library still has a Jobs button beside the Jobs tab")
	}
	if p.app.Exists("id:bar-badge-tab-jobs") {
		t.Fatal("the Jobs tab counts with nothing to do")
	}
	moved(d, "job1", "https://youtu.be/dQw4w9WgXcQ", sync.JobRunning, 0, 0.1)
	p.app.WaitUntil(wait, func() bool { return p.app.Exists("id:bar-badge-tab-jobs") && p.texts("id:bar-badge-tab-jobs") == "1" })
	if got := p.app.Find("id:bar-badge-tab-jobs").Style.Background; got != theme.BadgeRed {
		t.Errorf("the badge of a job running is %v, want the badges' red", got)
	}
	failed := moved(d, "job2", "https://youtu.be/M7lc1UVf-VE", sync.JobFailed, 3, 0)
	failed.Failure = &sync.Failure{Step: sync.StepDownload, Code: sync.FailNetwork, Remedy: sync.RemedyRetry}
	d.SetJob(failed)
	p.app.WaitUntil(wait, func() bool { return p.app.Exists("id:bar-badge-tab-jobs") && p.texts("id:bar-badge-tab-jobs") == "2" })
	if got := p.app.Find("id:bar-badge-tab-jobs").Style.Background; got != theme.BadgeOrange {
		t.Errorf("the badge of a failed job is %v, want the badges' orange", got)
	}

	for _, id := range []string{"job1", "job2"} {
		done := moved(d, id, "https://youtu.be/"+id, sync.JobDone, 3, 0)
		d.SetJob(done)
	}
	p.app.WaitForGone("id:bar-badge-tab-jobs", wait)
	p.openTab(jobsTab, msgs.JobsTitle())
	p.app.WaitFor("id:jobs-page", wait)
}

// TestTheLibraryCountsJobsWhereTheTabBarIsTheSystems mounts the Library as the app does where the tab bar is the system's (shell.SystemBar), which shows no badge: the page's header then has a Jobs button with the unfinished count, said in its label, that opens Jobs. The headless tests run simulated, where the app draws its own bar, so App does not take this path here and the page is mounted on its own.
func TestTheLibraryCountsJobsWhereTheTabBarIsTheSystems(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	opened := 0
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		theme.Apply(cx)
		s := session.New(cx, session.Config{Dir: dir, Device: "Test Phone", Platform: sync.PlatformIOS, Now: sessionNow, RelayEvery: relayEvery})
		nav := base.NewNav(cx, 0)
		return core.Show{When: s.Loaded, Then: func(cx *core.Scope) core.Node {
			return nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, _ int, _ func() int) base.NavPage {
				return films.Page(cx, s, films.Actions{Add: func() {}, Pair: func() {}, Jobs: func() { opened++ }})
			})
		}}
	}, nsgotest.Environment(core.Environment{Locale: lang}), pairedWith(t, d, dir))
	app.WaitFor("id:films-jobs", wait)
	if got := app.Find("id:films-jobs").Props.Label; got != msgs.JobsTitle() {
		t.Fatalf("the Jobs button with nothing to do is named %q, want %q", got, msgs.JobsTitle())
	}
	moved(d, "job1", "https://youtu.be/dQw4w9WgXcQ", sync.JobRunning, 0, 0.1)
	app.WaitUntil(wait, func() bool {
		return app.Find("id:films-jobs").Props.Label == msgs.JobsBadgeLabel(1) && (&phone{t: t, app: app}).texts("id:films-jobs-badge") == "1"
	})
	if got := app.Find("id:films-jobs-badge").Style.Background; got != theme.BadgeRed {
		t.Errorf("the badge of a job running is %v, want the badges' red", got)
	}
	app.Press("id:films-jobs")
	if opened != 1 {
		t.Fatalf("pressing Jobs opened it %d times, want once", opened)
	}
}

// A job that failed for a region the uploader restricted offers another link, which opens the Add sheet, rather than trying again; one whose sign-in to its site expired on the computer names the site and is tried again once the user signed in there.
func TestAFailedJobSaysWhatCanBeDoneAboutIt(t *testing.T) {
	d := newDesktop(t)
	region := moved(d, "job1", "https://youtu.be/ScMzIvxBSi4", sync.JobFailed, 3, 0)
	region.Steps[0].State, region.Failure = sync.StepFailed, &sync.Failure{Step: sync.StepDownload, Code: sync.FailRegion}
	d.SetJob(region)
	login := moved(d, "job2", "https://www.bilibili.com/video/BV1GJ411x7h7", sync.JobFailed, 3, 0)
	login.Steps[0].State, login.Failure = sync.StepFailed, &sync.Failure{Step: sync.StepDownload, Code: sync.FailLoginExpired, Remedy: sync.RemedyDesktop}
	d.SetJob(login)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.openTab(jobsTab, msgs.JobsTitle())
	p.app.WaitFor("id:jobs-another-job1", wait)
	if got := p.app.Find("id:jobs-failure-job1").Text; got != msgs.JobsFailRegion() {
		t.Fatalf("the region failure says %q", got)
	}
	if got := p.app.Find("id:jobs-failure-body-job1").Text; got != msgs.JobsRemedyRegion("Studio") {
		t.Fatalf("the region failure's remedy says %q", got)
	}
	if p.app.Exists("id:jobs-retry-job1") {
		t.Fatal("a video restricted by region is offered another try")
	}
	bilibili := msgs.FilmsSourceBilibili()
	if got := p.app.Find("id:jobs-failure-job2").Text; got != msgs.JobsFailLoginExpiredSite(bilibili) {
		t.Fatalf("the sign-in failure says %q", got)
	}
	if got := p.app.Find("id:jobs-failure-body-job2").Text; got != msgs.JobsRemedySignIn(bilibili, "Studio") {
		t.Fatalf("the sign-in failure's remedy says %q", got)
	}
	if got := p.texts("id:jobs-retry-job2"); got != msgs.JobsSignedInRetry() {
		t.Fatalf("the sign-in failure's button reads %q", got)
	}
	p.app.Press("id:jobs-retry-job2")
	p.app.WaitUntil(wait, func() bool {
		for _, j := range d.Jobs() {
			if j.ID == "job2" && j.State != sync.JobFailed {
				return true
			}
		}
		return false
	})
	p.app.Press("id:jobs-another-job1")
	p.app.WaitFor("id:add-link", wait)
}

// A job whose AI service on the computer could not be used, as one out of credit, says so in the phone's own neutral words and offers to try again once it is fixed on the computer: nothing of the computer's own, which carries no message for it, nor of payment.
func TestAJobTheComputersAIServiceCouldNotDoSaysSoNeutrally(t *testing.T) {
	d := newDesktop(t)
	j := moved(d, "job1", "https://youtu.be/ScMzIvxBSi4", sync.JobFailed, 2, 0)
	j.Steps[2].State, j.Failure = sync.StepFailed, &sync.Failure{Step: sync.StepTranslate, Code: sync.FailAIUnavailable, Remedy: sync.RemedyDesktop}
	d.SetJob(j)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.openTab(jobsTab, msgs.JobsTitle())
	p.app.WaitFor("id:jobs-failure-job1", wait)
	if got := p.app.Find("id:jobs-failure-job1").Text; got != msgs.JobsFailAIUnavailable() {
		t.Fatalf("the failure says %q, want %q", got, msgs.JobsFailAIUnavailable())
	}
	if got := p.app.Find("id:jobs-failure-body-job1").Text; got != msgs.JobsRemedyDesktop("Studio") {
		t.Fatalf("the failure's remedy says %q", got)
	}
	if got := p.texts("id:jobs-retry-job1"); got != msgs.JobsFixedRetry() {
		t.Fatalf("the failure's button reads %q", got)
	}
}

// A computer that drops out of reach says when it last answered, not when its jobs were listed, though it was followed over its events for hours after; its unfinished jobs say they wait for it, over their pictures and under their titles, and one with no site to name says so as a sentence.
func TestAComputerOutOfReachSaysWhenItLastAnswered(t *testing.T) {
	var later atomic.Int64
	sessionNow = func() time.Time { return now().Add(time.Duration(later.Load())) }
	t.Cleanup(func() { sessionNow = now })
	d := newDesktop(t)
	// The job is 47% through its three steps, its running one at 40%, so the picture's share is not the step's.
	moved(d, "job1", "https://youtu.be/dQw4w9WgXcQ", sync.JobRunning, 1, 0.4)
	moved(d, "job2", "not a link", sync.JobWaiting, 0, 0)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	p.openTab(jobsTab, msgs.JobsTitle())
	p.app.WaitFor("id:jobs-thumb-job1", wait)
	p.app.WaitUntil(wait, func() bool { return p.app.Find("id:jobs-thumb-job1").Text == msgs.JobsPercent(47) })
	// Two hours on, the computer still answers: the job moves on and the phone hears it over its events.
	later.Store(int64(2 * time.Hour))
	moved(d, "job1", "https://youtu.be/dQw4w9WgXcQ", sync.JobRunning, 2, 0.4)
	p.app.WaitUntil(wait, func() bool { return p.app.Find("id:jobs-thumb-job1").Text == msgs.JobsPercent(80) })
	d.SetOff(true)
	p.app.WaitUntil(reconnect, func() bool { return p.app.Find("id:jobs-thumb-job1").Text == msgs.JobsThumbWaiting() })
	if got, want := p.texts("id:jobs-summary-"+id), msgs.JobsOfflineSeen(msgs.JobsAgoMinutes(1)); got != want {
		t.Fatalf("the computer out of reach says %q, want %q", got, want)
	}
	if got, want := p.app.Find("id:jobs-status-job1").Text, msgs.FilmsSourceYoutube()+" · "+msgs.JobsWaitingSuffix("Studio"); got != want {
		t.Fatalf("the job says %q, want %q", got, want)
	}
	if got, want := p.app.Find("id:jobs-status-job2").Text, msgs.JobsWaitingFor("Studio"); got != want {
		t.Fatalf("the job with no site says %q, want %q", got, want)
	}
}
