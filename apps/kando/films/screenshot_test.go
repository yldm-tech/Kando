package films

import (
	"os"
	"testing"

	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotDropZone renders what the library shows while files are dragged over it, the drop zone over the content column at the prototype's size (1068×782, its window less the sidebar), as drop-zone, or drop-zone-dark in the dark appearance with KANDO_SCREENSHOTS_DARK=1, as the app's scenes are. The app's own scenes (TestScreenshotsLibrary in the main package) cannot show it: a drop ends the drag in the same step. It is skipped when KANDO_SCREENSHOTS is unset.
func TestScreenshotDropZone(t *testing.T) {
	if os.Getenv("KANDO_SCREENSHOTS") == "" {
		t.Skip("set KANDO_SCREENSHOTS=<dir> to render the screenshots")
	}
	env, name := testEnvironment(), "drop-zone"
	if os.Getenv("KANDO_SCREENSHOTS_DARK") == "1" {
		env.Dark, name = true, "drop-zone-dark"
	}
	tt := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		// The window's theme, as the main window's view sets it; the page under the zone is left transparent, so a comparison can lay it over the library's own picture.
		theme.Apply(cx)
		return core.View{Style: core.Style{FlexGrow: 1}, Children: []core.Node{dropZone{}}}
	}, nsgotest.Size(1068, 782), nsgotest.Environment(env))
	if !tt.Exists("id:films-drop-zone") {
		t.Fatalf("no drop zone\n%s", tt.Dump())
	}
	if nsgotest.Target() == "" {
		// Headless runs have no pixels; the picture comes from `nsgo test -target`, which saves it under .nsgo/test/screenshots/<target> (README's Screenshots), the package's tests linking the platform for it (platform_test.go).
		return
	}
	tt.Screenshot(name)
}
