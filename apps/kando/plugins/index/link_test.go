package index

import "testing"

// TestInstallLink checks the link that asks Kando to install a pack of the index: InstallLink's link reads back its id, a link naming no valid id reads as one with no id, and any other link is not one.
func TestInstallLink(t *testing.T) {
	if link := InstallLink("example.anime-ja-zh"); link != "kando://plugin/install?id=example.anime-ja-zh" {
		t.Errorf("InstallLink = %q", link)
	}
	for link, want := range map[string]string{
		InstallLink("acme.hello"):                    "acme.hello",
		"KANDO://plugin/install/?id=example.terms":   "example.terms",
		"kando:plugin/install?id=example.terms":      "example.terms",
		"kando://plugin/install?id=../../etc/passwd": "",
		"kando://plugin/install?id=Example.Terms":    "",
		"kando://plugin/install":                     "",
		"kando://plugin/install?id=noseparator":      "",
	} {
		if id, ok := ReadInstallLink(link); !ok || id != want {
			t.Errorf("ReadInstallLink(%q) = %q, %v, want %q", link, id, ok, want)
		}
	}
	for _, link := range []string{"kando://add?url=https%3A%2F%2Fexample.org", "kando://plugin/other?id=example.terms", "https://kando.yldm.ai/plugin/install?id=example.terms", "kando://plugin?id=example.terms", "::"} {
		if _, ok := ReadInstallLink(link); ok {
			t.Errorf("ReadInstallLink(%q) took it", link)
		}
	}
}
