// Package cli is the kando command line: the app's executable run with one of its commands drives the running app through its socket (remote), starting it when it is not running, and `kando mcp` serves MCP (mcpserver) on standard input and output.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"kando/apps/kando/remote"
)

// Main runs the command args name and returns its exit code, or ok false when args are what the app is opened with (OpensWindow), and the app then starts its window. Anything else is the command line's, so a mistyped command or a flag it does not know is an error in the terminal rather than the window opening.
func Main(args []string, version string) (code int, ok bool) {
	if OpensWindow(args) {
		return 0, false
	}
	// On Windows the app is a GUI program, which starts without the terminal's standard output; this hands it back.
	attachConsole(args)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return Run(ctx, args, os.Stdout, os.Stderr, version), true
}

// OpensWindow reports whether args are ones the app is opened with rather than a command: no arguments; the -psn_, -NS and -Apple arguments macOS passes; a link with a scheme, such as a deep link; or the path of an existing file or folder the app is asked to open. A command's name wins over a file of the same name in the current folder.
func OpensWindow(args []string) bool {
	if len(args) == 0 {
		return true
	}
	first := args[0]
	if lookup(first) != nil {
		return false
	}
	for _, prefix := range []string{"-psn_", "-NS", "-Apple"} {
		if strings.HasPrefix(first, prefix) {
			return true
		}
	}
	if strings.Contains(first, "://") {
		return true
	}
	if strings.HasPrefix(first, "-") {
		return false
	}
	_, err := os.Stat(first)
	return err == nil
}

// The exit codes of Run.
const (
	exitOK    = 0
	exitFail  = 1
	exitUsage = 2
)

// Run runs the command args name, writing its output to stdout and its errors to stderr, and returns the exit code: 0 when it succeeded, 1 when it failed, 2 when it was used wrongly. Commands that need the app connect to it at remote.SocketPath, starting it when it is not running.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer, version string) int {
	if len(args) == 0 {
		writeUsage(stderr)
		return exitUsage
	}
	// --json may come before the command, as in `kando --json status`: it is moved after the command's name, where the command reads it, and is a usage error there for a command that does not take it.
	var lead []string
	for len(args) > 0 && isJSONFlag(args[0]) {
		lead = append(lead, args[0])
		args = args[1:]
	}
	if len(args) == 0 {
		fmt.Fprintf(stderr, "kando: %s needs a command after it\n\n", lead[0])
		writeUsage(stderr)
		return exitUsage
	}
	if len(lead) > 0 {
		args = append(append([]string{args[0]}, lead...), args[1:]...)
	}
	name := args[0]
	switch name {
	case "-h", "-help", "--help":
		name = "help"
	case "--version":
		name = "version"
	}
	cmd := lookup(name)
	if cmd == nil {
		what := "command"
		if strings.HasPrefix(name, "-") {
			what = "flag"
		}
		fmt.Fprintf(stderr, "kando: unknown %s %q\n\n", what, name)
		writeUsage(stderr)
		return exitUsage
	}
	e := &env{ctx: ctx, cmd: cmd, stdout: stdout, stderr: stderr, version: version}
	defer e.close()
	err := cmd.run(e, args[1:])
	return e.exit(err)
}

// isJSONFlag reports whether arg is the --json flag, in any of the spellings the flag package reads: one or two dashes, and with =value.
func isJSONFlag(arg string) bool {
	name := strings.TrimPrefix(strings.TrimPrefix(arg, "-"), "-")
	if name == arg {
		return false
	}
	name, _, _ = strings.Cut(name, "=")
	return name == "json"
}

// command is one of the command line's commands.
type command struct {
	name string
	// synopsis is what follows the name in its usage line, summary the line `kando help` lists it with.
	synopsis string
	summary  string
	// app is set for a command that talks to the app, which takes --json.
	app bool
	run func(e *env, args []string) error
}

// commands is the command line, in the order `kando help` lists it. It is filled in by init, as help reads it.
var commands []*command

func init() {
	commands = []*command{
		{name: "status", summary: "Show the library, the programs and models in use, and whether a job can start", app: true, run: runStatus},
		{name: "list", synopsis: "[--source S] [query]", summary: "List the library's films, newest first", app: true, run: runList},
		{name: "show", synopsis: "<film-id>", summary: "Show one film", app: true, run: runShow},
		{name: "subtitles", synopsis: "<film-id> [--format srt|text|json] [--original|--translation]", summary: "Print a film's subtitles", app: true, run: runSubtitles},
		{name: "jobs", summary: "List the jobs: the running one, the waiting ones, then the finished ones", app: true, run: runJobs},
		{name: "add", synopsis: "<link-or-file>... [--wait]", summary: "Queue videos to download, transcribe and translate", app: true, run: runAdd},
		{name: "cancel", synopsis: "<job-id>", summary: "Stop a running job or take a waiting one out of the queue", app: true, run: runCancel},
		{name: "retry", synopsis: "<job-id> [--wait]", summary: "Queue a failed, canceled or finished job again", app: true, run: runRetry},
		{name: "remove", synopsis: "<job-id>", summary: "Take a job off the library, stopping it if it runs; its files stay", app: true, run: runRemove},
		{name: "sites", synopsis: "[query] [--limit N]", summary: "Search the websites Kando downloads from", app: true, run: runSites},
		{name: "open", synopsis: "[film-id]", summary: "Bring Kando to the front, showing a film or the library", app: true, run: runOpen},
		{name: "mcp", summary: "Serve MCP on standard input and output for AI agents", run: runMCP},
		{name: "version", summary: "Print the version", run: runVersion},
		{name: "help", synopsis: "[command]", summary: "Show this help, or a command's", run: runHelp},
	}
}

