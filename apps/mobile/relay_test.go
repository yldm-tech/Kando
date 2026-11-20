package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kando/apps/mobile/store"
	"kando/packages/sync"
	"kando/packages/sync/synctest"
)

// The computer's storage relay ticks every relayTurn in the tests, so what the phone leaves there is answered within a step's wait.
const relayTurn = 20 * time.Millisecond

// openJobs opens the Jobs tab.
func (p *phone) openJobs() {
	p.t.Helper()
	p.openTab(jobsTab, msgs.JobsTitle())
	p.app.WaitFor("id:jobs-page", wait)
}

// waitRelayJoined waits until the phone keeps links to desktop's relay that the storage did not refuse, which it does once the computer's answer to its join arrives. The computer counts a join (RelayJoins) before it answers, so turning it off (SetOff) as soon as it counted one can hang up on the answer, and the phone, holding no relay, never follows it through its storage.
func (p *phone) waitRelayJoined(desktop string) {
	p.t.Helper()
	p.app.WaitUntil(wait, func() bool { r, ok, _ := store.Open(p.dir).Relay(desktop); return ok && !r.Lapsed })
}

// reach is what desktop's card on the Jobs page says of how it is reached.
func (p *phone) reach(desktop string) string {
	sel := "id:jobs-reach-" + desktop
	if !p.app.Exists(sel) {
		return ""
	}
	return p.app.Find(sel).Text
}

// Paired on the computer's network, the phone joins its storage relay, keeping the key in the secure store rather than among its files. Away from that network, the Jobs page follows the computer's queue through the storage: a job is paused there and a link sent there becomes a job of the phone's; back on the network, the phone talks to the computer directly again, and unpairing it forgets its relay.
func TestAwayFromTheComputerTheQueueGoesThroughItsStorage(t *testing.T) {
	d := newDesktop(t)
	d.StartRelay(t, synctest.NewBucket(t, now), now, relayTurn)
	moved(d, "job9", "https://youtu.be/aaaaaaaaaaa", sync.JobRunning, 1, -1)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	p.app.WaitUntil(wait, func() bool { return d.RelayJoins() == 1 })
	p.app.WaitUntil(wait, func() bool { _, ok, _ := store.Open(dir).Relay(id); return ok })
	kept, err := os.ReadFile(filepath.Join(dir, "relay", id+".json"))
	if err != nil || strings.Contains(string(kept), `"key"`) {
		t.Fatalf("the relay kept among the phone's files: %s, %v", kept, err)
	}
	p.openJobs()
	p.app.WaitFor("id:jobs-job-job9", wait)
	if got := p.reach(id); got != msgs.JobsOnline() {
		t.Fatalf("on the computer's network the card says %q", got)
	}

	d.SetOff(true)
	p.app.WaitUntil(reconnect, func() bool { return p.reach(id) == msgs.JobsViaStorage() })
	if got, want := p.app.Find("id:jobs-summary-"+id).Text, msgs.JobsRelayed(msgs.JobsActive(1)); got != want {
		t.Fatalf("away, the card says %q, want %q", got, want)
	}
	p.app.WaitFor("id:jobs-pause-job9", wait)
	p.app.Press("id:jobs-pause-job9")
	p.app.WaitUntil(wait, func() bool { return d.Jobs()[0].State == sync.JobPaused })
	p.app.WaitFor("id:jobs-resume-job9", wait)

	p.app.Press("id:jobs-add")
	p.app.WaitFor("id:add-link", wait)
	p.app.Type("id:add-link", "https://youtu.be/dQw4w9WgXcQ")
	if got := p.app.Find("id:add-note").Text; got != msgs.AddNoteRelay("Studio") {
		t.Fatalf("the Add sheet says %q away from the computer", got)
	}
	p.app.Press("id:add-send")
	p.app.WaitFor("id:jobs-job-job1", wait)
	if jobs := d.Jobs(); len(jobs) != 2 || jobs[1].Link != "https://youtu.be/dQw4w9WgXcQ" || jobs[1].Device != "Test Phone" {
		t.Fatalf("the computer's queue is %+v", jobs)
	}
	p.app.WaitUntil(wait, func() bool { links, _ := store.Open(dir).Links(); return len(links) == 0 })

	d.SetOff(false)
	p.app.WaitUntil(reconnect, func() bool { return p.reach(id) == msgs.JobsOnline() })
	if jobs := d.Jobs(); len(jobs) != 2 {
		t.Fatalf("back on the network, the computer has %d jobs", len(jobs))
	}

	// Unpaired, the computer's relay is forgotten with it.
	p.openComputers()
	p.app.Press("id:computer-unpair")
	p.app.Answer(1)
	p.app.WaitForGone("id:settings-computer-"+id, wait)
	p.app.WaitUntil(wait, func() bool { _, ok, _ := store.Open(dir).Relay(id); return !ok })
}

