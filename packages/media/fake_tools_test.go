package media

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// The tests run fake yt-dlp, ffmpeg, whisper-cli and everyapi programs instead of whatever this machine has installed. Each fake is a link named after the tool to this test binary (a copy where links cannot be made, or where the code under test follows links), and what it does is written beside it in <tool>.fake.json; started under that name, the binary reads the file, acts as the tool and exits before any test runs. Being the test binary rather than a shell script, a fake runs the same on Linux, macOS and Windows, which cannot start a script.

func init() {
	if fake, ok := readFakeTool(); ok {
		os.Exit(fake.run(os.Args[1:]))
	}
}

// fakeTool is what a fake program does when it runs, each step in the order of the fields.
type fakeTool struct {
	// Want is the leading arguments the fake answers, as everyapi answers only `auth credential`; started with any others it fails. Empty answers any.
	Want []string
	// Runs, when set, is a file the fake adds a line "run" to each time it runs.
	Runs string
	// Args, when set, is a file the fake saves its arguments to, one a line.
	Args string
	// Jar and Mode, when set, are where the fake copies the cookies file it is given with --cookies, and writes that file's permission bits in octal, as yt-dlp reads the file while it runs.
	Jar, Mode string
	// Act is what the fake does as the tool: "" nothing more; "download" downloads a split-format video as yt-dlp does (fakeDownload); "update" answers --version and -U as the standalone yt-dlp does (fakeUpdate); "extract-audio" writes an empty file where its last argument says, as ffmpeg writes the audio track; "transcribe" writes Transcript where -of says, with .json added, as whisper-cli -oj does.
	Act        string
	Transcript string
	// Stdout and Stderr are printed on the two streams, and then the contents of StdoutFile and StderrFile, as a real run of the tool printed them.
	Stdout, Stderr         string
	StdoutFile, StderrFile string
	// Wait keeps the fake running, for 30 seconds, until it is stopped; then, or straight away without Wait, it exits with Exit.
	Wait bool
	Exit int
	// Standalone makes the fake a copy of the test binary rather than a link to it, for code that follows links to the program they name, as FindYTDLPInstall does.
	Standalone bool `json:"-"`
}

// writeFakeTool writes a fake program named name in dir that does what fake says, and returns its path: name, with .exe on Windows, where a program has to have it.
func writeFakeTool(t *testing.T, dir, name string, fake fakeTool) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, file := range []*string{&fake.StdoutFile, &fake.StderrFile} {
		if *file == "" {
			continue
		}
		abs, err := filepath.Abs(*file)
		if err != nil {
			t.Fatal(err)
		}
		*file = abs
	}
	data, err := json.Marshal(fake)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path+".fake.json", data, 0o644); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		path += ".exe"
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if !fake.Standalone && os.Symlink(exe, path) == nil {
		return path
	}
	if err := copyExecutable(exe, path); err != nil {
		t.Fatal(err)
	}
	return path
}

func copyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// fakeBase is the path of the fake this binary was started as without its .exe, which its .fake.json and the files it keeps are named after.
func fakeBase() string {
	return strings.TrimSuffix(os.Args[0], ".exe")
}

// readFakeTool reads what this binary does as a fake from the .fake.json beside it; ok is false when it was started as the test binary.
func readFakeTool() (fake fakeTool, ok bool) {
	data, err := os.ReadFile(fakeBase() + ".fake.json")
	if err != nil {
		return fake, false
	}
	if err := json.Unmarshal(data, &fake); err != nil {
		fmt.Fprintln(os.Stderr, "fake tool:", err)
		os.Exit(2)
	}
	return fake, true
}

func (f fakeTool) run(args []string) int {
	if len(args) < len(f.Want) || strings.Join(args[:len(f.Want)], "\x00") != strings.Join(f.Want, "\x00") {
		fmt.Fprintf(os.Stderr, "fake %s: only %s is supported\n", filepath.Base(fakeBase()), strings.Join(f.Want, " "))
		return 1
	}
	if err := f.act(args); err != nil {
		fmt.Fprintf(os.Stderr, "fake %s: %v\n", filepath.Base(fakeBase()), err)
		return 1
	}
	fmt.Print(f.Stdout)
	fmt.Fprint(os.Stderr, f.Stderr)
	for _, out := range []struct {
		file string
		to   *os.File
	}{{f.StdoutFile, os.Stdout}, {f.StderrFile, os.Stderr}} {
		if out.file == "" {
			continue
		}
		data, err := os.ReadFile(out.file)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		out.to.Write(data)
	}
	if f.Wait {
		time.Sleep(30 * time.Second)
	}
	return f.Exit
}

