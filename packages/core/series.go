package core

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// Episodes: films whose names say they are episodes of one show, as "Planet Earth S01E02.mkv", "地理课 第3集" or "Lecture 04", are likely a series. EpisodeOf reads the show's name from a film's name, so the app can ask the user whether films that share it are one series; it only reads names, so it never puts films in a series without asking.

var (
	// episodeTags are the bracketed tags release names carry, as [1080p] or (2024), which are not the show's name.
	episodeTags = regexp.MustCompile(`[\[【(（][^\]】)）]*[\]】)）]`)
	// episodeMarks are the ways a name marks an episode, each with the show's name before it: S01E02, EP02 or Episode 2, 第2集, and a number at the end after a separator.
	episodeMarks = []*regexp.Regexp{
		regexp.MustCompile(`(?i)^(.*?)[\s._-]*\bS\d{1,2}[\s._-]*E\d{1,3}\b`),
		regexp.MustCompile(`(?i)^(.*?)[\s._-]*\b(?:EP|E|Episode|Ep\.)\s*\d{1,3}\b`),
		regexp.MustCompile(`^(.*?)\s*第\s*[0-9０-９一二三四五六七八九十百]+\s*[集话話回期讲講課课]`),
		regexp.MustCompile(`^(.*?)[\s._-]+(?:#|No\.?\s*|Part\s*|Vol\.?\s*)?\d{1,3}$`),
	}
	// episodeSpace are the separators release names write for spaces.
	episodeSpace = regexp.MustCompile(`[\s._]+`)
)

// EpisodeOf is the name of the show the film named name, without its file extension, is an episode of, as name marks it, and the key films of the same show share whatever their case and separators; ok is false for a name that marks no episode, or leaves too little of a name before its mark.
func EpisodeOf(name string) (show, key string, ok bool) {
	name = strings.TrimSpace(episodeTags.ReplaceAllString(name, " "))
	for _, mark := range episodeMarks {
		m := mark.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		show = strings.Trim(episodeSpace.ReplaceAllString(m[1], " "), " -–—:：,，")
		if utf8.RuneCountInString(show) < 2 {
			return "", "", false
		}
		return show, strings.ToLower(show), true
	}
	return "", "", false
}
