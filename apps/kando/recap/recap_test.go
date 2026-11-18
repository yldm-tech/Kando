package recap

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/lines"
)

var seen = []lines.Line{
	lines.New(2*time.Second, "每条河都从山间的细流开始。", "Every river begins as a trickle high in the mountains."),
	lines.New(30*time.Second+400*time.Millisecond, "水流向下切割岩石。", "The water cuts down through the rock."),
	lines.New(70*time.Second+400*time.Millisecond, "这些弯道叫河曲。", "These sweeping curves are called meanders."),
	lines.New(95*time.Second, "", "Over time, the bends grow wider."),
}

// TestParseKeepsTheItemsAtLinesWatched checks that the reply's items come in the order of the film, each at the start of the watched line it names, that an item naming a moment no watched line starts at is dropped, and that no more than Count are kept.
func TestParseKeepsTheItemsAtLinesWatched(t *testing.T) {
	reply := "```json\n" + `{"items": [
		{"at": "1:10", "text": " 河流弯成河曲。 "},
		{"at": "0:02", "text": "河流始于山间细流。"},
		{"at": "5:00", "text": "后面才讲的。"},
		{"at": "0:30", "text": "水往下切割岩石。"},
		{"at": "1:35", "text": "弯道越来越宽。"}
	]}` + "\n```"
	got, err := Parse(reply, seen)
	if err != nil {
		t.Fatal(err)
	}
	want := []Item{{At: 2 * time.Second, Text: "河流始于山间细流。"}, {At: 30*time.Second + 400*time.Millisecond, Text: "水往下切割岩石。"}, {At: 70*time.Second + 400*time.Millisecond, Text: "河流弯成河曲。"}}
	if !slices.Equal(got, want) {
		t.Fatalf("Parse = %+v, want %+v", got, want)
	}
	for _, reply := range []string{"", "not json", `{"items": []}`, `{"items": [{"at": "9:00", "text": "x"}]}`, `{"items": [{"at": "0:02", "text": " "}]}`} {
		if _, err := Parse(reply, seen); !errors.Is(err, ErrEmpty) {
			t.Fatalf("Parse(%q) error %v, want ErrEmpty", reply, err)
		}
	}
}

// TestExcerptsAreTheFirstMiddleAndLastLines checks the lines the card shows without an AI service: the first, the middle and the last watched, their translations or else their originals, and every line when there are no more than Count.
func TestExcerptsAreTheFirstMiddleAndLastLines(t *testing.T) {
	got := Excerpts(seen)
	want := []Item{{At: 2 * time.Second, Text: "每条河都从山间的细流开始。"}, {At: 70*time.Second + 400*time.Millisecond, Text: "这些弯道叫河曲。"}, {At: 95 * time.Second, Text: "Over time, the bends grow wider."}}
	if !slices.Equal(got, want) {
		t.Fatalf("Excerpts = %+v, want %+v", got, want)
	}
	if got := Excerpts(seen[:2]); len(got) != 2 {
		t.Fatalf("two lines gave %+v", got)
	}
	if got := Excerpts(nil); len(got) != 0 {
		t.Fatalf("no lines gave %+v", got)
	}
}

// TestRequestSendsTheLinesWatched checks that the message names the film and gives every line watched with its start, and the instruction asks for Count items in the language given.
func TestRequestSendsTheLinesWatched(t *testing.T) {
	got := Request("Rivers", seen)
	for _, want := range []string{"影片：Rivers", "[0:02] 每条河都从山间的细流开始。 / Every river begins", "[1:35] Over time, the bends grow wider."} {
		if !strings.Contains(got, want) {
			t.Fatalf("Request = %q, want %q in it", got, want)
		}
	}
	if p := Prompt("繁體中文"); !strings.Contains(p, "用繁體中文") || !strings.Contains(p, "3 条") {
		t.Fatalf("Prompt = %q", p)
	}
}

// TestRequestLeavesOutTheEarliestPastTheCap checks that a long film's message keeps its latest lines within the cap, which the recap is most about.
func TestRequestLeavesOutTheEarliestPastTheCap(t *testing.T) {
	var many []lines.Line
	for i := range 4000 {
		many = append(many, lines.New(time.Duration(i)*time.Second, "", strings.Repeat("x", 20)))
	}
	got := Request("Long", many)
	if n := len([]rune(got)); n > contextRunes+100 {
		t.Fatalf("message of %d runes", n)
	}
	if !strings.Contains(got, "[1:06:39]") || strings.Contains(got, "[0:00]") {
		t.Fatal("the message lost the latest lines rather than the earliest")
	}
}
