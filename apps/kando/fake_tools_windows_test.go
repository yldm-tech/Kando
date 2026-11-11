package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// fakeShell is the sh that runs the fake tools' shell scripts (writeFakeScript), Git for Windows' own, found once as the test binary starts, while PATH is still the one the tests were started with: a test may make PATH a folder of its fakes alone. It is "" where Git for Windows is not installed.
var fakeShell = findFakeShell()

// findFakeShell finds Git for Windows' usr\bin\sh.exe, by the sh or the git on PATH or where Git installs itself. Its bin\sh.exe only starts that one, whose folder also holds the commands the scripts run, such as cat and sleep.
func findFakeShell() string {
	var near []string
	if sh, err := exec.LookPath("sh"); err == nil {
		near = append(near, filepath.Dir(sh))
	}
	if git, err := exec.LookPath("git"); err == nil {
		near = append(near, filepath.Dir(git))
	}
	near = append(near, filepath.Join(os.Getenv("ProgramFiles"), "Git", "bin"))
	for _, dir := range near {
		for _, sh := range []string{filepath.Join(dir, "sh.exe"), filepath.Join(filepath.Dir(dir), "usr", "bin", "sh.exe")} {
			if strings.EqualFold(filepath.Base(filepath.Dir(sh)), "bin") && strings.EqualFold(filepath.Base(filepath.Dir(filepath.Dir(sh))), "usr") {
				if _, err := os.Stat(sh); err == nil {
					return sh
				}
			}
		}
	}
	return ""
}

// fakeScriptShim runs the shell script a fake tool's .exe stands for, when this test binary was started as one (writeFakeScript): the file of the same name without .exe whose first line names the sh. It reports false when there is no such script, and otherwise the script's exit code. The script runs with its sh's folder first on PATH, so it finds the commands it runs whatever PATH the test gave Kando. This process and the script are in a job that ends with it, so a fake Kando stops, as one whose download is cancelled, takes the script and whatever it started with it, rather than leaving them holding the test's files and Kando's end of the pipes.
func fakeScriptShim() (int, bool) {
	exe, err := os.Executable()
	if err != nil || !strings.EqualFold(filepath.Ext(exe), ".exe") {
		return 0, false
	}
	script := strings.TrimSuffix(exe, filepath.Ext(exe))
	data, err := os.ReadFile(script)
	if err != nil {
		return 0, false
	}
	line, _, _ := strings.Cut(string(data), "\n")
	sh, ok := strings.CutPrefix(line, "#!")
	if !ok {
		return 0, false
	}
	if err := fakeScriptJob(); err != nil {
		fmt.Fprintln(os.Stderr, "fake tool:", err)
		return 1, true
	}
	cmd := exec.Command(sh, append([]string{script}, os.Args[1:]...)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	err = cmd.Run()
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		return exit.ExitCode(), true
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake tool:", err)
		return 1, true
	}
	return 0, true
}

// fakeScriptJob puts this process in a new job whose processes are all ended once the last handle to it closes, which is this process's own when it exits or is stopped. The processes it starts after are in the job too.
func fakeScriptJob() error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("create a job: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE}}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		return fmt.Errorf("set up the job: %w", err)
	}
	if err := windows.AssignProcessToJobObject(job, windows.CurrentProcess()); err != nil {
		return fmt.Errorf("join the job: %w", err)
	}
	return nil
}
