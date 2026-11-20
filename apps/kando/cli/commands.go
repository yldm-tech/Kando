package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
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
	language := fs.String("language", "", "only the films heard in this language, by its code, as ja or en")
	positional, err := e.parse(fs, args)
	if err != nil {
		return err
	}
	var films []remote.Film
	params := remote.FilmsParams{Source: *source, Query: strings.Join(positional, " "), Spoken: *language}
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
	from := fs.String("from", "", "only the lines that start at this time or later: seconds, mm:ss or hh:mm:ss")
	to := fs.String("to", "", "only the lines that start before this time")
	until := fs.String("until", "", "watched: only the lines up to how far the film was played, giving nothing away of the rest")
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
	which := remote.WhichBoth
	switch {
	case *original && *translation:
		return usagef("--original and --translation each leave the other out; leave both out for both")
	case *original:
		which = remote.WhichOriginal
	case *translation:
		which = remote.WhichTranslation
	}
	params := remote.SubtitlesParams{ID: positional[0]}
	if params.FromMS, err = clockFlag("from", *from); err != nil {
		return err
	}
	if params.ToMS, err = clockFlag("to", *to); err != nil {
		return err
	}
	if params.Until, err = untilFlag(*until); err != nil {
		return err
	}
	// The cues are decoded and printed here rather than by answer, as --original and --translation pick from them in JSON too.
	var cues []remote.Cue
	if err := e.call(remote.MethodSubtitles, params, &cues); err != nil {
		return err
	}
	picked := remote.PickCues(cues, which)
	switch {
	case e.json:
		data, err := json.Marshal(picked)
		if err != nil {
			return err
		}
		return writeJSON(e.stdout, data)
	case *format == "text":
		_, err := io.WriteString(e.stdout, printableLines(remote.CuesText(picked)))
		return err
	}
	_, err = io.WriteString(e.stdout, printableLines(remote.CuesSRT(picked)))
	return err
}

// clockFlag is the time the flag name was given as, in milliseconds, 0 when it was not given.
func clockFlag(name, value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	ms, err := remote.ParseClock(value)
	if err != nil {
		return 0, usagef("--%s: %v", name, err)
	}
	return ms, nil
}

// untilFlag checks --until, which takes only watched.
func untilFlag(value string) (string, error) {
	if value != "" && value != remote.UntilWatched {
		return "", usagef("--until takes %q, got %q", remote.UntilWatched, value)
	}
	return value, nil
}

func runSearch(e *env, args []string) error {
	fs := e.flags()
	film := fs.String("film", "", "only this film's lines, by its id")
	until := fs.String("until", "", "watched: only the lines up to how far each film was played")
	limit := fs.Int("limit", 0, "at most this many lines, 0 for all")
	positional, err := e.parse(fs, args)
	if err != nil {
		return err
	}
	query := strings.TrimSpace(strings.Join(positional, " "))
	if query == "" {
		return usagef("needs words to search for")
	}
	if *limit < 0 {
		return usagef("--limit must not be negative")
	}
	params := remote.SearchParams{Query: query, FilmID: *film, Limit: *limit}
	if params.Until, err = untilFlag(*until); err != nil {
		return err
	}
	var found []remote.Line
	if done, err := e.answer(remote.MethodSearch, params, &found); done || err != nil {
		return err
	}
	if len(found) == 0 {
		fmt.Fprintln(e.stderr, "No lines found.")
		return nil
	}
	return writeLines(e.stdout, found)
}

