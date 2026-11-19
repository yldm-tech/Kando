package main

import (
	"cmp"
	"hash/fnv"
	"hash/maphash"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"

	"kando/apps/kando/film"
	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/annotate"
	"kando/apps/kando/plugins/wordlist"
	"kando/packages/core"
)

// The marks drawn on the words of the open film's originals (docs/plugin-system.md, E2): readings over the words in Han characters, a line in the color of their level under the words of the word list chosen, and a dashed one under the words in the vocabulary, as 字幕外观's 逐词标注 card turns them on (savedPlugins). They are computed off the UI thread (annotate.Lines) when the plugins' playing or afterChange hook finds that the film's key changed: its video, the times and originals of its lines, how it is marked and what from (pluginsMarkKey). Only the latest computation runs on: starting another, or the key going back to that of the marks shown, stops the one before at its next line. The result is kept, never changed, until the key changes again; the picture only reads it (CueMarks), and keeps showing it while the next one is computed, but not on a line whose original changed. A line's marks depend on its original and how it is marked alone, so when only the lines changed, as each part of a film made while it plays comes in (media.Make writes its subtitles every few seconds, more lines each time), the next computation marks only the originals the marks shown do not hold, and the lines that were there keep theirs at once.

// pluginsMarks is the marks of the lines of a film: key is what they were computed for, lines the marks of each line's original, and originals the originals they were computed for, index for index; byOriginal is lines by their originals, which a line keeps when the lines around it change (CueMarks) and which the next computation for the same film marked the same way takes instead of marking those originals again.
type pluginsMarks struct {
	key        pluginsMarkKey
	lines      [][]subtitle.Mark
	originals  []string
	byOriginal map[string][]subtitle.Mark
}

// sameMarking reports whether marks for k mark a line as those for o do: the same film, marked the same way from the same sources, whatever lines it has.
func (k pluginsMarkKey) sameMarking(o pluginsMarkKey) bool {
	k.subs, o.subs = "", ""
	return k == o
}

// pluginsMarkKey is what a film's marks are computed for: its video and the hash of its lines (pluginsSubsHash), the language it is spoken in, and how it is marked: readings, from the larger Japanese dictionary (analyze) and from the Chinese dictionaries open (dicts, their ids and contents), the word list chosen (list, its id and content), and the words in the vocabulary (known, with vocab the hash of the words in it spoken in the film's language, pluginsVocabHash, so a word added, removed or changed marks the lines again). It is comparable, so the playing hook compares it at each report.
type pluginsMarkKey struct {
	video, subs, spoken string
	readings, analyze   bool
	known               bool
	list, dicts, vocab  string
}

// pluginsSubs is the lines a hash was last taken of (pluginsSubsHash).
type pluginsSubs struct {
	cues []core.Segment
	hash string
}

// pluginsList is the word list chosen, read off the UI thread for the content hash: l, or why it could not be read.
type pluginsList struct {
	id, hash string
	l        *wordlist.List
	err      error
}

// pluginsMarkPrefs is how the lines are marked, as 逐词标注 sets it: readings, the id of the word list whose levels draw a line under the words ("" for none), and known, a dashed line under the words in the vocabulary.
type pluginsMarkPrefs struct {
	readings bool
	list     string
	known    bool
}

