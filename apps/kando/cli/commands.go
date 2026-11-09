package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"kando/apps/kando/mcpserver"
	"kando/apps/kando/remote"
	"kando/packages/core"
)

// flags is the command's flag set, with --json for a command that talks to the app. Its errors are returned to the command, which reports them as usage errors.
func (e *env) flags() *flag.FlagSet {
	fs := flag.NewFlagSet("kando "+e.cmd.name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if e.cmd.app {
		fs.BoolVar(&e.json, "json", false, "print the app's answer as JSON")
	}
	e.fs = fs
	return fs
}

// parse parses args with fs and returns the positional arguments. Flags may come before, between or after them, as in `kando add <link> --wait`; "--" ends the flags, and everything after it is positional.
func (e *env) parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil, err
			}
			return nil, usagef("%v", err)
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return positional, nil
		}
		// Parse stops at the first positional argument, or just after "--", which it consumes.
		if consumed := len(args) - len(rest); consumed > 0 && args[consumed-1] == "--" {
			return append(positional, rest...), nil
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
}

// answer asks the app for method with params. With --json it prints the answer as it is and reports done; otherwise it decodes it into out, which may be nil, for the command to print.
func (e *env) answer(method string, params, out any) (done bool, err error) {
	var raw json.RawMessage
	if err := e.call(method, params, &raw); err != nil {
		return false, err
	}
	if e.json {
		return true, writeJSON(e.stdout, raw)
	}
	if out == nil || len(raw) == 0 {
		return false, nil
	}
	return false, json.Unmarshal(raw, out)
}

func runStatus(e *env, args []string) error {
	positional, err := e.parse(e.flags(), args)
	if err != nil {
		return err
	}
	if len(positional) > 0 {
		return usagef("takes no arguments")
	}
	var status remote.Status
	if done, err := e.answer(remote.MethodStatus, nil, &status); done || err != nil {
		return err
	}
	return writeStatus(e.stdout, status)
}

func runList(e *env, args []string) error {
	fs := e.flags()
	source := fs.String("source", "", "only the films from this source: youtube, bilibili, local or another site's name")
	positional, err := e.parse(fs, args)
	if err != nil {
		return err
	}
	var films []remote.Film
	params := remote.FilmsParams{Source: *source, Query: strings.Join(positional, " ")}
	if done, err := e.answer(remote.MethodFilms, params, &films); done || err != nil {
		return err
	}
	if len(films) == 0 {
		fmt.Fprintln(e.stderr, "No films.")
		return nil
	}
	return writeFilms(e.stdout, films)
}

func runShow(e *env, args []string) error {
	id, err := e.oneID(args, "a film id")
	if err != nil {
		return err
	}
	var film remote.Film
	if done, err := e.answer(remote.MethodFilm, remote.IDParams{ID: id}, &film); done || err != nil {
		return err
	}
	return writeFilm(e.stdout, film)
}

// oneID parses a command that takes exactly one id, what names what it is in the usage error.
func (e *env) oneID(args []string, what string) (string, error) {
	positional, err := e.parse(e.flags(), args)
	if err != nil {
		return "", err
	}
	switch len(positional) {
	case 0:
		return "", usagef("needs %s", what)
	case 1:
		return positional[0], nil
	default:
		return "", usagef("takes one id, got %d arguments", len(positional))
	}
}

func runSubtitles(e *env, args []string) error {
	fs := e.flags()
	format := fs.String("format", "srt", "srt, text or json")
	original := fs.Bool("original", false, "only the original speech")
	translation := fs.Bool("translation", false, "only the translation")
	positional, err := e.parse(fs, args)
	if err != nil {
		return err
	}
	switch {
	case len(positional) == 0:
		return usagef("needs a film id")
	case len(positional) > 1:
		return usagef("takes one film id, got %d arguments", len(positional))
	}
	switch *format {
	case "srt", "text":
	case "json":
		e.json = true
	default:
		return usagef("unknown format %q: use srt, text or json", *format)
	}
	side := sideBoth
	switch {
	case *original && *translation:
		return usagef("--original and --translation each leave the other out; leave both out for both")
	case *original:
		side = sideOriginal
	case *translation:
		side = sideTranslation
	}
	// The cues are decoded and printed here rather than by answer, as --original and --translation pick from them in JSON too.
	var cues []remote.Cue
	if err := e.call(remote.MethodSubtitles, remote.IDParams{ID: positional[0]}, &cues); err != nil {
		return err
	}
	switch {
	case e.json:
		data, err := json.Marshal(pickCues(cues, side))
		if err != nil {
			return err
		}
		return writeJSON(e.stdout, data)
	case *format == "text":
		return writeCuesText(e.stdout, cues, side)
	}
	return writeSRT(e.stdout, cues, side)
}

func runJobs(e *env, args []string) error {
	positional, err := e.parse(e.flags(), args)
	if err != nil {
		return err
	}
	if len(positional) > 0 {
		return usagef("takes no arguments")
	}
	var jobs []remote.Job
	if done, err := e.answer(remote.MethodJobs, nil, &jobs); done || err != nil {
		return err
	}
	if len(jobs) == 0 {
		fmt.Fprintln(e.stderr, "No jobs.")
		return nil
	}
	return writeJobs(e.stdout, jobs)
}

// pollInterval is how often `kando add --wait` asks the app how its jobs are going. Tests shorten it.
var pollInterval = time.Second

