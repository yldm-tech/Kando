package film

import (
	"slices"
	"testing"
)

// lines is the part of App that styledLines and the 双语 row use; the rest of App is never called.
type lines struct {
	App
	look        Look
	mode, order string
}

func (l *lines) Cue() *Look               { return &l.look }
func (l *lines) SubMode() string          { return l.mode }
func (l *lines) SetSubMode(mode string)   { l.mode = mode }
func (l *lines) SubOrder() string         { return l.order }
func (l *lines) SetSubOrder(order string) { l.order = order }

func texts(s screen, cue string) []string {
	var out []string
	for _, l := range s.styledLines(cue) {
		out = append(out, l.text)
	}
	return out
}

func TestStyledLinesFollowModeAndOrder(t *testing.T) {
	const cue = "你好\nhello"
	for _, tc := range []struct {
		mode, order string
		want        []string
	}{
		{SubBilingual, SubTranslationFirst, []string{"你好", "hello"}},
		{SubBilingual, SubOriginalFirst, []string{"hello", "你好"}},
		{SubTranslation, SubOriginalFirst, []string{"你好"}},
		{SubOriginal, SubTranslationFirst, []string{"hello"}},
		{SubOff, SubOriginalFirst, []string{"hello", "你好"}},
	} {
		s := screen{&lines{look: DefaultLook(), mode: tc.mode, order: tc.order}}
		if got := texts(s, cue); !slices.Equal(got, tc.want) {
			t.Fatalf("%s %s: %q, want %q", tc.mode, tc.order, got, tc.want)
		}
	}
	s := screen{&lines{look: DefaultLook(), mode: SubTranslation, order: SubTranslationFirst}}
	if got := texts(s, "hello"); !slices.Equal(got, []string{"hello"}) {
		t.Fatalf("translation only without a translation: %q, want the original", got)
	}
}

// TestLinesRowSetsModeAndOrder checks that the 双语 row's chips write the app's SubMode and SubOrder, and show the chip that is on.
func TestLinesRowSetsModeAndOrder(t *testing.T) {
	l := &lines{mode: SubBilingual, order: SubTranslationFirst}
	s := screen{l}
	s.chooseLines(SubTranslation)
	if l.mode != SubTranslation || l.order != SubTranslationFirst || s.linesChoice() != SubTranslation {
		t.Fatalf("只要译文: mode %q order %q chip %q", l.mode, l.order, s.linesChoice())
	}
	s.chooseLines(SubOriginalFirst)
	if l.mode != SubBilingual || l.order != SubOriginalFirst || s.linesChoice() != SubOriginalFirst {
		t.Fatalf("原文在上: mode %q order %q chip %q", l.mode, l.order, s.linesChoice())
	}
}
