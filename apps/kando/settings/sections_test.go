package settings

import (
	"testing"

	"kando/apps/kando/sidebar"
)

// TestSectionsAreTheSidebarPages checks the sections that are sidebar pages have the pages' values, which the main package's OpenSettings selects as they are, now that sectionid declares them without importing the sidebar.
func TestSectionsAreTheSidebarPages(t *testing.T) {
	for section, page := range map[string]string{
		SectionGeneral:   sidebar.PageGeneral,
		SectionSpeech:    sidebar.PageSpeech,
		SectionAI:        sidebar.PageAI,
		SectionTranslate: sidebar.PageTranslate,
		SectionAccount:   sidebar.PageAccount,
	} {
		if section != page {
			t.Errorf("section %q is sidebar page %q", section, page)
		}
	}
}
