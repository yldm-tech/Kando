package main

import (
	"path/filepath"
	"strings"

	"kando/apps/kando/film"
	"kando/packages/core"
	"kando/packages/media"
)

// The player's progress badge (film.Progresser): how far the open film's subtitles have come, layer by layer, while its job waits or runs, and the lines a done film's translation left untranslated.

// SubtitleLayers is how far the open film's subtitles have come: while its job waits or runs, its speech heard, its lines translated and, when Settings › 翻译 has it on, its final proofread, each as far as the job's stage and progress say; once done, how much of it is translated while lines are left untranslated. Nothing shows otherwise.
func (a *app) SubtitleLayers() film.Layers {
	item, ok := a.openFilm()
	if !ok || item.JobID == "" {
		return film.Layers{}
	}
	speechEngine, translateEngine := a.layersEngines()
	if item.Stage.Done() || item.Stage == "" {
		total := len(a.player.subs.cues)
		if item.Stage != core.StageComplete || item.Untranslated <= 0 || item.TranslationErr != "" || item.SameLanguage || total == 0 {
			return film.Layers{}
		}
		done := float64(max(total-item.Untranslated, 0)) / float64(total)
		return film.Layers{Show: true, Percent: int(done * 100), Untranslated: item.Untranslated, Rows: []film.Layer{
			{Kind: film.LayerSpeech, Engine: speechEngine, Done: 1},
			{Kind: film.LayerTranslation, Engine: translateEngine, Done: done},
		}}
	}
	job, _ := a.library.Find(item.JobID)
	p := job.Progress
	fraction := func() float64 {
		if p.Of > 0 {
			return float64(p.Lines) / float64(p.Of)
		}
		if p.Known {
			return p.Fraction
		}
		return 0
	}
	// A job translating a done film again has its speech already; one processing it again entirely hears it anew.
	var speech, translation, review float64
	if job.Redo.TranslatesOnly() {
		speech = 1
	}
	switch item.Stage {
	case core.StageTranscribing:
		speech = fraction()
	case core.StageTranslating:
		speech = 1
		switch {
		case p.Reviewing:
			translation, review = 1, fraction()
		case !p.Proofreading:
			translation = fraction()
		}
	case core.StageExporting:
		speech, translation, review = 1, 1, 1
	}
	rows := []film.Layer{
		{Kind: film.LayerSpeech, Engine: speechEngine, Done: speech},
		{Kind: film.LayerTranslation, Engine: translateEngine, Done: translation},
	}
	if a.prefs.review {
		rows = append(rows, film.Layer{Kind: film.LayerReview, Engine: translateEngine, Done: review})
	}
	return film.Layers{Show: true, Running: true, Percent: int(translation * 100), Rows: rows}
}

// layersEngines are what hears the films' speech and what translates them, as the badge names them: the whisper model's name, without its file's ggml- and .bin, and the AI service's model; "" for one not set.
func (a *app) layersEngines() (speech, translate string) {
	if tools, ok := a.pipeline.(media.Tools); ok && tools.WhisperModel != "" {
		speech = strings.TrimSuffix(strings.TrimPrefix(filepath.Base(tools.WhisperModel), "ggml-"), ".bin")
	}
	return speech, strings.TrimSpace(a.prefs.llmModel)
}
