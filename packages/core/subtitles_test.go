package core

import (
	"slices"
	"testing"
)

func TestCueLinesFollowTheModeAndOrder(t *testing.T) {
	tr, orig := CueLine{Text: "你好"}, CueLine{Text: "hello", Original: true}
	for _, c := range []struct {
		mode, order, translation, original string
		want                               []CueLine
	}{
		{SubBilingual, SubTranslationFirst, "你好", "hello", []CueLine{tr, orig}},
		{SubBilingual, SubOriginalFirst, "你好", "hello", []CueLine{orig, tr}},
		{SubOff, SubTranslationFirst, "你好", "hello", []CueLine{tr, orig}},
		{SubTranslation, SubTranslationFirst, "你好", "hello", []CueLine{tr}},
		{SubTranslation, SubTranslationFirst, "", "hello", []CueLine{orig}},
		{SubOriginal, SubOriginalFirst, "你好", "hello", []CueLine{orig}},
		{SubOriginal, SubTranslationFirst, "你好", "", nil},
		{SubBilingual, SubTranslationFirst, "", "hello", []CueLine{orig}},
	} {
		if got := CueLines(c.mode, c.order, c.translation, c.original); !slices.Equal(got, c.want) {
			t.Errorf("CueLines(%s, %s, %q, %q) = %v, want %v", c.mode, c.order, c.translation, c.original, got, c.want)
		}
	}
}

func TestPictureLinesDimOrBlankAnUntranslatedCue(t *testing.T) {
	if got, want := PictureLines(SubTranslation, SubTranslationFirst, UntranslatedDim, "", "hello"), []CueLine{{Text: "hello", Original: true, Dim: true}}; !slices.Equal(got, want) {
		t.Errorf("dim: %v, want %v", got, want)
	}
	if got := PictureLines(SubTranslation, SubTranslationFirst, UntranslatedBlank, "", "hello"); got != nil {
		t.Errorf("blank: %v, want none", got)
	}
	if got, want := PictureLines(SubBilingual, SubTranslationFirst, UntranslatedBlank, "", "hello"), []CueLine{{Text: "hello", Original: true}}; !slices.Equal(got, want) {
		t.Errorf("bilingual keeps the original undimmed: %v, want %v", got, want)
	}
	if got, want := PictureLines(SubTranslation, SubTranslationFirst, UntranslatedBlank, "你好", "hello"), []CueLine{{Text: "你好"}}; !slices.Equal(got, want) {
		t.Errorf("translated: %v, want %v", got, want)
	}
}
