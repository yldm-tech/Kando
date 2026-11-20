package main

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// An Android phone draws the design's Material pieces: the section of what is set on it headed This Phone, Material 3's switch, its 20pt medium section headings, the 22pt regular title in the toolbar of a page Settings opens, and the plus level with the Jobs page's 28pt medium title.
func TestAnAndroidPhoneDrawsMaterial(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("the platform is the target's")
	}
	dir := t.TempDir()
	keepTalk(t, dir)
	p := startPhone(t, dir, nsgotest.Environment(core.Environment{Locale: lang, OS: core.OSAndroid}))
	p.app.WaitFor(talkRow("gonedesk"), wait)

	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.WaitFor("id:settings-goal", wait)
	if !p.app.Exists("text:" + msgs.SettingsThisPhone()) {
		t.Fatalf("Settings on an Android phone heads its own section %q, not %q", msgs.SettingsThisDevice(), msgs.SettingsThisPhone())
	}
	sw := p.app.Find("id:settings-remind-switch")
	if len(sw.Children) != 1 {
		t.Fatalf("the switch has %d parts, want its track", len(sw.Children))
	}
	track := sw.Children[0]
	if track.Frame.Size.Width != 52 || track.Frame.Size.Height != 32 || track.Style.BorderWidth != 2 || track.Style.CornerRadius != 16 {
		t.Fatalf("the switch's track is %v×%v, outlined %v, %v round; want Material's 52×32, outlined 2, 16 round", track.Frame.Size.Width, track.Frame.Size.Height, track.Style.BorderWidth, track.Style.CornerRadius)
	}
	if len(track.Children) != 1 || track.Children[0].Frame.Size.Width != 24 {
		t.Fatalf("the switch's knob is not Material's 24pt knob, scaled down while off")
	}

	p.app.Press("id:settings-style")
	p.app.WaitFor("id:style-page", wait)
	heading := p.app.Find("text:" + msgs.StyleColor())
	if f := heading.TextStyle.Font; f.Size != 20 || f.Weight != core.FontWeightMedium {
		t.Fatalf("a section heading on Android is %vpt weight %v, want 20pt medium", f.Size, f.Weight)
	}
	p.app.WaitUntil(wait, func() bool { return len(p.app.QueryMounted("text:"+msgs.SettingsSubtitleStyle())) > 0 })
	// The Settings row that opened the page reads the same, under it.
	var sizes []core.Points
	for _, v := range p.app.QueryMounted("text:" + msgs.SettingsSubtitleStyle()) {
		if f := v.TextStyle.Font; f.Size == 22 && f.Weight == core.FontWeightNormal {
			sizes = nil
			break
		}
		sizes = append(sizes, v.TextStyle.Font.Size)
	}
	if sizes != nil {
		t.Fatalf("the page's title reads at %v points, none Material's title large, 22pt regular", sizes)
	}

	p.openTab(jobsTab, msgs.JobsTitle())
	p.app.WaitFor("id:jobs-add", wait)
	if row := titleRow(p.app.Find("id:jobs-add"), msgs.JobsTitle()); row == nil {
		t.Fatal("the Jobs page's plus is not on its title's row")
	} else if f := row.TextStyle.Font; f.Size != 28 || f.Weight != core.FontWeightMedium {
		t.Fatalf("the Jobs page's title is %vpt weight %v, want Android's 28pt medium", f.Size, f.Weight)
	}
}

// Elsewhere than Android the section keeps its name, This Device.
func TestOnlyAndroidNamesThisDeviceByItsKind(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("the platform is the target's")
	}
	dir := t.TempDir()
	keepTalk(t, dir)
	p := startPhone(t, dir, nsgotest.Environment(core.Environment{Locale: lang, OS: core.OSIOS}))
	p.app.WaitFor(talkRow("gonedesk"), wait)
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.WaitFor("id:settings-goal", wait)
	if !p.app.Exists("text:"+msgs.SettingsThisDevice()) || p.app.Exists("text:"+msgs.SettingsThisPhone()) {
		t.Fatalf("Settings on an iPhone heads its own section other than %q", msgs.SettingsThisDevice())
	}
}

// titleRow is the text reading title beside v, in v's row or the rows over it up to three levels, nil if there is none.
func titleRow(v *nsgotest.View, title string) *nsgotest.View {
	for level := 0; level < 3 && v.Parent != nil; level++ {
		v = v.Parent
		for _, c := range v.Children {
			if c.Text == title {
				return c
			}
		}
	}
	return nil
}
