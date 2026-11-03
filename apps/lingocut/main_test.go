package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// The view runs without a window in tests, which click and type as a user
// would.
func TestView(t *testing.T) {
	a := &app{}
	tt := ui.NewTester(a.view, 820, 520)
	if err := tt.Click("视频链接"); err != nil {
		t.Fatal(err)
	}
	tt.Type("https://example.com/video")
	if err := tt.Click("开始处理"); err != nil {
		t.Fatal(err)
	}
	if a.job == nil || a.status != "任务已加入队列" {
		t.Errorf("texts %q after typing a name", tt.Texts())
	}
}