// A computer that has not written to its storage lately is off: a link sent to it waits in the storage, saying so, and becomes a job once the computer runs again; back on its network, the computer takes it once.
func TestALinkWaitsInTheStorageWhileTheComputerIsOff(t *testing.T) {
	d := newDesktop(t)
	bucket := synctest.NewBucket(t, now)
	// The computer's clock is an hour behind the phone's, so what it writes is stale as soon as it is written.
	d.StartRelay(t, bucket, func() time.Time { return now().Add(-time.Hour) }, relayTurn)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	folder := sync.RelayPrefix(d.Fingerprint)
	p.app.WaitUntil(wait, func() bool { return len(bucket.Keys(folder+"outbox/")) > 0 })
	d.StopRelay()
	d.SetOff(true)
	p.openJobs()
	p.app.WaitUntil(reconnect, func() bool { return p.app.Find("id:jobs-summary-"+id).Text == msgs.JobsOfflineRelay() })

	p.app.Press("id:jobs-add")
	p.app.WaitFor("id:add-link", wait)
	p.app.Type("id:add-link", "https://youtu.be/dQw4w9WgXcQ")
	if got := p.app.Find("id:add-note").Text; got != msgs.AddNoteOfflineRelay("Studio") {
		t.Fatalf("the Add sheet says %q to a computer that is off", got)
	}
	p.app.Press("id:add-send")
	var links []store.Link
	p.app.WaitUntil(wait, func() bool { links, _ = store.Open(dir).Links(); return len(links) == 1 && links[0].Relayed })
	status := "id:jobs-link-status-" + links[0].ID
	p.app.WaitUntil(wait, func() bool { return p.app.Exists(status) && p.app.Find(status).Text == msgs.JobsInStorage("Studio") })
	if got := bucket.Keys(folder + "inbox/"); len(got) != 1 {
		t.Fatalf("the storage's inbox holds %q", got)
	}

	d.ResumeRelay()
	p.app.WaitFor("id:jobs-job-job1", wait)
	p.app.WaitForGone("id:jobs-link-"+links[0].ID, wait)
	d.SetOff(false)
	p.app.WaitUntil(reconnect, func() bool { return p.reach(id) == msgs.JobsOnline() })
	if jobs := d.Jobs(); len(jobs) != 1 {
		t.Fatalf("the computer made %d jobs of the link", len(jobs))
	}
}

// A link left in the storage whose answer the phone has not read yet goes to the computer once it is back on its network, under the same ID, and the computer takes it once.
func TestALinkLeftInTheStorageGoesDirectlyOnceHome(t *testing.T) {
	d := newDesktop(t)
	bucket := synctest.NewBucket(t, now)
	d.StartRelay(t, bucket, now, relayTurn)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	id := store.DesktopID(d.Fingerprint)
	folder := sync.RelayPrefix(d.Fingerprint)
	p.app.WaitUntil(wait, func() bool { return len(bucket.Keys(folder+"outbox/")) > 0 })
	p.openJobs()
	d.SetOff(true)
	p.app.WaitUntil(reconnect, func() bool { return p.reach(id) == msgs.JobsViaStorage() })
	d.StopRelay()
	p.app.Press("id:jobs-add")
	p.app.WaitFor("id:add-link", wait)
	p.app.Type("id:add-link", "https://youtu.be/dQw4w9WgXcQ")
	p.app.Press("id:add-send")
	p.app.WaitUntil(wait, func() bool { links, _ := store.Open(dir).Links(); return len(links) == 1 && links[0].Relayed })

	d.SetOff(false)
	p.app.WaitFor("id:jobs-job-job1", reconnect)
	p.app.WaitUntil(wait, func() bool { links, _ := store.Open(dir).Links(); return len(links) == 0 })
	// The computer, running its relay again, finds the link it took on its network already answered.
	d.ResumeRelay()
	p.app.WaitUntil(wait, func() bool { return len(bucket.Keys(folder+"inbox/")) == 0 })
	if jobs := d.Jobs(); len(jobs) != 1 {
		t.Fatalf("the computer made %d jobs of the link", len(jobs))
	}
}