// pluginsMarksFollow is the plugins' playing and afterChange hook: when the open film's key (pluginsMarkKey) is not that of its marks, nor of the marks being computed, it computes them off the UI thread. Each call costs a comparison only. While the open film's subtitles are read again (reloadOpenSubtitles, a translation batch rewriting the file) it keeps the marks and the computation awaited, so lines that come back with the same times and originals show the same marks without computing them again. It reports whether it changed what a binding reads: marks the picture showed dropped, as when the marks were turned off, or the word list read forgotten, whose problem the plugins page shows (pluginsListIssues).
func (a *app) pluginsMarksFollow() bool {
	p := &a.plugins
	if subs := a.player.subs; a.openKey != "" && subs.key == a.openKey && !subs.ready {
		return false
	}
	pack, listed := a.pluginsMarkList()
	forgot := false
	if p.list != nil && (!listed || p.list.id != pack.ID || p.list.hash != pack.Hash) {
		p.list, forgot = nil, true
	}
	key, ok := a.pluginsMarkKey()
	if !ok {
		a.pluginsMarkStop()
		shown := p.marks.lines != nil
		p.marks = pluginsMarks{}
		return shown || forgot
	}
	if key == p.marks.key {
		// Back to the key of the marks shown: marks still being computed for another key are stopped, and dropped if they come.
		a.pluginsMarkStop()
		return forgot
	}
	if key == p.marking || a.update == nil {
		return forgot
	}
	a.pluginsMarkStop()
	stop := &atomic.Bool{}
	p.marking, p.stop = key, stop
	cues := a.playerCues()
	lines := make([]string, len(cues))
	for i, c := range cues {
		_, lines[i] = film.SplitCue(c.Text)
	}
	job := a.pluginsMarkJobFor(key)
	job.src.Stop = stop.Load
	// The originals the marks shown hold for the same film marked the same way are not marked again.
	if p.marks.key.sameMarking(key) {
		job.had = p.marks.byOriginal
	}
	update := a.update
	go func() {
		marks, byOriginal, list := job.run(lines)
		if stop.Load() && (list == nil || list == job.cached) {
			return
		}
		update(func() {
			a.pluginsKeepList(list, job.cached)
			if p.stop != stop || stop.Load() {
				return
			}
			p.marking, p.stop = pluginsMarkKey{}, nil
			p.marks = pluginsMarks{key: key, lines: marks, originals: lines, byOriginal: byOriginal}
		})
	}()
	return forgot
}

// pluginsMarkJob is what computing the marks of a key takes, read on the engine's goroutine (pluginsMarkJobFor): the sources but the word list, the word list chosen when there is one (pack, listed), the list read of it already (cached), which run reads otherwise, and had, the marks of originals computed before for the same film marked the same way (pluginsMarks.byOriginal), which run takes rather than marking those originals again.
type pluginsMarkJob struct {
	src    annotate.Sources
	pack   plugins.Pack
	listed bool
	cached *pluginsList
	had    map[string][]subtitle.Mark
}

// pluginsMarkJobFor is what computing the marks of key takes, on the engine's goroutine: the Chinese dictionaries open for the readings, the words of the vocabulary spoken in the film's language for the dashed lines, and the word list chosen.
func (a *app) pluginsMarkJobFor(key pluginsMarkKey) pluginsMarkJob {
	p := &a.plugins
	j := pluginsMarkJob{src: annotate.Sources{Spoken: key.spoken, Readings: key.readings, Analyze: key.analyze}}
	if key.readings {
		j.src.Dicts, _ = a.pluginsDictsFor("zh", "")
	}
	if key.known {
		j.src.Known = map[string]bool{}
		for _, w := range a.savedBook().Words {
			if annotate.SpokenIn(w.Spoken, key.spoken) {
				j.src.Known[wordlist.Key(w.Text)] = true
			}
		}
	}
	j.pack, j.listed = a.pluginsMarkList()
	if j.listed {
		j.src.ListLanguage = j.pack.Wordlist.Language
		if p.list != nil && p.list.id == j.pack.ID && p.list.hash == j.pack.Hash {
			j.cached = p.list
		}
	}
	return j
}

// run computes the marks of lines, the originals of a film's lines, off the engine's goroutine (annotate.Lines), marking only the originals j.had does not hold: it answers each line's marks, index for index, those marks by original, and the word list read for them, nil when none is chosen: the one read already, or the one it read, which the engine's goroutine keeps (pluginsKeepList).
func (j pluginsMarkJob) run(lines []string) ([][]subtitle.Mark, map[string][]subtitle.Mark, *pluginsList) {
	list := j.cached
	if j.listed && list == nil {
		l, err := wordlist.Open(j.pack)
		list = &pluginsList{id: j.pack.ID, hash: j.pack.Hash, l: l, err: err}
	}
	src := j.src
	if list != nil {
		src.List = list.l
	}
	var fresh []string
	for _, line := range lines {
		if _, ok := j.had[line]; !ok {
			fresh = append(fresh, line)
		}
	}
	got := annotate.Lines(fresh, src)
	byOriginal := make(map[string][]subtitle.Mark, len(lines))
	for i, line := range got {
		var marks []subtitle.Mark
		for _, m := range line {
			marks = append(marks, subtitle.Mark(m))
		}
		byOriginal[fresh[i]] = marks
	}
	marks := make([][]subtitle.Mark, len(lines))
	for i, line := range lines {
		m, ok := byOriginal[line]
		if !ok {
			m = j.had[line]
			byOriginal[line] = m
		}
		marks[i] = m
	}
	return marks, byOriginal, list
}

