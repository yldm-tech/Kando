package media

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The tests run fake yt-dlp, ffmpeg, whisper-cli and everyapi programs instead of whatever this machine has installed. Each fake is a link named after the tool to this test binary (a copy where links cannot be made, or where the code under test follows links: on macOS a script that runs a link instead, writeFakeTool), and what it does is written beside it in <tool>.fake.json; started under that name, the binary reads the file, acts as the tool and exits before any test runs. Being the test binary rather than a shell script, a fake runs the same on Linux, macOS and Windows, which cannot start a script.

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
	// Cookies, when set, is what the fake saves to the cookies file it is given with --cookies, as yt-dlp saves there the cookies it read from a browser.
	Cookies string
	// Act is what the fake does as the tool: "" nothing more; "download" downloads a split-format video as yt-dlp does (fakeDownload); "update" answers --version and -U as the standalone yt-dlp does (fakeUpdate); "extract-audio" writes an empty file where its last argument says, as ffmpeg writes the audio track; "subtitles" writes Files in the folder -P names; "transcribe" writes Transcript where -of says, with .json added, as whisper-cli -oj does; "extract-wav" writes a silent 16 kHz WAV as long as -t says where its last argument says, as ffmpeg writes the audio of part of a film; "speech" is a whisper-cli that finds speech (fakeSpeech).
	Act        string
	Transcript string
	// Speech is where the "speech" fake says the speech is, in seconds into each sound it is given, and Told the languages it tells its sounds as, in turn; with NoVAD it prints nothing of the speech it finds, as a whisper-cli whose VAD failed, and with NoVADAgain only the first time it is asked in a folder at a threshold. With Timing it has -dtw and times each word; with FailTiming it fails to start when asked to time words, as a whisper-cli given a preset that does not fit its model.
	Speech [][2]float64 `json:",omitempty"`
	// Quiet, when set, is where it says the speech is when asked at quietThreshold, as a VAD less sure finds more; Says, when set, is what it hears in each file in place of the file's name, and NoVoice what it hears in a file of noVoiceSounds.
	Quiet      [][2]float64 `json:",omitempty"`
	Says       string       `json:",omitempty"`
	NoVoice    string       `json:",omitempty"`
	Told       []string     `json:",omitempty"`
	NoVAD      bool         `json:",omitempty"`
	NoVADAgain bool         `json:",omitempty"`
	Timing     bool         `json:",omitempty"`
	FailTiming bool         `json:",omitempty"`
	// Files are what Act "subtitles" writes in the folder -P names, by file name, as yt-dlp --skip-download --write-subs writes the subtitles it fetched.
	Files map[string]string `json:",omitempty"`
	// Stdout and Stderr are printed on the two streams, and then the contents of StdoutFile and StderrFile, as a real run of the tool printed them.
	Stdout, Stderr         string
	StdoutFile, StderrFile string
	// Wait keeps the fake running, for 30 seconds, until it is stopped; then, or straight away without Wait, it exits with Exit.
	Wait bool
	Exit int
	// SignedOut, when set, is what the fake does in place of all the above when it is not given --cookies-from-browser, as yt-dlp that could not read a browser's sign-in fails and the same run without it does not.
	SignedOut *fakeTool `json:",omitempty"`
	// Standalone makes the fake a file of its own rather than a link to the test binary, for code that follows links to the program they name, as FindYTDLPInstall does.
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
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if fake.Standalone && runtime.GOOS == "darwin" {
		// A copy of the test binary is a new program to macOS, which the system's security policy (syspolicyd) assesses as it starts; while many new programs start at once, as in parallel test runs, the assessment stalls or fails and the kernel kills the program ("ASP: Security policy would not allow process" in the system log), so the fake died of SIGKILL or hung past its caller's deadline. A hard link to the test binary is killed every time it runs under its other name. So on macOS the standalone fake is a script of its own at path that runs a link to the test binary kept in a folder beside it, and what the fake does is written beside that link, which it is started as; macOS lets a program's file be written while it runs, so a script stands for a running standalone yt-dlp there as well as a copy did.
		link := filepath.Join(dir, ".fake", name)
		if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(link+".fake.json", data, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(exe, link); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexec '"+link+"' \"$@\"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	if err := os.WriteFile(path+".fake.json", data, 0o644); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		path += ".exe"
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
	if f.SignedOut != nil && !slices.Contains(args, "--cookies-from-browser") {
		return f.SignedOut.run(args)
	}
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
	if f.Cookies != "" {
		cookies, ok := argAfterIn(args, "--cookies")
		if !ok {
			return errors.New("no --cookies")
		}
		if err := os.WriteFile(cookies, []byte(f.Cookies), 0o600); err != nil {
			return err
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
	case "subtitles":
		dir, ok := argAfterIn(args, "-P")
		if !ok {
			return errors.New("no -P")
		}
		for name, data := range f.Files {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
				return err
			}
		}
		return nil
	case "transcribe":
		out, ok := argAfterIn(args, "-of")
		if !ok {
			return errors.New("no -of")
		}
		return os.WriteFile(out+".json", []byte(f.Transcript), 0o644)
	case "extract-wav":
		if len(args) == 0 {
			return errors.New("no output file")
		}
		length := time.Minute
		if t, ok := argAfterIn(args, "-t"); ok {
			secs, err := strconv.ParseFloat(t, 64)
			if err != nil {
				return err
			}
			length = time.Duration(secs * float64(time.Second))
		}
		return writePCM(args[len(args)-1], make([]byte, int(length.Seconds()*16000)*2))
	case "speech":
		return f.fakeSpeech(args)
	}
	return fmt.Errorf("unknown act %q", f.Act)
}

// fakeSpeech acts as a whisper-cli with --vad: it answers -h with its options, with -dtw among them given Timing; with --vad and -dl it prints where the speech is in the sound it is given (Speech), as its VAD does; with -dl alone, the language it tells each sound it is given as (Told); given -of, it writes Transcript as "transcribe" does; otherwise it hears each file in turn, as whisper-cli does: it says on stderr it reads the file, prints a line naming the file on stdout, says on stderr it saves what it heard and then writes it as JSON beside the file. With -dtw the line is "Hi! Bye", its words timed 0.4 s and 3.6 s into the file.
func (f fakeTool) fakeSpeech(args []string) error {
	if slices.Equal(args, []string{"-h"}) {
		fmt.Fprintln(os.Stderr, "  -vm FNAME, --vad-model FNAME [       ] VAD model path")
		if f.Timing || f.FailTiming {
			fmt.Fprintln(os.Stderr, "  -dtw MODEL --dtw MODEL            [       ] compute token-level timestamps")
			fmt.Fprintln(os.Stderr, "  -nfa,      --no-flash-attn        [false  ] disable flash attention")
		}
		return nil
	}
	var files []string
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-f" {
			files = append(files, args[i+1])
		}
	}
	timed := slices.Contains(args, "-dtw")
	switch {
	case slices.Contains(args, "--vad"):
		if f.NoVAD {
			return nil
		}
		if f.NoVADAgain && len(files) > 0 {
			// Asked at each threshold once, as the speech is found at two at once (findVoices).
			vt, _ := argAfterIn(args, "-vt")
			marker := filepath.Join(filepath.Dir(files[0]), "vad-asked-"+vt)
			if _, err := os.Stat(marker); err == nil {
				return nil
			}
			if err := os.WriteFile(marker, nil, 0o644); err != nil {
				return err
			}
		}
		speech := f.Speech
		if vt, _ := argAfterIn(args, "-vt"); vt == quietThreshold && f.Quiet != nil {
			speech = f.Quiet
		}
		for i, s := range speech {
			fmt.Fprintf(os.Stderr, "whisper_vad_segments_from_probs: VAD segment %d: start = %.2f, end = %.2f (duration: %.2f)\n", i, s[0], s[1], s[1]-s[0])
		}
		fmt.Fprintf(os.Stderr, "whisper_vad_segments_from_probs: Final speech segments after filtering: %d\n", len(speech))
	case slices.Contains(args, "-dl"):
		for i, file := range files {
			fmt.Fprintf(os.Stderr, "main: processing '%s'\n", file)
			fmt.Fprintf(os.Stderr, "whisper_full_with_state: auto-detected language: %s (p = 0.900000)\n", f.Told[i%len(f.Told)])
		}
	case slices.Contains(args, "-of"):
		out, _ := argAfterIn(args, "-of")
		fmt.Println("[00:00:01.000 --> 00:00:02.000]  heard whole")
		return os.WriteFile(out+".json", []byte(f.Transcript), 0o644)
	case timed && f.FailTiming:
		return errors.New("whisper_init_with_params_no_state: tried to set alignment head on text layer 6, but model only has 4 text layers")
	default:
		for _, file := range files {
			fmt.Fprintf(os.Stderr, "read_audio_data: reading audio data from '%s' ...\n", file)
			fmt.Printf("[00:00:00.200 --> 00:00:01.000]  %s\n", filepath.Base(file))
			fmt.Fprintf(os.Stderr, "output_json: saving output to '%s.json'\n", file)
			says := cmp.Or(f.Says, filepath.Base(file))
			if strings.HasPrefix(filepath.Base(file), "novoice-") {
				says = f.NoVoice
			}
			heard := fmt.Sprintf(`{"transcription":[{"offsets":{"from":200,"to":1000},"text":%q}]}`, says)
			if timed {
				heard = `{"transcription":[{"offsets":{"from":0,"to":3800},"text":"Hi! Bye","tokens":[{"text":"[_BEG_]","t_dtw":-1},{"text":"Hi","t_dtw":40},{"text":"!","t_dtw":60},{"text":" Bye","t_dtw":360},{"text":"[_TT_190]","t_dtw":-1}]}]}`
			}
			if err := os.WriteFile(file+".json", []byte(heard), 0o644); err != nil {
				return err
			}
		}
	}
	return nil
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
