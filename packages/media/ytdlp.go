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
	// Kind is "homebrew", "pipx", "pip", "system" or "standalone". "system" is a Python script Kando may not write outside Homebrew and pipx, as the yt-dlp a Linux distribution's packages put in /usr/bin or one sudo pip installed: pip run by Kando cannot upgrade it in place (it is missing there, refuses an externally managed Python, or installs a copy for the user that leaves the one Kando runs as it was), so it is updated the way it was installed, with the package manager or as root.
	Kind string
	// Update is the command that updates it, or nil when Kando cannot; the app then tells the user how from Kind and Dir.
	Update []string
	// Dir is the folder the yt-dlp is in once its links are followed.
	Dir string
}

// ErrCannotUpdate is UpdateYTDLP's error when Kando cannot update the yt-dlp itself (its YTDLPInstall has no Update).
var ErrCannotUpdate = errors.New("yt-dlp cannot be updated by Kando")

// FindYTDLPInstall tells how the yt-dlp at path was installed: by Homebrew (its file is in a Cellar), by pipx (a Python script in a pipx venv), by pip (a Python script Kando may write) or by the system's package manager (one it may not), or as the standalone build, which updates itself with -U when Kando may write it.
func FindYTDLPInstall(path string) YTDLPInstall {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		resolved = path
	}
	slashed := filepath.ToSlash(resolved)
	dir := filepath.Dir(resolved)
	if i := strings.Index(slashed, "/Cellar/yt-dlp/"); i >= 0 {
		brew := filepath.Join(filepath.FromSlash(slashed[:i]), "bin", "brew")
		if isExecutable(brew) {
			return YTDLPInstall{Kind: "homebrew", Update: []string{brew, "upgrade", "yt-dlp"}, Dir: dir}
		}
		return YTDLPInstall{Kind: "homebrew", Dir: dir}
	}
	if interpreter := pythonShebang(resolved); interpreter != "" {
		if strings.Contains(slashed, "/pipx/venvs/") {
			if pipx := lookTool("pipx"); pipx != "" {
				return YTDLPInstall{Kind: "pipx", Update: []string{pipx, "upgrade", "yt-dlp"}, Dir: dir}
			}
			return YTDLPInstall{Kind: "pipx", Dir: dir}
		}
		if writable(resolved) {
			return YTDLPInstall{Kind: "pip", Update: []string{interpreter, "-m", "pip", "install", "--upgrade", "yt-dlp"}, Dir: dir}
		}
		return YTDLPInstall{Kind: "system", Dir: dir}
	}
	if writable(resolved) {
		return YTDLPInstall{Kind: "standalone", Update: []string{resolved, "-U"}, Dir: dir}
	}
	return YTDLPInstall{Kind: "standalone", Dir: dir}
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

// UpdateYTDLP updates the yt-dlp at path the way it was installed (FindYTDLPInstall) and returns the version it has afterwards. When Kando cannot update it, the error is ErrCannotUpdate.
func UpdateYTDLP(ctx context.Context, path string) (string, error) {
	if path == "" {
		return "", errors.New("yt-dlp is not installed")
	}
	install := FindYTDLPInstall(path)
	if install.Update == nil {
		return "", fmt.Errorf("%w: %s install in %s", ErrCannotUpdate, install.Kind, install.Dir)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if _, err := run(ctx, install.Update[0], install.Update[1:]...); err != nil {
		return "", fmt.Errorf("%s: %w", strings.Join(install.Update, " "), err)
	}
	return YTDLPVersion(ctx, path)
}
