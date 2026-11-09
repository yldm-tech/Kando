package onboarding

import (
	"os"
	"testing"

	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
)

// TestMain reads the interface in the language Kando shows by default, whatever the languages of the machine running the tests.
func TestMain(m *testing.M) {
	testlocale.Pin(locales.Default)
	os.Exit(m.Run())
}
