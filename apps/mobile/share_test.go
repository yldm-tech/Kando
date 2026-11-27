package main

import (
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/mobile"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// sharedText is text another app shares, as Android's SEND of text/plain and iOS's share extension both deliver it.
func sharedText(text string) mobile.SharedItem {
	return mobile.SharedItem{Text: text, MIME: "text/plain"}
}

// onPhone is the phone's system for each of the share sheets the app is in: iOS's share extension, Android's SEND.
var onPhone = map[string]core.OS{"ios": core.OSIOS, "android": core.OSAndroid}

// waitAddLink waits for the Add sheet to open with want in its link field.
func (p *phone) waitAddLink(want string) {
	p.t.Helper()
	p.app.WaitFor("id:add-link", wait)
	p.app.WaitUntil(wait, func() bool { return p.app.Find("id:add-link").Text == want })
}

// A link YouTube, Safari or Bilibili shares to Kando opens the Add sheet with it in the field, on iOS and Android alike: a bare link, the first link in a share sheet's text that puts the video's title before it, and text with no link, which the sheet says is not one, as a paste of it does. It goes to the computer from there as one pasted does.
func TestALinkSharedFromAnotherAppOpensTheAddSheet(t *testing.T) {
	for name, os := range onPhone {
		t.Run(name, func(t *testing.T) {
			d := newDesktop(t)
			dir := t.TempDir()
			p := startPhone(t, dir, pairedWith(t, d, dir), nsgotest.Environment(core.Environment{Locale: lang, OS: os}))
			p.app.WaitFor("id:films-add", wait)

			p.app.ReceiveShare(sharedText("https://youtu.be/dQw4w9WgXcQ"))
			p.waitAddLink("https://youtu.be/dQw4w9WgXcQ")
			if got := p.app.WaitFor("id:add-site", wait).Text; got != msgs.FilmsSourceYoutube() {
				t.Fatalf("the sheet names the site %q, want %q", got, msgs.FilmsSourceYoutube())
			}
			p.app.Press("id:add-cancel")
			p.app.WaitForGone("id:add-link", wait)

			p.app.ReceiveShare(sharedText("Never Gonna Give You Up\nhttps://www.bilibili.com/video/BV1GJ411x7h7 via Bilibili"))
			p.waitAddLink("https://www.bilibili.com/video/BV1GJ411x7h7")
			p.app.Press("id:add-cancel")
			p.app.WaitForGone("id:add-link", wait)

			p.app.ReceiveShare(sharedText("just some words"))
			p.waitAddLink("just some words")
			p.app.WaitFor("id:add-invalid", wait)

			p.app.ReceiveShare(sharedText("Look https://youtu.be/dQw4w9WgXcQ"))
			p.waitAddLink("https://youtu.be/dQw4w9WgXcQ")
			p.app.Press("id:add-send")
			p.app.WaitFor("id:jobs-page", wait)
			p.app.WaitUntil(wait, func() bool { jobs := d.Jobs(); return len(jobs) == 1 && jobs[0].Link == "https://youtu.be/dQw4w9WgXcQ" })
		})
	}
}

// Of a share with several items, as iOS's of a page's link and its title, the field takes the first link among them; a file alone, as a picture or a video file, is not the phone's to take, and a toast says so with no sheet.
func TestAShareTakesItsLinkAndLeavesFiles(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir), nsgotest.Environment(core.Environment{Locale: lang, OS: core.OSIOS}))
	p.app.WaitFor("id:films-add", wait)

	p.app.ReceiveShare(mobile.SharedItem{Path: "/cache/clip.mp4", Name: "clip.mp4", MIME: "video/mp4"})
	p.app.WaitUntil(wait, func() bool {
		return p.app.Exists("id:toasts") && strings.Contains(p.texts("id:toasts"), msgs.AddSharedFile())
	})
	if p.app.Exists("id:add-link") {
		t.Fatal("a file shared alone opens the Add sheet")
	}

	p.app.ReceiveShare(sharedText("Never Gonna Give You Up"), sharedText("https://youtu.be/dQw4w9WgXcQ"))
	p.waitAddLink("https://youtu.be/dQw4w9WgXcQ")
}

// A link shared with no computer paired opens the pairing page, as the Library's Add does, and once the phone paired, the Add sheet with the link; one shared while the introduction shows waits for the user to go through it.
func TestALinkSharedBeforePairingWaitsForIt(t *testing.T) {
	d := newDesktop(t)
	dir := t.TempDir()
	keepTalk(t, dir)
	p := startPhone(t, dir)
	p.app.WaitFor(talkRow("gonedesk"), wait)
	p.app.ReceiveShare(sharedText("https://youtu.be/dQw4w9WgXcQ"))
	p.app.WaitFor("id:pair-page", wait)
	if p.app.Exists("id:add-link") {
		t.Fatal("the Add sheet opens with no computer to send the link to")
	}
	p.fillPairing(d.Addr.String(), "123456")
	p.app.Press("id:pair-continue")
	p.app.WaitFor("id:pair-fingerprint", wait)
	p.app.Press("id:pair-confirm")
	p.waitAddLink("https://youtu.be/dQw4w9WgXcQ")

	d = newDesktop(t)
	p = startPhone(t, "")
	p.app.WaitFor("id:onboarding-intro", wait)
	p.app.ReceiveShare(sharedText("https://youtu.be/dQw4w9WgXcQ"))
	p.startPairing()
	if p.app.Exists("id:add-link") {
		t.Fatal("the Add sheet opens over the introduction")
	}
	p.fillPairing(d.Addr.String(), "123456")
	p.app.Press("id:pair-continue")
	p.app.WaitFor("id:pair-fingerprint", wait)
	p.app.Press("id:pair-confirm")
	p.app.WaitFor("id:onboarding-paired", wait)
	p.app.Press("id:onboarding-start")
	p.waitAddLink("https://youtu.be/dQw4w9WgXcQ")
}

// TestScreenshotsShare is the Add sheet a link YouTube shares to Kando opens, its title before the link, and the toast of a video file shared alone.
func TestScreenshotsShare(t *testing.T) {
	shot := screenshots(t)
	d := newDesktop(t)
	dir := t.TempDir()
	p := startPhone(t, dir, pairedWith(t, d, dir))
	p.app.WaitFor("id:films-add", wait)
	p.app.ReceiveShare(sharedText("Never Gonna Give You Up https://youtu.be/dQw4w9WgXcQ"))
	p.waitAddLink("https://youtu.be/dQw4w9WgXcQ")
	p.app.WaitFor("id:add-site", wait)
	shot(p, "share-link")
	p.app.Press("id:add-cancel")
	p.app.WaitForGone("id:add-link", wait)
	p.app.ReceiveShare(mobile.SharedItem{Path: "/cache/clip.mp4", Name: "clip.mp4", MIME: "video/mp4"})
	p.app.WaitUntil(wait, func() bool {
		return p.app.Exists("id:toasts") && strings.Contains(p.texts("id:toasts"), msgs.AddSharedFile())
	})
	shot(p, "share-file")
}