// act runs every step before printing: the records the test reads afterwards, then Act.
func (f fakeTool) act(args []string) error {
	if f.Runs != "" {
		runs, err := os.OpenFile(f.Runs, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		_, err = runs.WriteString("run\n")
		if err := errors.Join(err, runs.Close()); err != nil {
			return err
		}
	}
	if f.Args != "" {
		var lines strings.Builder
		for _, a := range args {
			lines.WriteString(a + "\n")
		}
		if err := os.WriteFile(f.Args, []byte(lines.String()), 0o644); err != nil {
			return err
		}
	}
	if f.Jar != "" {
		if cookies, ok := argAfterIn(args, "--cookies"); ok {
			info, err := os.Stat(cookies)
			if err != nil {
				return err
			}
			if err := os.WriteFile(f.Mode, []byte(fmt.Sprintf("%o", info.Mode().Perm())), 0o644); err != nil {
				return err
			}
			data, err := os.ReadFile(cookies)
			if err != nil {
				return err
			}
			// The copy appears whole, so a test waiting for it never reads half of it.
			if err := os.WriteFile(f.Jar+".tmp", data, 0o644); err != nil {
				return err
			}
			if err := os.Rename(f.Jar+".tmp", f.Jar); err != nil {
				return err
			}
		}
	}
	switch f.Act {
	case "":
		return nil
	case "download":
		return fakeDownload(args)
	case "update":
		return fakeUpdate(args)
	case "extract-audio":
		if len(args) == 0 {
			return errors.New("no output file")
		}
		return os.WriteFile(args[len(args)-1], nil, 0o644)
	case "transcribe":
		out, ok := argAfterIn(args, "-of")
		if !ok {
			return errors.New("no -of")
		}
		return os.WriteFile(out+".json", []byte(f.Transcript), 0o644)
	}
	return fmt.Errorf("unknown act %q", f.Act)
}

// argAfterIn is the argument after flag in args.
func argAfterIn(args []string, flag string) (string, bool) {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1], true
		}
	}
	return "", false
}

// fakeDownload downloads a split-format video as yt-dlp does: the video and sound tracks go to the temp path (-P temp:) or, without one, next to the video, and are merged into <home>/Me at the zoo [jNQXAC9IVRw]/video.mp4, whose path it prints. When that file is already there it is reported as downloaded and left as it is.
func fakeDownload(args []string) error {
	var home, temp string
	for i := 0; i+1 < len(args) && args[i] != "--"; i++ {
		if args[i] != "-P" {
			continue
		}
		i++
		if rest, ok := strings.CutPrefix(args[i], "temp:"); ok {
			temp = rest
		} else {
			home = args[i]
		}
	}
	const folder = "Me at the zoo [jNQXAC9IVRw]"
	final := filepath.Join(home, folder, "video.mp4")
	if _, err := os.Stat(final); err == nil {
		fmt.Println(final)
		return nil
	}
	if temp == "" {
		temp = home
	}
	work := filepath.Join(temp, folder)
	for _, dir := range []string{work, filepath.Join(home, folder)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tracks := []struct{ name, data string }{{"video.f395.mp4", "track 395"}, {"video.f251.webm", "track 251"}}
	var merged string
	for _, track := range tracks {
		if err := os.WriteFile(filepath.Join(work, track.name), []byte(track.data), 0o644); err != nil {
			return err
		}
	}
	for _, track := range tracks {
		data, err := os.ReadFile(filepath.Join(work, track.name))
		if err != nil {
			return err
		}
		merged += string(data)
	}
	if err := os.WriteFile(filepath.Join(work, "video.temp.mp4"), []byte(merged), 0o644); err != nil {
		return err
	}
	for _, track := range tracks {
		if err := os.Remove(filepath.Join(work, track.name)); err != nil {
			return err
		}
	}
	if err := os.Rename(filepath.Join(work, "video.temp.mp4"), final); err != nil {
		return err
	}
	fmt.Println(final)
	return nil
}

// fakeUpdate is the standalone yt-dlp at version 2024.03.10 until it is given -U, which replaces it with 2026.11.03 as the real one replaces itself. The version is kept in a file beside the fake.
func fakeUpdate(args []string) error {
	version := fakeBase() + ".version"
	current := "2024.03.10"
	if data, err := os.ReadFile(version); err == nil {
		current = strings.TrimSpace(string(data))
	}
	if len(args) == 0 {
		return errors.New("no option")
	}
	switch args[0] {
	case "--version":
		fmt.Println(current)
	case "-U":
		fmt.Println("Current version: stable@" + current)
		fmt.Println("Updating to stable@2026.11.03 ...")
		if err := os.WriteFile(version, []byte("2026.11.03\n"), 0o644); err != nil {
			return err
		}
		fmt.Println("Updated yt-dlp to stable@2026.11.03")
	}
	return nil
}
