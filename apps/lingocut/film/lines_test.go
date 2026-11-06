package film

import (
	"slices"
	"testing"
	"time"
)

// lines is the part of App that styledLines and pictureLines use; the rest of App is never called.
type lines struct {
	App
	look                      Look
	mode, order, untranslated string
}

func (l *lines) Cue() *Look               { return &l.look }
func (l *lines) SubMode() string          { return l.mode }
func (l *lines) SetSubMode(mode string)   { l.mode = mode }
func (l *lines) SubOrder() string         { return l.order }
func (l *lines) SetSubOrder(order string) { l.order = order }
func (l *lines) Untranslated() string     { return l.untranslated }

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

// TestPictureLinesForAMissingTranslation checks what the picture shows in 仅译文 for a cue without a translation: the original dimmed, or nothing when the setting leaves it blank.
func TestPictureLinesForAMissingTranslation(t *testing.T) {
	l := &lines{look: DefaultLook(), mode: SubTranslation, order: SubTranslationFirst, untranslated: UntranslatedDim}
	got := screen{l}.pictureLines("hello")
	if len(got) != 1 || got[0].text != "hello" || !got[0].dim {
		t.Fatalf("dim: %+v", got)
	}
	if got := (screen{l}).pictureLines("你好\nhello"); len(got) != 1 || got[0].text != "你好" || got[0].dim {
		t.Fatalf("translated: %+v", got)
	}
	l.untranslated = UntranslatedBlank
	if got := (screen{l}).pictureLines("hello"); len(got) != 0 {
		t.Fatalf("blank: %+v", got)
	}
	l.mode = SubBilingual
	if got := (screen{l}).pictureLines("hello"); len(got) != 1 || got[0].dim {
		t.Fatalf("bilingual: %+v", got)
	}
}

func TestDelayText(t *testing.T) {
	for d, want := range map[time.Duration]string{
		0:                       "0.0 秒",
		300 * time.Millisecond:  "+0.3 秒",
		-100 * time.Millisecond: "-0.1 秒",
		-20 * time.Millisecond:  "0.0 秒",
		1500 * time.Millisecond: "+1.5 秒",
	} {
		if got := DelayText(d); got != want {
			t.Errorf("DelayText(%v) = %q, want %q", d, got, want)
		}
	}
}
