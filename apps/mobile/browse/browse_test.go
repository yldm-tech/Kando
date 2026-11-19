package browse

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
)

// A page's address and title are taken only from a page of the same origin, on the web, in the shape pageScript posts them.
func TestParsePageTakesOnlyAPagesOwnAddress(t *testing.T) {
	for _, c := range []struct {
		origin, data string
		want         bool
	}{
		{"https://m.youtube.com", `kando-page:{"url":"https://m.youtube.com/watch?v=jNQXAC9IVRw","title":"Me at the zoo"}`, true},
		{"https://m.youtube.com", `kando-page:{"url":"https://M.YouTube.com/watch?v=jNQXAC9IVRw","title":""}`, true},
		{"https://evil.example", `kando-page:{"url":"https://m.youtube.com/watch?v=jNQXAC9IVRw","title":"Me at the zoo"}`, false},
		{"http://m.youtube.com", `kando-page:{"url":"https://m.youtube.com/watch?v=jNQXAC9IVRw"}`, false},
		{"null", `kando-page:{"url":"javascript:alert(1)"}`, false},
		{"https://m.youtube.com", `kando-page:not json`, false},
		{"https://m.youtube.com", `{"url":"https://m.youtube.com/watch?v=jNQXAC9IVRw"}`, false},
	} {
		_, ok := parsePage(core.WebMessage{Origin: c.origin, Data: c.data})
		if ok != c.want {
			t.Errorf("parsePage(%s, %s) = %v, want %v", c.origin, c.data, ok, c.want)
		}
	}
}
