package core

import "testing"

// TestFixOfFindsWhatChanged checks that FixOf finds the words an edit of a line changed: the run of characters that differ in a language written without spaces, whole words in one written with them, and nothing for an edit too small to be worth looking for elsewhere.
func TestFixOfFindsWhatChanged(t *testing.T) {
	for _, c := range []struct {
		before, after string
		want          Fix
		ok            bool
	}{
		{"河曲是河流的大弯道。", "曲流是河流的大弯道。", Fix{From: "河曲", To: "曲流"}, true},
		{"我叫约翰。", "我叫琼恩。", Fix{From: "约翰", To: "琼恩"}, true},
		{"Jon is here.", "John is here.", Fix{From: "Jon", To: "John"}, true},
		{"See you, Jon.", "See you, John.", Fix{From: "Jon", To: "John"}, true},
		{"the red barn", "the old red barn", Fix{}, false},
		{"meet the river delta", "meet the river deltas", Fix{From: "delta", To: "deltas"}, true},
		{"你好。", "你好！", Fix{}, false},
		{"一样", "一样", Fix{}, false},
		{"它们是曲流", "它们是", Fix{From: "曲流", To: ""}, true},
	} {
		got, ok := FixOf(c.before, c.after)
		if ok != c.ok || got != c.want {
			t.Errorf("FixOf(%q, %q) = %+v, %v; want %+v, %v", c.before, c.after, got, ok, c.want, c.ok)
		}
	}
}

// TestFixAppliesToWholeWords checks that a fix finds and replaces its words in another line: anywhere in Chinese, only as whole words in English, so fixing "Jon" leaves "Jonathan" alone.
func TestFixAppliesToWholeWords(t *testing.T) {
	jon := Fix{From: "Jon", To: "John"}
	for _, c := range []struct {
		text, want string
		in         bool
	}{
		{"Jon, wait.", "John, wait.", true},
		{"Ask Jon and Jon's sister.", "Ask John and John's sister.", true},
		{"Jonathan left.", "Jonathan left.", false},
		{"DeJon left.", "DeJon left.", false},
	} {
		if got := jon.In(c.text); got != c.in {
			t.Errorf("In(%q) = %v", c.text, got)
		}
		if got := jon.Apply(c.text); got != c.want {
			t.Errorf("Apply(%q) = %q, want %q", c.text, got, c.want)
		}
	}
	river := Fix{From: "河曲", To: "曲流"}
	if !river.In("这些河曲很美") || river.Apply("这些河曲很美，河曲") != "这些曲流很美，曲流" {
		t.Errorf("Chinese: %q", river.Apply("这些河曲很美，河曲"))
	}
}

// TestAFixThatTakesWordsOutTidiesTheLine checks that a fix taking words out takes a space beside them out too, and that OnLine tidies the spaces left and refuses to leave an original empty, as a cue with a translation alone reads back as an original.
func TestAFixThatTakesWordsOutTidiesTheLine(t *testing.T) {
	out := Fix{From: "Jon", To: ""}
	for _, c := range []struct{ text, want string }{
		{"Ask Jon.", "Ask."},
		{"Jon, wait.", ", wait."},
		{"Ask Jon and me", "Ask and me"},
		{"Jon Jon", ""},
		{"Jon", ""},
	} {
		if got := out.Apply(c.text); got != c.want {
			t.Errorf("Apply(%q) = %q, want %q", c.text, got, c.want)
		}
	}
	for _, c := range []struct {
		line     Line
		original bool
		want     string
		ok       bool
	}{
		{Line{Original: "Ask Jon.", Translation: "问约翰。"}, true, "Ask.", true},
		{Line{Original: "Jon", Translation: "约翰"}, true, "", false},
		{Line{Original: "Jonathan left.", Translation: "乔纳森走了。"}, true, "", false},
		{Line{Original: "Jon", Translation: "Jon"}, false, "", true},
	} {
		got, ok := out.OnLine(c.line, c.original)
		if got != c.want || ok != c.ok {
			t.Errorf("OnLine(%+v, %v) = %q, %v; want %q, %v", c.line, c.original, got, ok, c.want, c.ok)
		}
	}
	john := Fix{From: "约翰", To: "琼恩"}
	if got, ok := john.OnLine(Line{Original: "Jon is here.", Translation: "约翰来了。"}, false); got != "琼恩来了。" || !ok {
		t.Errorf("OnLine of a translation = %q, %v", got, ok)
	}
}