func lookup(name string) *command {
	for _, c := range commands {
		if c.name == name {
			return c
		}
	}
	return nil
}

// usageLine is the command's usage, as `kando help <command>` and its usage errors show it.
func (c *command) usageLine() string {
	line := "kando " + c.name
	if c.synopsis != "" {
		line += " " + c.synopsis
	}
	if c.app {
		line += " [--json]"
	}
	return line
}

func writeUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: kando <command> [arguments]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Kando run with a command drives the Kando app, starting it when it is not running. Run with no command, it opens its window.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	for _, c := range commands {
		fmt.Fprintf(w, "  %-10s %s\n", c.name, c.summary)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Every command that talks to the app takes --json to print the app's answer as JSON.")
	fmt.Fprintln(w, "Run 'kando help <command>' for a command's arguments.")
}

// env is one run of a command: where it writes, and its connection to the app, made on its first call.
type env struct {
	ctx     context.Context
	cmd     *command
	stdout  io.Writer
	stderr  io.Writer
	version string
	// json is the command's --json flag.
	json bool
	// fs is the command's flag set once it has made it, which its help lists.
	fs     *flag.FlagSet
	client *remote.Client
	// stopClose ends the watch that closes the connection when ctx ends.
	stopClose func() bool
}

// usageError is a command used wrongly: exit code 2, with the command's usage.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

func usagef(format string, args ...any) error {
	return &usageError{msg: fmt.Sprintf(format, args...)}
}

// errReported is a failure the command has already told the user about: exit code 1 and nothing more.
var errReported = errors.New("failed")

// exit reports err on stderr and turns it into the exit code.
func (e *env) exit(err error) int {
	var usage *usageError
	var remoteErr *remote.Error
	switch {
	case err == nil:
		return exitOK
	case errors.Is(err, flag.ErrHelp):
		e.writeCommandUsage(e.stdout)
		return exitOK
	case errors.As(err, &usage):
		fmt.Fprintf(e.stderr, "kando %s: %s\nUsage: %s\n", e.cmd.name, printable(usage.msg), e.cmd.usageLine())
		return exitUsage
	case errors.Is(err, errReported):
		return exitFail
	case e.ctx.Err() != nil:
		fmt.Fprintf(e.stderr, "kando %s: interrupted\n", e.cmd.name)
		return exitFail
	case errors.As(err, &remoteErr):
		// The app's messages are written for the user, not_ready's above all, so they are shown without the code.
		fmt.Fprintf(e.stderr, "kando %s: %s\n", e.cmd.name, printable(remoteErr.Message))
		return exitFail
	default:
		fmt.Fprintf(e.stderr, "kando %s: %s\n", e.cmd.name, printable(err.Error()))
		return exitFail
	}
}

// writeCommandUsage writes the command's help: its usage line, what it does and its flags, as `kando help <command>` and `kando <command> -h` show it.
func (e *env) writeCommandUsage(w io.Writer) {
	fmt.Fprintf(w, "Usage: %s\n\n%s.\n", e.cmd.usageLine(), e.cmd.summary)
	if e.fs == nil {
		return
	}
	var rows [][]string
	e.fs.VisitAll(func(f *flag.Flag) {
		kind, usage := flag.UnquoteUsage(f)
		name := "--" + f.Name
		if kind != "" {
			name += " " + kind
		}
		switch f.DefValue {
		case "", "false", "0":
		default:
			usage += fmt.Sprintf(" (default %s)", f.DefValue)
		}
		rows = append(rows, []string{"  " + name, usage})
	})
	if len(rows) == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Flags:")
	table(w, rows)
}

// commandHelp writes cmd's help to w. The flag set is the command's own, made by running it with -h: every command parses its flags before it does anything else, and -h stops it there.
func commandHelp(ctx context.Context, cmd *command, w io.Writer) {
	e := &env{ctx: ctx, cmd: cmd, stdout: io.Discard, stderr: io.Discard}
	_ = cmd.run(e, []string{"-h"})
	e.writeCommandUsage(w)
}

// call asks the app for method with params and decodes its answer into out, connecting first when this run has not yet. When the app is not running it is started, and "Starting Kando…" goes to stderr while it opens its socket.
func (e *env) call(method string, params, out any) error {
	if e.client == nil {
		client, err := remote.Dial(e.ctx, false)
		if errors.Is(err, remote.ErrNotRunning) {
			fmt.Fprintln(e.stderr, "Starting Kando…")
			client, err = remote.Dial(e.ctx, true)
		}
		if err != nil {
			return err
		}
		e.client = client
		// Ctrl-C ends the context; closing the connection then ends a call that waits for its answer.
		e.stopClose = context.AfterFunc(e.ctx, func() { client.Close() })
	}
	return e.client.Call(e.ctx, method, params, out)
}

func (e *env) close() {
	if e.client == nil {
		return
	}
	e.stopClose()
	e.client.Close()
}