// pluginsKeepList keeps list, the word list a computation of marks read (pluginsMarkJob.run), when it is not cached, the one it was given.
func (a *app) pluginsKeepList(list, cached *pluginsList) {
	if list != nil && list != cached {
		a.plugins.list = list
	}
}

// pluginsMarkStop stops the marks being computed, if any, at the next line (annotate.Sources.Stop); their result, if it comes, is dropped.
func (a *app) pluginsMarkStop() {
	p := &a.plugins
	if p.stop != nil {
		p.stop.Store(true)
	}
	p.marking, p.stop = pluginsMarkKey{}, nil
}

// pluginsMarkKey is the open film's key (pluginsMarkKey), and false when there is nothing to mark: no film open with lines, or no mark turned on.
func (a *app) pluginsMarkKey() (pluginsMarkKey, bool) {
	cues := a.playerCues()
	if len(cues) == 0 || a.player.film.video == "" || !a.pluginsMarksOn() {
		return pluginsMarkKey{}, false
	}
	job, _ := a.library.Find(a.openKey)
	return a.pluginsMarkKeyFor(a.player.film.video, a.pluginsSubsHash(cues), cmp.Or(job.SpeechLanguage, job.SpokenLanguage), true)
}

// pluginsMarksOn reports whether a mark is turned on: the readings, a word list installed and turned on chosen, or the words in the vocabulary.
func (a *app) pluginsMarksOn() bool {
	_, listed := a.pluginsMarkList()
	return a.plugins.prefs.readings || listed || a.plugins.prefs.known
}

// pluginsMarkKeyFor is the key (pluginsMarkKey) of the marks of the lines of video, whose hash is subs, heard in spoken, as they are marked now, and false when no mark is turned on. vocab says whether the words of the vocabulary are underlined, as they are in a film of the open library, whose vocabulary is the one the app holds (savedBook).
func (a *app) pluginsMarkKeyFor(video, subs, spoken string, vocab bool) (pluginsMarkKey, bool) {
	if !a.pluginsMarksOn() {
		return pluginsMarkKey{}, false
	}
	prefs := a.plugins.prefs
	pack, listed := a.pluginsMarkList()
	key := pluginsMarkKey{video: video, subs: subs, spoken: spoken, readings: prefs.readings, known: prefs.known && vocab}
	if listed {
		key.list = pack.ID + "\x00" + pack.Hash
	}
	if key.known {
		key.vocab = a.pluginsVocabHash(key.spoken)
	}
	if key.readings {
		var ids []string
		for _, d := range a.pluginsOn(plugins.KindDictionary) {
			if open := a.plugins.dicts[d.ID]; open != nil && open.d != nil && slices.ContainsFunc(d.Dictionary.From, func(f string) bool { return core.BaseLanguage(f) == "zh" }) {
				ids = append(ids, d.ID+"\x00"+open.hash)
			}
		}
		key.dicts = strings.Join(ids, "\x01")
	}
	// The larger Japanese dictionary is read only for a Japanese word list or dictionary pack (segment.Annotated).
	key.analyze = (listed && core.BaseLanguage(pack.Wordlist.Language) == "ja") || slices.ContainsFunc(a.pluginsOn(plugins.KindDictionary), func(d plugins.Pack) bool {
		return slices.ContainsFunc(d.Dictionary.From, func(f string) bool { return core.BaseLanguage(f) == "ja" })
	})
	return key, true
}

// pluginsVocabSeed seeds the hashes of pluginsVocabHash; the hash is only compared within a run of the app, never kept, so a seed of the run's own does.
var pluginsVocabSeed = maphash.MakeSeed()

// pluginsVocabHash is the hash of the words of the vocabulary spoken in spoken, as the marks' known set has them (wordlist.Key). It is the sum of each word's hash, so it does not follow the vocabulary's order; it is taken at each report, as the vocabulary is changed in place (study.Book) by many hands (the vocabulary, the saved lines, both syncs), which a counter of its changes would have to follow everywhere, so it costs a pass over its words, without allocating for a word already in lower case (maphash.String, wordlist.Key).
func (a *app) pluginsVocabHash(spoken string) string {
	var sum uint64
	n := 0
	for _, w := range a.savedBook().Words {
		if annotate.SpokenIn(w.Spoken, spoken) {
			sum += maphash.String(pluginsVocabSeed, wordlist.Key(w.Text))
			n++
		}
	}
	return strconv.Itoa(n) + " " + strconv.FormatUint(sum, 36)
}

