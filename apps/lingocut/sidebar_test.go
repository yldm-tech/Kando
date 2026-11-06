package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestSidebarKeepsContentBelowTheTitleBar checks that the drag band under the title bar pushes the sidebar and the content beside it down by the title bar's height.
func TestSidebarKeepsContentBelowTheTitleBar(t *testing.T) {
	texts := []string{"全部影片", "开始处理"}
	shifts, _ := titleBarShifts(t, ui.TitleBar{Height: 28, Left: 80}, nil, texts...)
	for i, d := range shifts {
		if abs32(d-28) > 0.5 {
			t.Fatalf("%q moved %v under a 28-high title bar", texts[i], d)
		}
	}
}
