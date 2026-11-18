package lookup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTokens(t *testing.T) {
	cases := []struct {
		line string
		want []Token
	}{
		{"Every river begins as a trickle.", []Token{{"Every", true}, {" ", false}, {"river", true}, {" ", false}, {"begins", true}, {" ", false}, {"as", true}, {" ", false}, {"a", true}, {" ", false}, {"trickle", true}, {".", false}}},
		{"Don't—it's well-known, 'quoted' 42", []Token{{"Don't", true}, {"—", false}, {"it's", true}, {" ", false}, {"well-known", true}, {", '", false}, {"quoted", true}, {"' 42", false}}},
		{"Ça va? Très bien!", []Token{{"Ça", true}, {" ", false}, {"va", true}, {"? ", false}, {"Très", true}, {" ", false}, {"bien", true}, {"!", false}}},
		{"今日はいい天気", []Token{{"今日はいい天気", false}}},
		{"안녕 세상", []Token{{"안녕", true}, {" ", false}, {"세상", true}}},
		{"", nil},
	}
	for _, c := range cases {
		got := Tokens(c.line)
		if len(got) != len(c.want) {
			t.Fatalf("Tokens(%q) = %+v, want %+v", c.line, got, c.want)
		}
		var joined strings.Builder
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("Tokens(%q)[%d] = %+v, want %+v", c.line, i, got[i], c.want[i])
			}
			joined.WriteString(got[i].Text)
		}
		if joined.String() != c.line {
			t.Fatalf("Tokens(%q) joins to %q", c.line, joined.String())
		}
	}
}

func TestParse(t *testing.T) {
	e, err := Parse("rivers", "```json\n{\"word\": \"river\", \"reading\": \"/ˈrɪvə/\", \"meaning\": \"n. 河，江\"}\n```")
	if err != nil || e != (Entry{Word: "river", Reading: "/ˈrɪvə/", Meaning: "n. 河，江"}) {
		t.Fatalf("Parse = %+v, %v", e, err)
	}
	e, err = Parse(" trickle ", `{"meaning": "细流"}`)
	if err != nil || e.Word != "trickle" || e.Meaning != "细流" {
		t.Fatalf("a reply without the word: %+v, %v", e, err)
	}
	e, err = Parse("trickle", "细流；涓涓地流")
	if err != nil || e.Meaning != "细流；涓涓地流" || e.Word != "trickle" {
		t.Fatalf("a plain reply: %+v, %v", e, err)
	}
	for _, reply := range []string{"", `{"word": "x", "meaning": " "}`} {
		if _, err := Parse("x", reply); err != ErrNoMeaning {
			t.Fatalf("Parse(%q) error = %v", reply, err)
		}
	}
}

func TestPromptAndRequest(t *testing.T) {
	if p := Prompt("日本語"); !strings.Contains(p, "日本語") || !strings.Contains(p, `"meaning"`) {
		t.Fatalf("Prompt = %q", p)
	}
	if p := Prompt(""); !strings.Contains(p, "简体中文") {
		t.Fatalf("Prompt without a language = %q", p)
	}
	r := Request("trickle", " Every river begins as a trickle. ", "Rivers")
	if r != "影片：Rivers\n字幕：Every river begins as a trickle.\n要查的词：trickle" {
		t.Fatalf("Request = %q", r)
	}
	if r := Request("a", "b", ""); strings.Contains(r, "影片") {
		t.Fatalf("Request without a film = %q", r)
	}
}

// TestCacheKeepsLookupsOnDisk checks that a word looked up is found again by any case of it, in the same language only, after the cache is read back from its file; that a broken file reads as an empty cache; and that the oldest go once there are too many.
func TestCacheKeepsLookupsOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", FileName)
	c := Load(path)
	at := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	c.Put("Rivers", "简体中文", Entry{Word: "river", Meaning: "河"}, at)
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	got := Load(path)
	if e, ok := got.Get("rivers", "简体中文"); !ok || e.Word != "river" || e.Meaning != "河" {
		t.Fatalf("read back %+v, %v", e, ok)
	}
	if _, ok := got.Get("rivers", "日本語"); ok {
		t.Fatal("found in another language")
	}
	if err := os.WriteFile(path, []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	if Load(path).Len() != 0 {
		t.Fatal("a broken file read as words")
	}
	var mem Cache
	if _, ok := mem.Get("x", ""); ok || mem.Save() != nil {
		t.Fatal("the zero cache")
	}
	for i := range MaxEntries + 1 {
		mem.Put(strings.Repeat("w", i+1), "", Entry{Meaning: "m"}, at.Add(time.Duration(i)*time.Second))
	}
	if _, ok := mem.Get("w", ""); ok || mem.Len() != MaxEntries {
		t.Fatalf("the oldest stayed: %d entries", mem.Len())
	}
}
