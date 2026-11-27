// Package aiusage is the ledger of what Kando asked of the user's AI services, month by month: the tokens and the requests of each pass (what the requests were for, as media.Passes names them) and of each host they went to. It is kept on this computer alone, in a file of its own beside settings.json (FileName), and never synced. It imports no interface package.
package aiusage

import (
	"cmp"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// FileName is the ledger's file, in the folder settings.json is in.
const FileName = "ai-usage.json"

// Keep is how many months the ledger keeps: the current one and the eleven before it.
const Keep = 12

// maxKeys is how many passes or hosts a month keeps apart; the requests of any more are counted under "", so a month whose requests went to ever new hosts stays small.
const maxKeys = 32

// monthLayout is how a month is written: 2026-11.
const monthLayout = "2006-01"

// Count is the tokens and the requests of a pass or a host in a month. Estimated is how many of its tokens, prompt and completion together, Kando estimated from the text sent and received, as the service said none.
type Count struct {
	Prompt     int64 `json:"prompt,omitempty"`
	Completion int64 `json:"completion,omitempty"`
	Requests   int64 `json:"requests,omitempty"`
	Estimated  int64 `json:"estimated,omitempty"`
}

// Tokens is the count's prompt and completion tokens together.
func (c Count) Tokens() int64 { return c.Prompt + c.Completion }

// add counts one request of prompt and completion tokens, estimated or as the service said.
func (c Count) add(prompt, completion int64, estimated bool) Count {
	c.Prompt += prompt
	c.Completion += completion
	c.Requests++
	if estimated {
		c.Estimated += prompt + completion
	}
	return c
}

// plus is c and d together.
func (c Count) plus(d Count) Count {
	return Count{Prompt: c.Prompt + d.Prompt, Completion: c.Completion + d.Completion, Requests: c.Requests + d.Requests, Estimated: c.Estimated + d.Estimated}
}

// clean makes every field of c 0 or more, and Estimated no more than its tokens.
func (c Count) clean() Count {
	c.Prompt, c.Completion, c.Requests = max(c.Prompt, 0), max(c.Completion, 0), max(c.Requests, 0)
	c.Estimated = min(max(c.Estimated, 0), c.Tokens())
	return c
}

// Month is one month's usage (MonthOf), by pass and by host.
type Month struct {
	Month  string           `json:"month"`
	ByPass map[string]Count `json:"byPass,omitempty"`
	ByHost map[string]Count `json:"byHost,omitempty"`
}

// Total is the month's usage of every pass together.
func (m Month) Total() Count {
	var total Count
	for _, c := range m.ByPass {
		total = total.plus(c)
	}
	return total
}

// Ledger is the months of usage kept, oldest first, no more than Keep.
type Ledger struct {
	Months []Month `json:"months,omitempty"`
}

// MonthOf is the month t is in, in t's time zone, as the ledger writes it: 2026-11.
func MonthOf(t time.Time) string { return t.Format(monthLayout) }

// Add counts one request of month, made for pass to host, of prompt and completion tokens, estimated or as the service said. A month the ledger does not have yet is started, and the oldest months beyond Keep are dropped.
func (l *Ledger) Add(month, pass, host string, prompt, completion int, estimated bool) {
	i := slices.IndexFunc(l.Months, func(m Month) bool { return m.Month == month })
	if i < 0 {
		l.Months = append(l.Months, Month{Month: month})
		l.sort()
		i = slices.IndexFunc(l.Months, func(m Month) bool { return m.Month == month })
	}
	m := &l.Months[i]
	p, c := int64(max(prompt, 0)), int64(max(completion, 0))
	m.ByPass = addTo(m.ByPass, pass, func(n Count) Count { return n.add(p, c, estimated) })
	m.ByHost = addTo(m.ByHost, host, func(n Count) Count { return n.add(p, c, estimated) })
	l.prune()
}

// addTo applies fn to the count of key in counts, or of "" when counts keeps maxKeys keys apart already and key is not one of them, and returns counts, made when it was nil.
func addTo(counts map[string]Count, key string, fn func(Count) Count) map[string]Count {
	if counts == nil {
		counts = map[string]Count{}
	}
	if _, ok := counts[key]; !ok && key != "" && keyed(counts) >= maxKeys {
		key = ""
	}
	counts[key] = fn(counts[key])
	return counts
}

// keyed is how many keys of counts are not "".
func keyed(counts map[string]Count) int {
	n := len(counts)
	if _, ok := counts[""]; ok {
		n--
	}
	return n
}

// Month is the usage of month, a Month with nothing counted when the ledger has none of it.
func (l Ledger) Month(month string) Month {
	for _, m := range l.Months {
		if m.Month == month {
			return m
		}
	}
	return Month{Month: month}
}

// Clean makes a ledger read from disk one Add could have made: months that are not written as MonthOf writes them are dropped, the same month twice is counted once with both, counts below 0 are 0, a pass with no name is dropped, the passes and hosts beyond maxKeys are counted under "", the months are sorted, and only the Keep newest stay.
func (l *Ledger) Clean() {
	merged := map[string]Month{}
	for _, m := range l.Months {
		if t, err := time.Parse(monthLayout, m.Month); err != nil || MonthOf(t) != m.Month {
			continue
		}
		into := merged[m.Month]
		into.Month = m.Month
		into.ByPass = mergeCounts(into.ByPass, m.ByPass, false)
		into.ByHost = mergeCounts(into.ByHost, m.ByHost, true)
		merged[m.Month] = into
	}
	l.Months = l.Months[:0]
	for _, m := range merged {
		l.Months = append(l.Months, m)
	}
	l.sort()
	l.prune()
}

// mergeCounts adds the counts of from, cleaned, to into, in the order of their keys so the same file always folds the same keys under "". A key "" is dropped unless keepEmpty, as a pass always has a name and a host may not.
func mergeCounts(into, from map[string]Count, keepEmpty bool) map[string]Count {
	keys := make([]string, 0, len(from))
	for key := range from {
		if key != "" || keepEmpty {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	for _, key := range keys {
		c := from[key].clean()
		into = addTo(into, key, func(n Count) Count { return n.plus(c) })
	}
	return into
}

// sort puts the months oldest first.
func (l *Ledger) sort() {
	slices.SortFunc(l.Months, func(a, b Month) int { return cmp.Compare(a.Month, b.Month) })
}

// prune drops the oldest months beyond Keep.
func (l *Ledger) prune() {
	if n := len(l.Months); n > Keep {
		l.Months = slices.Delete(l.Months, 0, n-Keep)
	}
}

// Load reads the ledger at path and cleans it (Ledger.Clean). A file that is not there is an empty ledger and no error; one that cannot be read or parsed is an empty ledger and the error.
func Load(path string) (Ledger, error) {
	var l Ledger
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return l, nil
	}
	if err != nil {
		return l, err
	}
	if err := json.Unmarshal(data, &l); err != nil {
		return Ledger{}, err
	}
	l.Clean()
	return l, nil
}

// Save writes the ledger to path through a file beside it renamed over it, readable by the user alone, making its folder when it is not there.
func (l Ledger) Save(path string) error {
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(append(data, '\n'))
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}
