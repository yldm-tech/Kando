package feedback

import "testing"

func TestFeedbackSystemNames(t *testing.T) {
	cases := []struct{ goos, version, arch, want string }{
		{"darwin", "27.2", "arm64", "macOS 27.2 (arm64)"},
		{"darwin", "", "arm64", "macOS (arm64)"},
		{"windows", windowsVersion("\r\nMicrosoft Windows [Version 10.0.22631.4317]\r\n"), "amd64", "Windows 10.0.22631.4317 (amd64)"},
		{"linux", osRelease("NAME=\"Ubuntu\"\nPRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\nID=ubuntu\n"), "amd64", "Ubuntu 24.04.1 LTS (amd64)"},
		{"linux", "", "arm64", "Linux (arm64)"},
	}
	for _, c := range cases {
		if got := SystemName(c.goos, c.version, c.arch); got != c.want {
			t.Errorf("%s %q: %q, want %q", c.goos, c.version, got, c.want)
		}
	}
}
