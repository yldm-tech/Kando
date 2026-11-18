package synctest

import (
	"context"
	"strings"

	"kando/packages/sync"
)

// study is the stand-in's lookups and explanations: the entries it answers by word, in lower case, the explanation it answers every line with, whether no AI service is set, and the calls it was asked, in order.
type study struct {
	entries     map[string]sync.WordEntry
	explanation string
	noAI        bool
	lookups     []sync.WordLookup
	explains    []sync.LineExplain
}

// SetEntry has the desktop answer a lookup of word, whatever its case, with e.
func (d *Desktop) SetEntry(word string, e sync.WordEntry) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.study.entries == nil {
		d.study.entries = map[string]sync.WordEntry{}
	}
	d.study.entries[strings.ToLower(word)] = e
}

// SetExplanation has the desktop explain every line with text.
func (d *Desktop) SetExplanation(text string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.study.explanation = text
}

// SetNoAI has the desktop answer lookups and explanations as one with no AI service set does (sync.CodeNoAI), or answer them again.
func (d *Desktop) SetNoAI(on bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.study.noAI = on
}

// Lookups are the lookups the desktop was asked, in order.
func (d *Desktop) Lookups() []sync.WordLookup {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]sync.WordLookup(nil), d.study.lookups...)
}

// Explains are the explanations the desktop was asked for, in order.
func (d *Desktop) Explains() []sync.LineExplain {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]sync.LineExplain(nil), d.study.explains...)
}

// studyService is the stand-in's lookups and explanations as v2's calls ask for them. A word it has no entry for means itself in capitals, so any word answers.
type studyService struct{ d *Desktop }

func (s studyService) LookUp(_ context.Context, _ sync.Caller, req sync.WordLookup) (sync.WordEntry, error) {
	s.d.mu.Lock()
	defer s.d.mu.Unlock()
	s.d.study.lookups = append(s.d.study.lookups, req)
	if s.d.study.noAI {
		return sync.WordEntry{}, sync.Errorf(sync.CodeNoAI, "no AI service is set")
	}
	if e, ok := s.d.study.entries[strings.ToLower(req.Word)]; ok {
		return e, nil
	}
	return sync.WordEntry{Word: req.Word, Meaning: strings.ToUpper(req.Word), Language: "简体中文"}, nil
}

func (s studyService) Explain(_ context.Context, _ sync.Caller, req sync.LineExplain) (sync.Explanation, error) {
	s.d.mu.Lock()
	defer s.d.mu.Unlock()
	s.d.study.explains = append(s.d.study.explains, req)
	if s.d.study.noAI {
		return sync.Explanation{}, sync.Errorf(sync.CodeNoAI, "no AI service is set")
	}
	text := s.d.study.explanation
	if text == "" {
		text = "Explained: " + req.Lines[req.Index].Original
	}
	return sync.Explanation{Text: text, Language: "简体中文", Model: "test-model"}, nil
}