// pluginsMarkList is the word list pack chosen to color the words by their levels, while it is installed and turned on.
func (a *app) pluginsMarkList() (plugins.Pack, bool) {
	if a.plugins.prefs.list == "" {
		return plugins.Pack{}, false
	}
	for _, pack := range a.pluginsOn(plugins.KindWordlist) {
		if pack.ID == a.plugins.prefs.list {
			return pack, true
		}
	}
	return plugins.Pack{}, false
}

// pluginsSubsHash is the hash of what the marks of cues, the open film's lines, depend on: each line's start, end and original (film.SplitCue), not its translation, so a translation batch landing or a translation corrected does not mark the film again. The player puts new lines in a new slice whenever they change (playerCorrectSubs) and never changes those it has, so the hash is taken again only for a slice it has not seen.
func (a *app) pluginsSubsHash(cues []core.Segment) string {
	seen := &a.plugins.subs
	if len(cues) == len(seen.cues) && len(cues) > 0 && &cues[0] == &seen.cues[0] {
		return seen.hash
	}
	h := fnv.New64a()
	for _, c := range cues {
		_, original := film.SplitCue(c.Text)
		h.Write([]byte(strconv.FormatInt(int64(c.Start), 36) + " " + strconv.FormatInt(int64(c.End), 36) + " " + original + "\x00"))
	}
	*seen = pluginsSubs{cues: cues, hash: strconv.FormatUint(h.Sum64(), 36)}
	return seen.hash
}

// pluginsListIssues is the word list chosen when it could not be read, and why, for the page's problems.
func (a *app) pluginsListIssues() []plugins.Issue {
	pack, listed := a.pluginsMarkList()
	if l := a.plugins.list; listed && l != nil && l.err != nil {
		return []plugins.Issue{{Dir: pack.Dir, Err: l.err}}
	}
	return nil
}

// CueMarks are the marks of cue i's original, while the marks kept are of the open film's video and were computed for that original: the marks of line i when the film has as many lines as they were computed for and line i's original is the one they were for, else those of the same original among them (byOriginal), as when lines were added around it; nil otherwise. When the lines change, the marks kept go on showing on the lines whose originals did not change until those of the new lines replace them, so a translation landing or the next part of a film made while it plays does not make the readings go and come back nor the lines jump, while a corrected or new line shows no marks until its own are computed.
func (a *app) CueMarks(i int) []subtitle.Mark {
	m := a.plugins.marks
	cues := a.playerCues()
	if i < 0 || i >= len(cues) || m.key.video != a.player.film.video {
		return nil
	}
	_, original := film.SplitCue(cues[i].Text)
	if len(cues) == len(m.lines) && original == m.originals[i] {
		return m.lines[i]
	}
	return m.byOriginal[original]
}

// The 逐词标注 card's reads and edits (settings.MarksApp). Each change is saved, and marks the open film's lines again.

func (a *app) MarkReadings() bool { return a.plugins.prefs.readings }

func (a *app) SetMarkReadings(on bool) {
	a.pluginsSetMarks(func(p *pluginsMarkPrefs) { p.readings = on })
}

func (a *app) MarkWordlists() []plugins.Pack { return a.pluginsOn(plugins.KindWordlist) }

// MarkWordlist is the id of the word list chosen while it is turned on, else "".
func (a *app) MarkWordlist() string {
	pack, _ := a.pluginsMarkList()
	return pack.ID
}

func (a *app) SetMarkWordlist(id string) {
	a.pluginsSetMarks(func(p *pluginsMarkPrefs) { p.list = id })
}

func (a *app) MarkKnown() bool { return a.plugins.prefs.known }

func (a *app) SetMarkKnown(on bool) { a.pluginsSetMarks(func(p *pluginsMarkPrefs) { p.known = on }) }

// pluginsSetMarks changes how the lines are marked, saves the settings, saying so when they cannot be saved, and marks the open film's lines again.
func (a *app) pluginsSetMarks(change func(p *pluginsMarkPrefs)) {
	change(&a.plugins.prefs)
	if err := a.saveSettingsChanged(); err != nil {
		a.saveErr = a.messages().SettingsSaveFailed(err.Error())
	}
	a.pluginsMarksFollow()
}
