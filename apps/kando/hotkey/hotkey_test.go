package hotkey

import "testing"

func TestMatch(t *testing.T) {
	for _, tc := range []struct {
		query, label, group, keys string
		rank                      int
		ok                        bool
	}{
		{"", "全屏", "显示", "F", 1, true},
		{"esc", "返回资料库", "显示", "Esc", 1, true},
		{"字幕", "字幕提前 0.1 秒", "字幕", "Z", 0, true},
		{"字幕", "显示/隐藏字幕", "字幕", "V", 1, true},
		{"Back", "Back to Library", "View", "Esc", 0, true},
		{"view esc", "Back to Library", "View", "Esc", 1, true},
		{"view f", "Back to Library", "View", "Esc", 0, false},
		{"  FULL  ", "Full Screen", "View", "F", 0, true},
		{"火山", "全屏", "显示", "F", 0, false},
	} {
		rank, ok := Match(tc.query, tc.label, tc.group, tc.keys)
		if ok != tc.ok || (ok && rank != tc.rank) {
			t.Errorf("Match(%q, %q, %q, %q) = %d, %v; want %d, %v", tc.query, tc.label, tc.group, tc.keys, rank, ok, tc.rank, tc.ok)
		}
	}
}
