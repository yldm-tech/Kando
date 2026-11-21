package synctest

import (
	"context"
	"strings"

	"kando/packages/sync"
)

// study is the stand-in's lookups, explanations, follow-up questions and recaps: the entries it answers by word, in lower case, the explanation it answers every line with, and its sections, the recap it answers, whether no AI service is set, and the calls it was asked, in order.
type study struct {
	entries     map[string]sync.WordEntry
	explanation string
	sections    []sync.ExplainSection
	recap       []sync.RecapItem
	noAI        bool
	// model is the model explanations, answers and recaps name, "test-model" when "".
	model    string
	lookups  []sync.WordLookup
	explains []sync.LineExplain
	asks     []sync.ExplainAsk
	recaps   []sync.FilmRecap
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

// SetExplanationSections has the desktop explain every line with sections, as a desktop that heads them does; its text is the sections' paragraphs.
func (d *Desktop) SetExplanationSections(sections ...sync.ExplainSection) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.study.sections = sections
	var paragraphs []string
	for _, s := range sections {
		paragraphs = append(paragraphs, s.Text)
	}
	d.study.explanation = strings.Join(paragraphs, "\n\n")
}

// SetRecap has the desktop sum up any part watched with items; with none set, it sums up the first and the last line sent.
func (d *Desktop) SetRecap(items ...sync.RecapItem) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.study.recap = items
}

// SetModel has the desktop name model as the one its explanations, answers and recaps came from, rather than test-model.
func (d *Desktop) SetModel(model string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.study.model = model
}

// modelLocked is the model the desktop names, with d.mu held.
func (d *Desktop) modelLocked() string {
	if d.study.model == "" {
		return "test-model"
	}
	return d.study.model
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

// Asks are the follow-up questions the desktop was asked, in order.
func (d *Desktop) Asks() []sync.ExplainAsk {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]sync.ExplainAsk(nil), d.study.asks...)
}

// Recaps are the recaps the desktop was asked for, in order.
func (d *Desktop) Recaps() []sync.FilmRecap {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]sync.FilmRecap(nil), d.study.recaps...)
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
	return sync.Explanation{Text: text, Sections: s.d.study.sections, Language: "简体中文", Model: s.d.modelLocked()}, nil
}

// Ask answers a question with the question and the line it is about.
func (s studyService) Ask(_ context.Context, _ sync.Caller, req sync.ExplainAsk) (sync.ExplainAnswer, error) {
	s.d.mu.Lock()
	defer s.d.mu.Unlock()
	s.d.study.asks = append(s.d.study.asks, req)
	if s.d.study.noAI {
		return sync.ExplainAnswer{}, sync.Errorf(sync.CodeNoAI, "no AI service is set")
	}
	return sync.ExplainAnswer{Text: "Answered: " + req.Question, Language: "简体中文", Model: s.d.modelLocked()}, nil
}

// Recap answers the items set, or else one at the first line sent and one at the last.
func (s studyService) Recap(_ context.Context, _ sync.Caller, req sync.FilmRecap) (sync.Recap, error) {
	s.d.mu.Lock()
	defer s.d.mu.Unlock()
	s.d.study.recaps = append(s.d.study.recaps, req)
	if s.d.study.noAI {
		return sync.Recap{}, sync.Errorf(sync.CodeNoAI, "no AI service is set")
	}
	items := s.d.study.recap
	if items == nil {
		first, last := req.Lines[0], req.Lines[len(req.Lines)-1]
		items = []sync.RecapItem{{StartMS: first.StartMS, Text: "Began: " + first.Original}}
		if last.StartMS != first.StartMS {
			items = append(items, sync.RecapItem{StartMS: last.StartMS, Text: "Then: " + last.Original})
		}
	}
	return sync.Recap{Items: items, Language: "简体中文", Model: s.d.modelLocked()}, nil
}