func runAdd(e *env, args []string) error {
	fs := e.flags()
	wait := fs.Bool("wait", false, "wait until every added job is complete or failed, printing its progress")
	sources, err := e.parse(fs, args)
	if err != nil {
		return err
	}
	if len(sources) == 0 {
		return usagef("needs a link or a file")
	}
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	var raw json.RawMessage
	if err := e.call(remote.MethodAdd, remote.AddParams{Sources: sources, Dir: dir}, &raw); err != nil {
		return err
	}
	var result remote.AddResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return err
	}
	if !e.json {
		for _, job := range result.Jobs {
			fmt.Fprintf(e.stdout, "%s\t%s\n", printable(job.ID), printable(job.Source))
		}
	}
	for _, skipped := range result.Skipped {
		fmt.Fprintf(e.stderr, "kando add: skipped %s: %s\n", printable(skipped.Source), printable(skipped.Reason))
	}
	failed := len(result.Skipped) > 0
	if *wait && len(result.Jobs) > 0 {
		// With --json, standard output is kept for the one JSON answer, the added jobs as they ended, and the progress goes to standard error.
		progress := e.stdout
		if e.json {
			progress = e.stderr
		}
		final, err := e.waitJobs(result.Jobs, progress)
		if err != nil {
			return err
		}
		for i, job := range final {
			if job.Stage != string(core.StageComplete) {
				failed = true
			}
			result.Jobs[i] = job
		}
		if raw, err = json.Marshal(result); err != nil {
			return err
		}
	}
	// With --json the answer is printed whatever happened to the sources, even when every one was skipped and there was nothing to wait for, so a script always has one JSON value to read.
	if e.json {
		if err := writeJSON(e.stdout, raw); err != nil {
			return err
		}
	}
	if failed {
		return errReported
	}
	return nil
}

// waitJobs asks the app for its jobs every pollInterval until each of jobs is complete or failed, writing a line to w whenever one's stage or progress changes, and returns them as they ended, in the same order. A job that leaves the queue before it ends, as a waiting one canceled in the window does, counts as failed.
func (e *env) waitJobs(jobs []remote.Job, w io.Writer) ([]remote.Job, error) {
	final := make([]remote.Job, len(jobs))
	ended := make([]bool, len(jobs))
	shown := make([]string, len(jobs))
	for {
		var current []remote.Job
		if err := e.call(remote.MethodJobs, nil, &current); err != nil {
			return nil, err
		}
		byID := make(map[string]remote.Job, len(current))
		for _, job := range current {
			byID[job.ID] = job
		}
		left := 0
		for i, added := range jobs {
			if ended[i] {
				continue
			}
			job, ok := byID[added.ID]
			if !ok {
				job = added
				job.Stage = string(core.StageFailed)
				job.Progress = 0
				job.Error = "it is no longer in the queue"
			}
			if line := printable(strings.Join(jobFields(job), "  ")); line != shown[i] {
				fmt.Fprintln(w, line)
				shown[i] = line
			}
			switch core.Stage(job.Stage) {
			case core.StageComplete, core.StageFailed:
				ended[i] = true
				final[i] = job
			default:
				left++
			}
		}
		if left == 0 {
			return final, nil
		}
		select {
		case <-e.ctx.Done():
			return nil, e.ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

func runCancel(e *env, args []string) error {
	id, err := e.oneID(args, "a job id")
	if err != nil {
		return err
	}
	var job remote.Job
	if done, err := e.answer(remote.MethodCancel, remote.IDParams{ID: id}, &job); done || err != nil {
		return err
	}
	return writeJobs(e.stdout, []remote.Job{job})
}

func runSites(e *env, args []string) error {
	fs := e.flags()
	limit := fs.Int("limit", 0, "at most this many sites, 0 for all")
	positional, err := e.parse(fs, args)
	if err != nil {
		return err
	}
	if *limit < 0 {
		return usagef("--limit must not be negative")
	}
	var sites []remote.Site
	params := remote.SitesParams{Query: strings.Join(positional, " "), Limit: *limit}
	if done, err := e.answer(remote.MethodSites, params, &sites); done || err != nil {
		return err
	}
	if len(sites) == 0 {
		fmt.Fprintln(e.stderr, "No sites found.")
		return nil
	}
	return writeSites(e.stdout, sites)
}

func runOpen(e *env, args []string) error {
	positional, err := e.parse(e.flags(), args)
	if err != nil {
		return err
	}
	if len(positional) > 1 {
		return usagef("takes at most one film id, got %d arguments", len(positional))
	}
	id := ""
	if len(positional) == 1 {
		id = positional[0]
	}
	_, err = e.answer(remote.MethodShow, remote.IDParams{ID: id}, nil)
	return err
}

func runMCP(e *env, args []string) error {
	positional, err := e.parse(e.flags(), args)
	if err != nil {
		return err
	}
	if len(positional) > 0 {
		return usagef("takes no arguments")
	}
	return mcpserver.Run(e.ctx, e.version)
}

func runVersion(e *env, args []string) error {
	positional, err := e.parse(e.flags(), args)
	if err != nil {
		return err
	}
	if len(positional) > 0 {
		return usagef("takes no arguments")
	}
	fmt.Fprintf(e.stdout, "kando %s\n", e.version)
	return nil
}

func runHelp(e *env, args []string) error {
	positional, err := e.parse(e.flags(), args)
	if err != nil {
		return err
	}
	switch len(positional) {
	case 0:
		writeUsage(e.stdout)
		return nil
	case 1:
		cmd := lookup(positional[0])
		if cmd == nil {
			return usagef("unknown command %q", positional[0])
		}
		commandHelp(e.ctx, cmd, e.stdout)
		return nil
	default:
		return usagef("takes at most one command")
	}
}
