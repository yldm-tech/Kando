package films

import (
	"os"
	"testing"

	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotDropZone renders what the library shows while files are dragged over it, the drop zone over the content column at the prototype's size (1068×782, its window less the sidebar), as drop-zone. The app's own scenes (TestScreenshotsLibrary in the main package) cannot show it: a drop ends the drag in the same step. It is skipped when KANDO_SCREENSHOTS is unset.
func TestScreenshotDropZone(t *testing.T) {
	if os.Getenv("KANDO_SCREENSHOTS") == "" {
		t.Skip("set KANDO_SCREENSHOTS=<dir> to render the screenshots")
	}
	tt := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		// The window's light theme, as the main window's view sets it; the page under the zone is left transparent, so a comparison can lay it over the library's own picture.
		theme.Apply(cx)
		return core.View{Style: core.Style{FlexGrow: 1}, Children: []core.Node{dropZone{}}}
	}, nsgotest.Size(1068, 782), nsgotest.Environment(testEnvironment()))
	if !tt.Exists("id:films-drop-zone") {
		t.Fatalf("no drop zone\n%s", tt.Dump())
	}
	if nsgotest.Target() == "" {
		// TODO(nsgo#119): write drop-zone.png into KANDO_SCREENSHOTS headless as MyGo's Tester.Image did, once nsgotest renders pixels or hands a test the bytes of its Screenshot; until then the picture comes from `nsgo test -target`, under .nsgo/test/screenshots/<target> https://github.com/nsgo-dev/nsgo/issues/119
		return
	}
	tt.Screenshot("drop-zone")
}
