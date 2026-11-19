package segment

import (
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"kando/packages/core"
)

// words are the words of tokens, in order.
func words(tokens []core.WordToken) []string {
	var out []string
	for _, t := range tokens {
		if t.Word {
			out = append(out, t.Text)
		}
	}
	return out
}

// joined is tokens joined again.
func joined(tokens []core.WordToken) string {
	var b strings.Builder
	for _, t := range tokens {
		b.WriteString(t.Text)
	}
	return b.String()
}

// cut is Words of line once its dictionary is read.
func cut(t *testing.T, line string) []core.WordToken {
	t.Helper()
	Load(line)
	got := Words(line, nil)
	if joined(got) != line {
		t.Fatalf("Words(%q) joins to %q", line, joined(got))
	}
	for i := 1; i < len(got); i++ {
		if !got[i-1].Word && !got[i].Word {
			t.Fatalf("Words(%q) has two runs between words in a row: %+v", line, got)
		}
	}
	return got
}

func TestJapaneseLinesAreCutIntoWords(t *testing.T) {
	cases := []struct {
		line string
		want []string
	}{
		{"今日は学校に行った。", []string{"今日", "は", "学校", "に", "行った"}},
		{"私は寿司を食べたい", []string{"私", "は", "寿司", "を", "食べたい"}},
		{"ありがとうございました！", []string{"ありがとう", "ございました"}},
		{"「コーヒーを飲みませんか？」", []string{"コーヒー", "を", "飲みません", "か"}},
		{"東京タワーへ行こう", []string{"東京", "タワー", "へ", "行こう"}},
	}
	for _, c := range cases {
		if got := words(cut(t, c.line)); !slices.Equal(got, c.want) {
			t.Errorf("Japanese %q cuts into %q, want %q", c.line, got, c.want)
		}
	}
}

func TestChineseLinesAreCutIntoWords(t *testing.T) {
	cases := []struct {
		line string
		want []string
	}{
		{"我们明天去北京看电影。", []string{"我们", "明天", "去", "北京", "看", "电影"}},
		{"你今天晚上有空吗？", []string{"你", "今天", "晚上", "有空", "吗"}},
		{"我們明天去看電影。", []string{"我們", "明天", "去", "看", "電影"}},
	}
	for _, c := range cases {
		if got := words(cut(t, c.line)); !slices.Equal(got, c.want) {
			t.Errorf("Chinese %q cuts into %q, want %q", c.line, got, c.want)
		}
	}
}

func TestOtherWordsAreCutAsBefore(t *testing.T) {
	// Latin among Chinese stays a word of its own, as core.WordTokens has it, and the Chinese around it is cut.
	if got := words(cut(t, "我在用iPhone拍照")); !slices.Equal(got, []string{"我", "在", "用", "iPhone", "拍照"}) {
		t.Errorf("a line of Chinese with a Latin word cuts into %q", got)
	}
	for _, line := range []string{"Every river begins as a trickle.", "안녕 세상", "สวัสดีครับ", "Don't—it's well-known, 42", ""} {
		if got, want := Words(line, nil), core.WordTokens(line); !slices.Equal(got, want) {
			t.Errorf("Words(%q) = %+v, want core.WordTokens's %+v", line, got, want)
		}
	}
}

// TestADictionaryIsReadOnceInTheBackground checks that a dictionary not read yet leaves its runs out of the words without waiting for it, reads it once however many ask meanwhile, and tells each that asked once it is read; from then on the runs are cut at once.
func TestADictionaryIsReadOnceInTheBackground(t *testing.T) {
	release := make(chan struct{})
	var reads sync.WaitGroup
	reads.Add(1)
	count := 0
	d := newDictionary(func() func(string) []string {
		count++
		reads.Done()
		<-release
		return func(run string) []string { return strings.Split(run, "") }
	})
	var told sync.WaitGroup
	told.Add(2)
	if d.cutter(told.Done) != nil || d.cutter(told.Done) != nil {
		t.Fatal("a dictionary not read yet cuts")
	}
	reads.Wait()
	close(release)
	done := make(chan struct{})
	go func() { told.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("those that asked were not told the dictionary is read")
	}
	cutter := d.cutter(func() { t.Error("told again once read") })
	if cutter == nil || count != 1 {
		t.Fatalf("once read: cutter %v, read %d times", cutter != nil, count)
	}
	got := cutRuns(core.WordTokens("Hi 你好。"), cutter)
	want := []core.WordToken{{Text: "Hi", Word: true}, {Text: " ", Word: false}, {Text: "你", Word: true}, {Text: "好", Word: true}, {Text: "。", Word: false}}
	if !slices.Equal(got, want) {
		t.Fatalf("cut %+v, want %+v", got, want)
	}
}

// TestACutThatIsNotTheRunIsLeftOut checks that a cut whose words do not join into the run again, as from a dictionary that could not be read, leaves the run out of the words, the line whole.
func TestACutThatIsNotTheRunIsLeftOut(t *testing.T) {
	got := cutRuns(core.WordTokens("Hi 你好"), func(string) []string { return nil })
	want := []core.WordToken{{Text: "Hi", Word: true}, {Text: " 你好", Word: false}}
	if !slices.Equal(got, want) {
		t.Fatalf("cut %+v, want %+v", got, want)
	}
}

func TestWordsCallsLoadedOnceTheDictionaryIsRead(t *testing.T) {
	line := "日本語を勉強しています"
	done := make(chan struct{})
	var once sync.Once
	first := Words(line, func() { once.Do(func() { close(done) }) })
	if joined(first) != line {
		t.Fatalf("Words(%q) joins to %q", line, joined(first))
	}
	if len(words(first)) == 0 {
		// Not read yet: it is read in the background and loaded is called.
		select {
		case <-done:
		case <-time.After(60 * time.Second):
			t.Fatal("loaded was not called")
		}
	}
	if got := words(Words(line, nil)); len(got) < 3 || got[0] != "日本語" {
		t.Fatalf("once read the line cuts into %q", got)
	}
}
