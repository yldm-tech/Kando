package media

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// YTDLPInstall is how the yt-dlp at a path was installed, which decides how it is updated.
type YTDLPInstall struct {
	// Kind is "homebrew", "pipx", "pip" or "standalone".
	Kind string
	// Update is the command that updates it, or nil when Kando cannot; Hint then says how to.
	Update []string
	Hint   string
}

// FindYTDLPInstall tells how the yt-dlp at path was installed: by Homebrew (its file is in a Cellar), by pipx or pip (it is a Python script), or as the standalone build, which updates itself with -U when Kando may write it.
func FindYTDLPInstall(path string) YTDLPInstall {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		resolved = path
	}
	slashed := filepath.ToSlash(resolved)
	if i := strings.Index(slashed, "/Cellar/yt-dlp/"); i >= 0 {
		brew := filepath.Join(filepath.FromSlash(slashed[:i]), "bin", "brew")
		if isExecutable(brew) {
			return YTDLPInstall{Kind: "homebrew", Update: []string{brew, "upgrade", "yt-dlp"}}
		}
		return YTDLPInstall{Kind: "homebrew", Hint: "在终端里运行 brew upgrade yt-dlp"}
	}
	if interpreter := pythonShebang(resolved); interpreter != "" {
		if strings.Contains(slashed, "/pipx/venvs/") {
			if pipx := lookTool("pipx"); pipx != "" {
				return YTDLPInstall{Kind: "pipx", Update: []string{pipx, "upgrade", "yt-dlp"}}
			}
			return YTDLPInstall{Kind: "pipx", Hint: "在终端里运行 pipx upgrade yt-dlp"}
		}
		return YTDLPInstall{Kind: "pip", Update: []string{interpreter, "-m", "pip", "install", "--upgrade", "yt-dlp"}}
	}
	if writable(resolved) {
		return YTDLPInstall{Kind: "standalone", Update: []string{resolved, "-U"}}
	}
	return YTDLPInstall{Kind: "standalone", Hint: "yt-dlp 在 " + filepath.Dir(resolved) + "，看懂没有权限替换它；用安装它的方式更新，或运行 sudo yt-dlp -U"}
}

// pythonShebang is the interpreter a Python script at path names on its first line, or "" when path is not one.
func pythonShebang(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	line, err := bufio.NewReader(f).ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "#!") || !strings.Contains(line, "python") {
		return ""
	}
	fields := strings.Fields(strings.TrimPrefix(line, "#!"))
	if len(fields) == 0 {
		return ""
	}
	// "#!/usr/bin/env python3" names the interpreter after env.
	if filepath.Base(fields[0]) == "env" && len(fields) > 1 {
		return lookTool(fields[1])
	}
	return fields[0]
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && (runtime.GOOS == "windows" || info.Mode()&0o111 != 0)
}

// writable reports whether the file at path, and the folder it is in, may be written: yt-dlp -U writes the new build beside the old one and renames it over.
func writable(path string) bool {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	f.Close()
	probe, err := os.CreateTemp(filepath.Dir(path), ".kando-write-*")
	if err != nil {
		return false
	}
	probe.Close()
	os.Remove(probe.Name())
	return true
}

// YTDLPVersion is what the yt-dlp at path says its version is, such as 2026.08.19.
func YTDLPVersion(ctx context.Context, path string) (string, error) {
	if path == "" {
		return "", errors.New("yt-dlp is not installed")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := run(ctx, path, "--version")
	if err != nil {
		return "", err
	}
	return lastLine(out), nil
}

// UpdateYTDLP updates the yt-dlp at path the way it was installed (FindYTDLPInstall) and returns the version it has afterwards. When Kando cannot update it, the error says how to.
func UpdateYTDLP(ctx context.Context, path string) (string, error) {
	if path == "" {
		return "", errors.New("yt-dlp is not installed")
	}
	install := FindYTDLPInstall(path)
	if install.Update == nil {
		return "", errors.New(install.Hint)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if _, err := run(ctx, install.Update[0], install.Update[1:]...); err != nil {
		return "", fmt.Errorf("%s: %w", strings.Join(install.Update, " "), err)
	}
	return YTDLPVersion(ctx, path)
}
