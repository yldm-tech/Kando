package feedback

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

// SystemVersion is the system's version as its own tool says it, or "" when it does not.
func SystemVersion(goos string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	switch goos {
	case "darwin":
		out, err := exec.CommandContext(ctx, "sw_vers", "-productVersion").Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	case "windows":
		out, err := exec.CommandContext(ctx, "cmd", "/c", "ver").Output()
		if err != nil {
			return ""
		}
		return windowsVersion(string(out))
	}
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	return osRelease(string(data))
}

// windowsVersion is the number in ver's "Microsoft Windows [Version 10.0.22631.4317]".
func windowsVersion(out string) string {
	out = strings.TrimSpace(out)
	if i := strings.LastIndexByte(out, ' '); i >= 0 {
		return strings.Trim(out[i+1:], "[]")
	}
	return ""
}

// osRelease is the PRETTY_NAME of an /etc/os-release, such as Ubuntu 24.04.1 LTS.
func osRelease(data string) string {
	for line := range strings.SplitSeq(data, "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), "PRETTY_NAME="); ok {
			return strings.Trim(value, `"'`)
		}
	}
	return ""
}

// SystemName writes the system as the issue does: macOS 27.2 (arm64), Windows 10.0.22631 (amd64), or a Linux's own name; without a version, only the system's name.
func SystemName(goos, version, arch string) string {
	name := goos
	switch goos {
	case "darwin":
		name = "macOS"
	case "windows":
		name = "Windows"
	case "linux":
		name = "Linux"
		if version != "" {
			name, version = version, ""
		}
	}
	if version != "" {
		name += " " + version
	}
	return name + " (" + arch + ")"
}
