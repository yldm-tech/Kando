package films

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// TestScreenshotDropZone renders what the library shows while files are dragged over it, the drop zone over the content column at the prototype's size (1068×782, its window less the sidebar), to drop-zone.png in the directory named by LINGOCUT_SCREENSHOTS. The app's own scenes (TestScreenshotsLibrary in the main package) cannot show it: the tester drops the files it drags over the window in the same step. It is skipped when the variable is unset.
func TestScreenshotDropZone(t *testing.T) {
	out := os.Getenv("LINGOCUT_SCREENSHOTS")
	if out == "" {
		t.Skip("set LINGOCUT_SCREENSHOTS=<dir> to render the screenshots")
	}
	tt := ui.NewTester(func(c *ui.Context) {
		// The window's light theme, as main's palette sets it.
		th := breeze.ThemeOf(c, breeze.PlatformOf(c), false, ui.Hex("#0A66D8"))
		th.Background, th.Text, th.TextMuted = ui.Hex("#FFFFFF"), ui.Hex("#1D1D1F"), ui.Hex("#6A6A70")
		c.SetTheme(th)
		// The page under it is left transparent, so a comparison can lay it over the library's own picture.
		c.Root().Background(ui.Transparent)
		ui.Box(c).Fill().Children(func() { dropZone(c, th) })
	}, 1068, 782)
	tt.Frame()
	file, err := os.Create(filepath.Join(out, "drop-zone.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, tt.Image()); err != nil {
		t.Fatal(err)
	}
}