func runJobs(e *env, args []string) error {
	positional, err := e.parse(e.flags(), args)
	if err != nil {
		return err
	}
	switch len(positional) {
	case 0:
	case 1:
		var job remote.Job
		if done, err := e.answer(remote.MethodJob, remote.IDParams{ID: positional[0]}, &job); done || err != nil {
			return err
		}
		return writeJobs(e.stdout, []remote.Job{job})
	default:
		return usagef("takes at most one job id, got %d arguments", len(positional))
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
	yes := fs.Bool("yes", false, "add the videos of playlist and channel links and of folders without asking")
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
	sources, listFailed, err := e.expandLists(sources, *yes)
	if err != nil {
		return err
	}
	var raw json.RawMessage
	result := remote.AddResult{Jobs: []remote.Job{}}
	// Every video of the lists may be in the library already, which leaves nothing to ask the app to add.
	if len(sources) > 0 {
		if err := e.call(remote.MethodAdd, remote.AddParams{Sources: sources, Dir: dir}, &raw); err != nil {
			return err
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			return err
		}
	} else if raw, err = json.Marshal(result); err != nil {
		return err
	}
	// A video the library already has is not added again, but its job is the one asked for: it counts as added, is printed as an added job is, and is waited for with --wait while it is still in the queue, so running the same command again succeeds. One whose job failed and could not be queued again is a failure, as any other source skipped.
	failed := false
	var dups, pending []int
	for i, skipped := range result.Skipped {
		if skipped.JobID != "" && skipped.Stage != string(core.StageFailed) {
			dups = append(dups, i)
			if skipped.Stage != string(core.StageComplete) {
				pending = append(pending, i)
			}
			fmt.Fprintf(e.stderr, "kando add: %s: %s\n", printable(skipped.Source), printable(skipped.Reason))
			continue
		}
		failed = true
		fmt.Fprintf(e.stderr, "kando add: skipped %s: %s\n", printable(skipped.Source), printable(skipped.Reason))
	}
	if result.Note != "" {
		fmt.Fprintf(e.stderr, "kando add: %s\n", printable(result.Note))
	}
	if !e.json {
		for _, job := range result.Jobs {
			fmt.Fprintf(e.stdout, "%s\t%s\n", printable(job.ID), printable(job.Source))
		}
		for _, i := range dups {
			fmt.Fprintf(e.stdout, "%s\t%s\n", printable(result.Skipped[i].JobID), printable(result.Skipped[i].Source))
		}
	}
	failed = failed || listFailed
	if *wait && len(result.Jobs)+len(pending) > 0 {
		// With --json, standard output is kept for the one JSON answer, the added jobs as they ended, and the progress goes to standard error.
		progress := e.stdout
		if e.json {
			progress = e.stderr
		}
		waiting := slices.Clone(result.Jobs)
		for _, i := range pending {
			waiting = append(waiting, remote.Job{ID: result.Skipped[i].JobID, Source: result.Skipped[i].Source, Stage: result.Skipped[i].Stage})
		}
		final, err := e.waitJobs(waiting, progress)
		if err != nil {
			return err
		}
		for i, job := range final {
			if !jobDone(job) {
				failed = true
			}
			if i < len(result.Jobs) {
				result.Jobs[i] = job
				continue
			}
			// A video already in the queue ends as its job ended.
			skipped := &result.Skipped[pending[i-len(result.Jobs)]]
			skipped.Stage, skipped.FilmID = job.Stage, job.FilmID
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

// stdin is where kando add reads the answer when it asks before adding the videos of a playlist or a channel, and stdinTerminal reports whether a person is there to answer, which a script piping into it is not. Tests replace them.
var (
	stdin         io.Reader = os.Stdin
	stdinTerminal           = func() bool {
		info, err := os.Stdin.Stat()
		return err == nil && info.Mode()&os.ModeCharDevice != 0
	}
)

// expandLists puts the videos of the playlist and channel links and of the folders among sources in their place, in order, as the app lists them (remote.MethodLists): a playlist whole, a channel's most recent, a folder's as dropping it on the window adds them, without those the library has already, which it would skip. It says on standard error what each list holds, and asks on standard input before it adds them unless yes is set; without a terminal to ask on, it is a usage error that says to pass --yes. A list the app could not read, or a folder without a video, is skipped as a source jobs.add skips is, and failed reports it.
func (e *env) expandLists(sources []string, yes bool) (out []string, failed bool, err error) {
	var lists []string
	for _, s := range sources {
		if list, ok := listSource(s); ok {
			lists = append(lists, list)
		}
	}
	if len(lists) == 0 {
		return sources, false, nil
	}
	var answer []remote.List
	if err := e.call(remote.MethodLists, remote.ListsParams{Sources: lists}, &answer); err != nil {
		return nil, false, err
	}
	if len(answer) != len(lists) {
		return nil, false, fmt.Errorf("the app listed %d of %d playlists and channels", len(answer), len(lists))
	}
	videos := map[string][]string{}
	total := 0
	for i, list := range answer {
		if list.Error != "" {
			fmt.Fprintf(e.stderr, "kando add: skipped %s: %s\n", printable(list.Source), printable(list.Error))
			failed = true
			continue
		}
		var fresh []string
		for _, v := range list.Videos {
			if !v.InLibrary {
				fresh = append(fresh, v.URL)
			}
		}
		videos[lists[i]] = fresh
		total += len(fresh)
		fmt.Fprintln(e.stderr, printable(listLine(list, len(list.Videos)-len(fresh))))
	}
	if total > 0 && !yes {
		if !stdinTerminal() {
			return nil, false, usagef("the playlists, channels and folders hold %s to add; pass --yes to add them without asking", videoCount(total))
		}
		fmt.Fprintf(e.stderr, "Add %s? [y/N] ", videoCount(total))
		line, _ := bufio.NewReader(stdin).ReadString('\n')
		if answer := strings.ToLower(strings.TrimSpace(line)); answer != "y" && answer != "yes" {
			fmt.Fprintln(e.stderr, "kando add: nothing added")
			return nil, false, errReported
		}
	}
	for _, s := range sources {
		if list, ok := listSource(s); ok {
			out = append(out, videos[list]...)
			continue
		}
		out = append(out, s)
	}
	return out, failed, nil
}

// listSource is what kando add asks the app to list for s, and whether s is a list at all: a playlist or a channel link as it is, or a folder, given as a path or a file:// URL, as its absolute path, as the app runs in another directory.
func listSource(s string) (string, bool) {
	if isList(s) {
		return s, true
	}
	path := s
	if strings.HasPrefix(strings.ToLower(s), "file:") {
		u, err := url.Parse(s)
		if err != nil {
			return "", false
		}
		local, ok := core.FilePath(u)
		if !ok {
			return "", false
		}
		path = local
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		return "", false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	return abs, true
}

// isList reports whether s is a playlist or a channel link (core.Link.IsList), whose videos kando add lists and adds rather than the link itself.
func isList(s string) bool {
	link, ok := core.ParseLink(strings.TrimSpace(s))
	return ok && link.IsList()
}

// listLine says what a list holds: its name, how many videos, a channel's most recent only, how many files of a folder are not videos, and how many of the videos the library has already.
func listLine(list remote.List, have int) string {
	name := list.Title
	if name == "" {
		name = list.Source
	}
	what := "playlist"
	switch list.Kind {
	case string(core.LinkChannel):
		what = fmt.Sprintf("channel, its %d most recent", list.Limit)
	case remote.ListFolder:
		what = "folder"
	}
	line := fmt.Sprintf("%s: %s (%s)", name, videoCount(len(list.Videos)), what)
	switch {
	case list.NotVideos == 1:
		line += ", 1 file that is not a video left out"
	case list.NotVideos > 1:
		line += fmt.Sprintf(", %d files that are not videos left out", list.NotVideos)
	}
	if have > 0 {
		line += fmt.Sprintf(", %d already in the library", have)
	}
	return line
}

// videoCount is n videos in words: "1 video", "54 videos".
func videoCount(n int) string {
	if n == 1 {
		return "1 video"
	}
	return fmt.Sprintf("%d videos", n)
}

// waitJobs asks the app for its jobs every pollInterval until each of jobs is complete or failed, writing a line to w whenever one's stage or progress changes, and returns them as they ended, in the same order. A job jobs.list no longer lists is asked for alone, as one making the subtitles of a video of the library's folder leaves the list when it fails; one the app no longer has, as a waiting one removed in the window, counts as failed.
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
				var err error
				if job, err = e.gone(added); err != nil {
					return nil, err
				}
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

// gone is job as the app answers it alone once jobs.list no longer lists it, or, when the app no longer has it, job failed, as it is no longer in the queue.
func (e *env) gone(job remote.Job) (remote.Job, error) {
	var alone remote.Job
	err := e.call(remote.MethodJob, remote.IDParams{ID: job.ID}, &alone)
	switch {
	case err == nil:
		return alone, nil
	case !remote.IsCode(err, remote.CodeNotFound) && !remote.IsCode(err, remote.CodeUnknownMethod):
		return remote.Job{}, err
	}
	job.Stage = string(core.StageFailed)
	job.Progress = 0
	job.Error = "it is no longer in the queue"
	return job, nil
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

func runRetry(e *env, args []string) error {
	return e.requeue(args, remote.MethodRetry, "a job id")
}

func runTranslate(e *env, args []string) error {
	return e.requeue(args, remote.MethodTranslate, "a film id")
}

// requeue asks the app for method, which queues a job again for the one id in args, what naming what that id is, and prints the job; with --wait it waits for it as add --wait does, and fails unless it ends complete with its film translated (jobDone).
func (e *env) requeue(args []string, method, what string) error {
	fs := e.flags()
	wait := fs.Bool("wait", false, "wait until the job is complete or failed, printing its progress")
	positional, err := e.parse(fs, args)
	if err != nil {
		return err
	}
	switch len(positional) {
	case 0:
		return usagef("needs %s", what)
	case 1:
	default:
		return usagef("takes one id, got %d arguments", len(positional))
	}
	if !*wait {
		var job remote.Job
		if done, err := e.answer(method, remote.IDParams{ID: positional[0]}, &job); done || err != nil {
			return err
		}
		return writeJobs(e.stdout, []remote.Job{job})
	}
	var job remote.Job
	if err := e.call(method, remote.IDParams{ID: positional[0]}, &job); err != nil {
		return err
	}
	// As add --wait does: with --json, standard output is kept for the one JSON answer, the job as it ended, and the progress goes to standard error.
	progress := e.stdout
	if e.json {
		progress = e.stderr
	}
	final, err := e.waitJobs([]remote.Job{job}, progress)
	if err != nil {
		return err
	}
	if e.json {
		raw, err := json.Marshal(final[0])
		if err != nil {
			return err
		}
		if err := writeJSON(e.stdout, raw); err != nil {
			return err
		}
	}
	if !jobDone(final[0]) {
		return errReported
	}
	return nil
}

// jobDone reports whether a job --wait waited for did what it was asked: it is complete and its film was translated. One that completed without translating its film, as when the AI service refused its key, says why (remote.Job.TranslationError), and is a failure.
func jobDone(j remote.Job) bool {
	return j.Stage == string(core.StageComplete) && j.TranslationError == ""
}

func runRemove(e *env, args []string) error {
	id, err := e.oneID(args, "a job id")
	if err != nil {
		return err
	}
	_, err = e.answer(remote.MethodRemove, remote.IDParams{ID: id}, nil)
	return err
}

func runMove(e *env, args []string) error {
	fs := e.flags()
	to := fs.String("to", "", "the library to move the film to: its name, as kando status lists it, or its folder")
	positional, err := e.parse(fs, args)
	if err != nil {
		return err
	}
	switch {
	case len(positional) == 0:
		return usagef("needs a film id")
	case len(positional) > 1:
		return usagef("takes one film id, got %d arguments", len(positional))
	case strings.TrimSpace(*to) == "":
		return usagef("needs --to and the library to move the film to")
	}
	// A library named by its folder is sent as its absolute path, as the app knows its libraries by theirs.
	library := *to
	if info, err := os.Stat(library); err == nil && info.IsDir() {
		if abs, err := filepath.Abs(library); err == nil {
			library = abs
		}
	}
	var moved remote.Moved
	if done, err := e.answer(remote.MethodMove, remote.MoveParams{ID: positional[0], To: library}, &moved); done || err != nil {
		return err
	}
	_, err = fmt.Fprintf(e.stdout, "%s\t%s\n", printable(moved.ID), printable(moved.Library))
	return err
}

// runSubscriptions lists the open library's subscriptions, or with add subscribes it to a list, or with check lists a subscription's videos now.
func runSubscriptions(e *env, args []string) error {
	positional, err := e.parse(e.flags(), args)
	if err != nil {
		return err
	}
	verb := "list"
	if len(positional) > 0 {
		verb, positional = positional[0], positional[1:]
	}
	switch verb {
	case "list":
		if len(positional) > 0 {
			return usagef("list takes no arguments, got %d", len(positional))
		}
		var subs []remote.Subscription
		if done, err := e.answer(remote.MethodSubscriptions, nil, &subs); done || err != nil {
			return err
		}
		if len(subs) == 0 {
			fmt.Fprintln(e.stderr, "No subscriptions.")
			return nil
		}
		return writeSubscriptions(e.stdout, subs)
	case "add":
		if len(positional) != 1 {
			return usagef("add takes one channel, collection or playlist link, got %d arguments", len(positional))
		}
		var got remote.Subscribed
		if done, err := e.answer(remote.MethodSubscribe, remote.SubscribeParams{Source: positional[0]}, &got); done || err != nil {
			return err
		}
		if got.Already {
			fmt.Fprintln(e.stderr, "The library follows it already.")
		}
		if !got.Already && got.Subscription.Added > 0 {
			fmt.Fprintf(e.stderr, "Queued %d of its videos.\n", got.Subscription.Added)
		}
		_, err := fmt.Fprintf(e.stdout, "%s\t%s\n", printable(got.Subscription.ID), printable(got.Subscription.Name))
		return err
	case "check":
		if len(positional) != 1 {
			return usagef("check takes one subscription id, got %d arguments", len(positional))
		}
		var got remote.SubscriptionChecked
		if done, err := e.answer(remote.MethodSubscriptionCheck, remote.IDParams{ID: positional[0]}, &got); done || err != nil {
			return err
		}
		return writeChecked(e.stdout, got)
	}
	return usagef("unknown subcommand %q: use list, add or check", verb)
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
	fs := e.flags()
	at := fs.String("at", "", "play the film from this time of its subtitles, as a line's start: seconds, mm:ss or hh:mm:ss")
	positional, err := e.parse(fs, args)
	if err != nil {
		return err
	}
	if len(positional) > 1 {
		return usagef("takes at most one film id, got %d arguments", len(positional))
	}
	params := remote.ShowParams{}
	if len(positional) == 1 {
		params.ID = positional[0]
	}
	if *at != "" {
		if params.ID == "" {
			return usagef("--at needs a film id")
		}
		ms, err := clockFlag("at", *at)
		if err != nil {
			return err
		}
		params.AtMS = &ms
	}
	var shown remote.ShowResult
	if done, err := e.answer(remote.MethodShow, params, &shown); done || err != nil {
		return err
	}
	// The window shows the library rather than a film it cannot play yet; the command still did what it could, so it says why and succeeds.
	switch shown.Reason {
	case remote.ShowInQueue:
		fmt.Fprintf(e.stderr, "kando open: %s is still being processed; showing the library\n", printable(params.ID))
	case remote.ShowNoVideo:
		fmt.Fprintf(e.stderr, "kando open: %s has no video on disk; showing the library\n", printable(params.ID))
	}
	return nil
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
