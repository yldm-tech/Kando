package phonesync

import (
	"encoding/json"
	"time"

	"kando/packages/study"
	psync "kando/packages/sync"
)

// WordRecords are book's words as records, and the words removed from it as tombstones. film is the ID of the film of a word's line's video, "" for one the desktop does not serve or a word looked up in none.
func WordRecords(book study.Book, film func(video string) string) []psync.Record {
	out := make([]psync.Record, 0, len(book.Words)+len(book.RemovedWords))
	for _, w := range book.Words {
		saved := psync.Stamp{At: w.SavedAt, Device: w.Device}
		meaning := stampOf(w.Stamps.Meaning)
		id := ""
		if w.Video != "" {
			id = film(w.Video)
		}
		out = append(out, psync.Record{Kind: psync.KindWord, ID: w.ID, Film: id, Fields: map[string]psync.Field{
			psync.FieldWord:               {Value: psync.Value(w.Text), Stamp: saved},
			psync.FieldSpoken:             {Value: psync.Value(w.Spoken), Stamp: saved},
			psync.FieldContext:            {Value: psync.Value(w.Context), Stamp: saved},
			psync.FieldContextTranslation: {Value: psync.Value(w.ContextTranslation), Stamp: saved},
			psync.FieldTitle:              {Value: psync.Value(w.Film), Stamp: saved},
			psync.FieldStartMS:            {Value: psync.Value(w.Start.Milliseconds()), Stamp: saved},
			psync.FieldSavedAt:            {Value: psync.Value(w.SavedAt), Stamp: saved},
			psync.FieldReading:            {Value: psync.Value(w.Reading), Stamp: meaning},
			psync.FieldMeaning:            {Value: psync.Value(w.Meaning), Stamp: meaning},
			psync.FieldLanguage:           {Value: psync.Value(w.Language), Stamp: meaning},
			psync.FieldReview:             {Value: psync.Value(psync.Review{Box: w.Box, Due: w.Due}), Stamp: stampOf(w.Stamps.Review)},
		}})
	}
	for _, r := range book.RemovedWords {
		at := stampOf(r.At)
		out = append(out, psync.Record{Kind: psync.KindWord, ID: r.ID, Removed: &at})
	}
	return out
}

// WordsOf are word records, as the ledger has them once merged, as book takes them in (study.Book.MergeWords): the words there and the words removed. video is the path, as the book keys it, of the video of film, ok false for a film the desktop does not serve, whose word keeps no line's video. A record without a word is left out.
func WordsOf(book study.Book, records []psync.Record, video func(film string) (string, bool)) (words []study.Word, removed []study.Removed) {
	byID := map[string]study.Word{}
	for _, w := range book.Words {
		byID[w.ID] = w
	}
	for _, r := range records {
		if r.Kind != psync.KindWord {
			continue
		}
		have, had := byID[r.ID]
		if r.Removed != nil {
			removed = append(removed, study.Removed{ID: r.ID, Video: study.WordKey(have.Text), At: studyStamp(*r.Removed)})
			continue
		}
		w := have
		if !had {
			w = study.Word{ID: r.ID}
			if key, ok := video(r.Film); ok && r.Film != "" {
				w.Video = key
			}
		}
		text := func(name string, to *string) {
			if f, ok := r.Fields[name]; ok {
				*to = ""
				readJSON(f.Value, to)
			}
		}
		if f, ok := r.Fields[psync.FieldSavedAt]; ok {
			w.Device = f.Device
			readJSON(f.Value, &w.SavedAt)
		}
		text(psync.FieldWord, &w.Text)
		text(psync.FieldSpoken, &w.Spoken)
		text(psync.FieldContext, &w.Context)
		text(psync.FieldContextTranslation, &w.ContextTranslation)
		text(psync.FieldTitle, &w.Film)
		if f, ok := r.Fields[psync.FieldStartMS]; ok {
			w.Start = max(msField(f), 0)
		}
		if f, ok := r.Fields[psync.FieldMeaning]; ok {
			text(psync.FieldReading, &w.Reading)
			text(psync.FieldMeaning, &w.Meaning)
			text(psync.FieldLanguage, &w.Language)
			w.Stamps.Meaning = studyStamp(f.Stamp)
		}
		if f, ok := r.Fields[psync.FieldReview]; ok {
			var rv psync.Review
			readJSON(f.Value, &rv)
			w.Box, w.Due = max(rv.Box, 0), rv.Due
			w.Stamps.Review = studyStamp(f.Stamp)
		}
		if w.Text == "" {
			continue
		}
		words = append(words, w)
	}
	return words, removed
}

// wordFieldValid reports whether v is a value field name of a word record may hold.
func wordFieldValid(name string, v json.RawMessage, x any) bool {
	_, isString := x.(string)
	_, isNumber := x.(float64)
	switch name {
	case psync.FieldWord, psync.FieldSpoken, psync.FieldReading, psync.FieldMeaning, psync.FieldLanguage, psync.FieldContext, psync.FieldContextTranslation, psync.FieldTitle, psync.FieldNote:
		return isString
	case psync.FieldStartMS:
		return isNumber
	case psync.FieldSavedAt:
		var at time.Time
		return isString && json.Unmarshal(v, &at) == nil
	case psync.FieldReview:
		var r psync.Review
		_, isObject := x.(map[string]any)
		return isObject && json.Unmarshal(v, &r) == nil
	}
	return true
}

// WordSign is what a phone is told of w that the desktop changes, as text, for the desktop to sum up and see when it changed.
func WordSign(w study.Word) []string {
	return []string{w.ID, w.Video, stampSign(w.Stamps.Meaning), stampSign(w.Stamps.Review)}
}
