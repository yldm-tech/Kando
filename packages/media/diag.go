package media

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"slices"
	"strings"
	"time"

	"kando/packages/core"
	"kando/packages/track"
)

// 诊断日志：一部片做完后哪里留了空、为什么——没译出的句子、放弃的格、重试的请求——写进标准 logger，app 把它写进日志文件（kando.log），以后发现一段没译时，日志里能查到是哪几句、哪一步、什么错误。只记时间、数量和错误，不记台词也不记片名：片子用任务 ID 指代。

type logJobKey struct{}

type logSpanKey struct{}

// withLogJob names job id in what logf writes of the work ctx is for.
func withLogJob(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, logJobKey{}, id)
}

// withLogSpan names the lines from start to end of the film in what logf writes of the work ctx is for, as a batch of a translation does.
func withLogSpan(ctx context.Context, lines []core.Segment) context.Context {
	if len(lines) == 0 {
		return ctx
	}
	return withLogRange(ctx, lines[0].Start, lines[len(lines)-1].End, len(lines))
}

// withLogRange is withLogSpan of n lines from start to end of the film.
func withLogRange(ctx context.Context, start, end time.Duration, n int) context.Context {
	return context.WithValue(ctx, logSpanKey{}, rangeText(start, end, n))
}

// logf writes a diagnostic line, led by the job and the lines ctx names (withLogJob, withLogSpan).
func logf(ctx context.Context, format string, args ...any) {
	prefix := ""
	if id, ok := ctx.Value(logJobKey{}).(string); ok {
		prefix = "job " + id + ": "
	}
	if span, ok := ctx.Value(logSpanKey{}).(string); ok {
		prefix += "lines " + span + ": "
	}
	log.Print(prefix + webAddress.ReplaceAllString(fmt.Sprintf(format, args...), "<address>"))
}

// webAddress is a web address in an error, as a request that timed out names the AI service's: the log does not keep it, as it does not the service's key.
var webAddress = regexp.MustCompile(`https?://[^\s"'<>]+`)

// clock writes d as h:mm:ss, as the player shows a time in the film.
func clock(d time.Duration) string {
	s := int(max(d, 0) / time.Second)
	return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
}

// spanText is the part of the film lines cover, as "0:01:42–0:04:10 (23)": from the first's start to the last's end, and how many they are.
func spanText(lines []core.Segment) string {
	return rangeText(lines[0].Start, lines[len(lines)-1].End, len(lines))
}

// rangeText is n lines from start to end of the film, as "0:01:42–0:04:10 (23)".
func rangeText(start, end time.Duration, n int) string {
	return fmt.Sprintf("%s–%s (%d)", clock(start), clock(end), n)
}

// maxLoggedGaps is how many runs of untranslated lines a line of the log names at most, the longest of them.
const maxLoggedGaps = 12

// gapsText is where the lines of lines that empty says are untranslated fall: each run of them in a row as its span (spanText), in the film's order, the longest maxLoggedGaps of them when there are more.
func gapsText(lines []core.Segment, empty func(i int) bool) string {
	type run struct{ from, to int }
	var runs []run
	for i := range lines {
		switch {
		case !empty(i):
		case len(runs) > 0 && runs[len(runs)-1].to == i:
			runs[len(runs)-1].to = i + 1
		default:
			runs = append(runs, run{i, i + 1})
		}
	}
	if len(runs) > maxLoggedGaps {
		longest := slices.Clone(runs)
		slices.SortStableFunc(longest, func(a, b run) int { return (b.to - b.from) - (a.to - a.from) })
		longest = longest[:maxLoggedGaps]
		runs = slices.DeleteFunc(runs, func(r run) bool { return !slices.Contains(longest, r) })
	}
	parts := make([]string, len(runs))
	for i, r := range runs {
		parts[i] = spanText(lines[r.from:r.to])
	}
	return strings.Join(parts, ", ")
}

// because is ": " and err, to end a line of the log with why, "" for no error.
func because(err error) string {
	if err == nil {
		return ""
	}
	return ": " + err.Error()
}

// unitsText is the part of the film units cover, the cells of one kind of work in a row, as "cells 0:04:30–0:07:00".
func unitsText(units []track.Unit) string {
	if len(units) == 0 {
		return "no cells"
	}
	start, _ := units[0].Span()
	_, end := units[len(units)-1].Span()
	if units[0].Work == track.WorkFinal {
		end = start + track.Block*track.Cell
	}
	return "cells " + clock(start) + "–" + clock(end)
}

// cellsText is the part of the film from the first of cells to the end of the last, as "cells 0:04:30–0:07:00".
func cellsText(cells []int) string {
	if len(cells) == 0 {
		return "no cells"
	}
	return "cells " + clock(time.Duration(cells[0])*track.Cell) + "–" + clock(time.Duration(cells[len(cells)-1]+1)*track.Cell)
}
