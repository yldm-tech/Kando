package feedback

import (
	"testing"

	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestMain reads the interface in the language Kando shows by default, whatever the languages of the machine running the tests.
func TestMain(m *testing.M) {
	testlocale.Pin(locales.Default)
	nsgotest.Main(m)
}
