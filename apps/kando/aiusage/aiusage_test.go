package aiusage

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// A request of a new month starts that month, and the month before keeps what it counted.
func TestAddStartsANewMonthAndKeepsTheLast(t *testing.T) {
	var l Ledger
	l.Add("2026-10", "translate", "api.everyapi.ai", 100, 20, false)
	l.Add("2026-11", "translate", "api.everyapi.ai", 50, 5, false)
	l.Add("2026-11", "chat", "api.everyapi.ai", 10, 2, true)
	if len(l.Months) != 2 || l.Months[0].Month != "2026-10" || l.Months[1].Month != "2026-11" {
		t.Fatalf("months %+v", l.Months)
	}
	if got := l.Month("2026-10").ByPass["translate"]; got != (Count{Prompt: 100, Completion: 20, Requests: 1}) {
		t.Fatalf("October's translate %+v", got)
	}
	nov := l.Month("2026-11")
	if got := nov.Total(); got != (Count{Prompt: 60, Completion: 7, Requests: 2, Estimated: 12}) {
		t.Fatalf("November's total %+v", got)
	}
	if got := nov.ByHost["api.everyapi.ai"]; got != (Count{Prompt: 60, Completion: 7, Requests: 2, Estimated: 12}) {
		t.Fatalf("November's host %+v", got)
	}
	if got := l.Month("2026-09"); got.Month != "2026-09" || got.Total() != (Count{}) {
		t.Fatalf("a month with nothing %+v", got)
	}
}

// The ledger keeps the Keep newest months, dropping the oldest, even when a request of an older month comes late.
func TestAddKeepsTwelveMonths(t *testing.T) {
	var l Ledger
	start := time.Date(2025, 1, 15, 0, 0, 0, 0, time.Local)
	for i := range 15 {
		l.Add(MonthOf(start.AddDate(0, i, 0)), "translate", "h", 1, 1, false)
	}
	if len(l.Months) != Keep || l.Months[0].Month != "2025-04" || l.Months[Keep-1].Month != "2026-03" {
		t.Fatalf("months kept: %d, %s to %s", len(l.Months), l.Months[0].Month, l.Months[len(l.Months)-1].Month)
	}
	l.Add("2024-12", "translate", "h", 1, 1, false)
	if len(l.Months) != Keep || l.Month("2024-12").ByPass != nil {
		t.Fatalf("a month older than those kept stayed: %+v", l.Months[0])
	}
}

// A month whose requests went to more hosts than maxKeys counts those beyond it under "".
func TestAddFoldsHostsBeyondTheBound(t *testing.T) {
	var l Ledger
	for i := range maxKeys + 3 {
		l.Add("2026-11", "translate", fmt.Sprintf("h%02d.example.com", i), 1, 0, false)
	}
	l.Add("2026-11", "translate", "h00.example.com", 1, 0, false)
	m := l.Month("2026-11")
	if len(m.ByHost) != maxKeys+1 || m.ByHost[""].Requests != 3 || m.ByHost["h00.example.com"].Requests != 2 {
		t.Fatalf("hosts %d, folded %+v, first %+v", len(m.ByHost), m.ByHost[""], m.ByHost["h00.example.com"])
	}
	if m.Total().Requests != maxKeys+4 {
		t.Fatalf("total %+v", m.Total())
	}
}

// Clean drops months not written as MonthOf writes them and passes with no name, counts the same month twice once, makes counts below 0 nothing and sorts the months.
func TestCleanMendsALedgerReadFromDisk(t *testing.T) {
	l := Ledger{Months: []Month{
		{Month: "2026-11", ByPass: map[string]Count{"translate": {Prompt: 10, Completion: 2, Requests: 1}, "": {Prompt: 99}}, ByHost: map[string]Count{"h": {Prompt: 10, Completion: 2, Requests: 1}}},
		{Month: "garbage", ByPass: map[string]Count{"translate": {Prompt: 5}}},
		{Month: "2026-1", ByPass: map[string]Count{"translate": {Prompt: 5}}},
		{Month: "2026-10", ByPass: map[string]Count{"chat": {Prompt: -4, Completion: 3, Requests: -1, Estimated: 50}}},
		{Month: "2026-11", ByPass: map[string]Count{"translate": {Prompt: 1, Requests: 1, Estimated: 1}}, ByHost: map[string]Count{"": {Prompt: 1, Requests: 1}}},
	}}
	l.Clean()
	want := Ledger{Months: []Month{
		{Month: "2026-10", ByPass: map[string]Count{"chat": {Completion: 3, Estimated: 3}}},
		{Month: "2026-11", ByPass: map[string]Count{"translate": {Prompt: 11, Completion: 2, Requests: 2, Estimated: 1}}, ByHost: map[string]Count{"h": {Prompt: 10, Completion: 2, Requests: 1}, "": {Prompt: 1, Requests: 1}}},
	}}
	if !reflect.DeepEqual(l, want) {
		t.Fatalf("cleaned\n%+v\nwant\n%+v", l, want)
	}
}

// Clean keeps only the Keep newest months of a file that holds more.
func TestCleanKeepsTwelveMonths(t *testing.T) {
	var l Ledger
	for i := range 14 {
		l.Months = append(l.Months, Month{Month: MonthOf(time.Date(2025, time.Month(14-i), 1, 0, 0, 0, 0, time.Local))})
	}
	l.Clean()
	if len(l.Months) != Keep || l.Months[0].Month != "2025-03" || l.Months[Keep-1].Month != "2026-02" {
		t.Fatalf("months %+v", l.Months)
	}
}

// Estimated tokens add up apart from those the service counted.
func TestEstimatedTokensAddUp(t *testing.T) {
	var l Ledger
	l.Add("2026-11", "proofread", "h", 100, 10, true)
	l.Add("2026-11", "proofread", "h", 200, 20, false)
	l.Add("2026-11", "proofread", "h", 7, 0, true)
	if got := l.Month("2026-11").ByPass["proofread"]; got != (Count{Prompt: 307, Completion: 30, Requests: 3, Estimated: 117}) || got.Tokens() != 337 {
		t.Fatalf("proofread %+v", got)
	}
}

// A ledger saved is the ledger loaded, and a file that is not there is an empty ledger.
func TestSaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", FileName)
	if l, err := Load(path); err != nil || len(l.Months) != 0 {
		t.Fatalf("missing file: %+v, %v", l, err)
	}
	var l Ledger
	l.Add("2026-10", "translate", "api.everyapi.ai", 100, 20, false)
	l.Add("2026-11", "chat", "localhost", 10, 2, true)
	if err := l.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || !reflect.DeepEqual(got, l) {
		t.Fatalf("loaded %+v, %v; saved %+v", got, err, l)
	}
	if info, err := os.Stat(path); err != nil || (os.PathSeparator == '/' && info.Mode().Perm() != 0o600) {
		t.Fatalf("file %v, %v", info, err)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatalf("files beside the ledger: %v", entries)
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if l, err := Load(path); err == nil || len(l.Months) != 0 {
		t.Fatalf("broken file: %+v, %v", l, err)
	}
}
