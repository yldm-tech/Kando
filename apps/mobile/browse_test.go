package main

import (
	"fmt"
	"strings"
	"testing"

	"kando/apps/mobile/store"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// browseWeb selects Browse's web view, which stays mounted while the start shows in its place.
const browseWeb = "id:browse-web"

// browseLoads are the addresses the web view was told to load after its first, oldest first.
func browseLoads(p *phone) []string {
	var out []string
	for _, c := range p.app.FindMounted(browseWeb).WebCommands {
		if c.Command == nsgotest.WebLoad {
			out = append(out, fmt.Sprint(c.Data))
		}
	}
	return out
}

// Browse opens a site from its tile; its home page offers nothing to send, and once the site moves within itself to a page of one video the bar over the toolbar names the video and sends it to the computer, which queues it with the phone's name. The start then lists it among the videos last looked at, which opens it again.
func TestBrowseSendsAPagesVideoToTheComputer(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.app.WaitFor(talkRow(store.DesktopID(d.Fingerprint)), wait)
	p.openTab(browseTab, msgs.BrowseTitle())
	p.app.WaitFor("id:browse-site-youtube", wait)
	if p.app.Exists("id:browse-recent") {
		t.Fatal("the start lists videos looked at before any was")
	}
	p.app.Press("id:browse-site-youtube")
	if got := p.app.FindMounted(browseWeb).Web.Source.Src; got != "https://m.youtube.com/" {
		t.Fatalf("the tile opened %q", got)
	}
	p.app.Navigate(browseWeb, core.WebNavigation{URL: "https://m.youtube.com/", Title: "YouTube"})
	if p.app.Exists("id:browse-video") || p.app.Exists("id:browse-start") {
		t.Fatal("YouTube's home page offers a video to send, or the start still shows")
	}
	if got := p.app.Find("id:browse-address").Text; got != "m.youtube.com" {
		t.Fatalf("the address field says %q", got)
	}

	const watch = "https://m.youtube.com/watch?v=jNQXAC9IVRw"
	p.app.PostMessageFrom(browseWeb, "https://m.youtube.com/", true, `kando-page:{"url":"`+watch+`","title":"(2) Me at the zoo - YouTube"}`)
	p.app.WaitFor("id:browse-video", wait)
	if got := p.texts("id:browse-video-title"); got != "Me at the zoo" {
		t.Fatalf("the bar names the video %q", got)
	}
	if got := p.texts("id:browse-video-kicker"); got != msgs.BrowseVideo("Studio") {
		t.Fatalf("the bar says %q, want %q", got, msgs.BrowseVideo("Studio"))
	}
	p.app.Press("id:browse-send")
	p.app.WaitUntil(wait, func() bool { return len(d.Jobs()) == 1 })
	if jobs := d.Jobs(); jobs[0].Link != watch || jobs[0].Device != "Test Phone" {
		t.Fatalf("the computer's queue is %+v", jobs)
	}
	p.app.WaitUntil(wait, func() bool { return p.texts("id:browse-video-kicker") == msgs.BrowseVideoSent("Studio") })
	if send := p.app.Find("id:browse-send"); !send.Props.Disabled || !strings.Contains(p.texts("id:browse-send"), msgs.BrowseSent()) {
		t.Fatalf("once sent, the button says %q and is pressable: %v", p.texts("id:browse-send"), !send.Props.Disabled)
	}
	var prefs store.Prefs
	p.app.WaitUntil(wait, func() bool { prefs, _ = store.Open(dir).Prefs(); return len(prefs.Visited) == 1 })
	if prefs.Visited[0] != (store.Visit{URL: watch, Title: "Me at the zoo"}) {
		t.Fatalf("kept %+v", prefs.Visited)
	}

	p.app.Press("id:browse-home")
	p.app.WaitFor("id:browse-recent", wait)
	if p.app.Exists("id:browse-video") {
		t.Fatal("the start shows the page's video bar")
	}
	if !strings.Contains(p.app.FindMounted(browseWeb).WebCommands[len(p.app.FindMounted(browseWeb).WebCommands)-1].Value, "pause()") {
		t.Fatal("the page out of sight goes on playing")
	}
	p.app.Press("id:browse-recent >> text:Me at the zoo")
	p.app.WaitForGone("id:browse-start", wait)
	if loads := browseLoads(p); len(loads) != 1 || !strings.Contains(loads[0], watch) {
		t.Fatalf("the video looked at loads %q", loads)
	}
}

// With no computer paired, a page's video offers to pair one; a page posting another site's address is not believed, and a page that failed to load says so, while a link to another app the web view refused leaves the page be.
func TestBrowseWithoutAComputerAndPagesThatFail(t *testing.T) {
	dir := t.TempDir()
	keepTalk(t, dir)
	p := startPhone(t, dir)
	p.app.WaitFor(talkRow("gonedesk"), wait)
	p.openTab(browseTab, msgs.BrowseTitle())
	p.app.Press("id:browse-site-vimeo")
	p.app.Navigate(browseWeb, core.WebNavigation{URL: "https://vimeo.com/76979871", Title: "Coastline"})
	p.app.WaitFor("id:browse-video", wait)
	if got := p.texts("id:browse-video-kicker"); got != msgs.BrowseVideoUnpaired() {
		t.Fatalf("the bar says %q with nothing paired", got)
	}

	p.app.Navigate(browseWeb, core.WebNavigation{URL: "https://vimeo.com/channels/staffpicks", Title: "Staff Picks"})
	p.app.WaitForGone("id:browse-video", wait)
	p.app.PostMessageFrom(browseWeb, "https://evil.example/", true, `kando-page:{"url":"https://vimeo.com/76979871","title":"Coastline"}`)
	if p.app.Exists("id:browse-video") {
		t.Fatal("a page claiming another site's address shows its video")
	}

	web := p.app.FindMounted(browseWeb)
	p.app.Report(nsgotest.Event{Kind: nsgotest.EventError, ID: web.ID, Text: "refused", Navigation: &core.WebNavigation{URL: "vimeo://app/76979871"}})
	if p.app.Exists("id:browse-failed") {
		t.Fatal("a link to another app the web view refused says the page failed")
	}
	p.app.Report(nsgotest.Event{Kind: nsgotest.EventError, ID: web.ID, Text: "offline", Navigation: &core.WebNavigation{URL: "https://vimeo.com/channels/staffpicks"}})
	p.app.WaitFor("id:browse-failed", wait)
	p.app.Press("id:browse-reload")
	p.app.WaitForGone("id:browse-failed", wait)

	p.app.Navigate(browseWeb, core.WebNavigation{URL: "https://vimeo.com/76979871", Title: "Coastline"})
	p.app.WaitFor("id:browse-pair", wait)
	p.app.Press("id:browse-pair")
	p.app.WaitFor("id:pair-page", wait)
	if cmds := p.app.FindMounted(browseWeb).WebCommands; !strings.Contains(cmds[len(cmds)-1].Value, "pause()") {
		t.Fatal("the page under pairing goes on playing")
	}
}

// What is typed in the address field opens as an address, or as a search for it; back with nothing to go back to in the page shows the start, and back from the start the page again.
func TestBrowseAddressFieldAndBack(t *testing.T) {
	dir := t.TempDir()
	keepTalk(t, dir)
	p := startPhone(t, dir)
	p.app.WaitFor(talkRow("gonedesk"), wait)
	p.openTab(browseTab, msgs.BrowseTitle())
	p.app.WaitFor("id:browse-address", wait)
	if !p.app.Find("id:browse-back").Props.Disabled {
		t.Fatal("back is on before any page was opened")
	}
	p.app.Type("id:browse-address", "cat videos\n")
	if got := p.app.FindMounted(browseWeb).Web.Source.Src; got != "https://www.google.com/search?q=cat+videos" {
		t.Fatalf("the field opened %q", got)
	}
	p.app.Navigate(browseWeb, core.WebNavigation{URL: "https://www.google.com/search?q=cat+videos", Title: "cat videos"})
	// Taking the keyboard, the field holds the page's whole address to edit.
	p.app.Focus("id:browse-address")
	if got := p.app.Find("id:browse-address").Text; got != "https://www.google.com/search?q=cat+videos" {
		t.Fatalf("the field holds %q to edit", got)
	}
	p.app.Clear("id:browse-address")
	p.app.Type("id:browse-address", "ted.com/talks\n")
	if loads := browseLoads(p); len(loads) != 1 || !strings.Contains(loads[0], "https://ted.com/talks") {
		t.Fatalf("the field loaded %q", loads)
	}
	p.app.Press("id:browse-back")
	p.app.WaitFor("id:browse-start", wait)
	p.app.Press("id:browse-back")
	p.app.WaitForGone("id:browse-start", wait)
}
