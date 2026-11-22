package theme

import "testing"

// Android's navigation bar is the design's in both appearances: its idle tabs grey, the selected tab navy in the light and the accent in the dark, on a pill of the accent at 16%.
func TestTheNavigationBarIsTheDesigns(t *testing.T) {
	th := Kando()
	for _, c := range []struct {
		name     string
		selected uint32
	}{{"light", uint32(M3NavActive)}, {"dark", uint32(Accent)}} {
		p := th.Light
		if c.name == "dark" {
			p = th.Dark
		}
		for role, want := range map[string]uint32{"tabBarIdle": uint32(TabInactive), "tabBarSelected": c.selected, "tabBarIndicator": uint32(Accent.Faded(M3NavIndicator))} {
			if got, _ := p.Color(role); uint32(got) != want {
				t.Errorf("the %s palette's %s is %#x, want %#x", c.name, role, uint32(got), want)
			}
		}
	}
}
