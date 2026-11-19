package core

import "testing"

func TestAddressTargetReadsAddressesAndSearches(t *testing.T) {
	for _, c := range []struct{ text, language, want string }{
		{"https://www.youtube.com/watch?v=jNQXAC9IVRw", "zh-Hans", "https://www.youtube.com/watch?v=jNQXAC9IVRw"},
		{"HTTP://example.com/a", "en", "HTTP://example.com/a"},
		{"example.com", "en", "https://example.com"},
		{"www.bilibili.com/video/BV1xx411c7mD?p=2", "zh-Hans", "https://www.bilibili.com/video/BV1xx411c7mD?p=2"},
		{"localhost:8080/watch", "en", "https://localhost:8080/watch"},
		{"192.168.1.2", "en", "https://192.168.1.2"},
		{"例子.中国", "zh-Hans", "https://例子.中国"},
		{"cat videos", "zh-Hans", "https://www.bing.com/search?q=cat+videos"},
		{"cat videos", "en", "https://www.google.com/search?q=cat+videos"},
		{"猫", "ja", "https://www.google.com/search?q=%E7%8C%AB"},
		{"3.14", "en", "https://www.google.com/search?q=3.14"},
		{"golang", "en", "https://www.google.com/search?q=golang"},
		{"file:///etc/passwd", "en", "https://www.google.com/search?q=file%3A%2F%2F%2Fetc%2Fpasswd"},
		{"javascript:alert(1)", "en", "https://www.google.com/search?q=javascript%3Aalert%281%29"},
		{"user@example.com", "en", "https://www.google.com/search?q=user%40example.com"},
	} {
		if got := AddressTarget(c.text, c.language); got != c.want {
			t.Errorf("AddressTarget(%q, %s) = %q, want %q", c.text, c.language, got, c.want)
		}
	}
}
