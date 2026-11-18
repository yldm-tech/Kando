package media

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kando/packages/core"
)

// TestApplyFixesTakesOnlyWordsInPlace checks that a proofreader's reply corrects what whisper misheard in a line and cannot reword it: words it adds, leaves out or rewrites in a longer run are dropped, while the corrections in the same reply are kept.
func TestApplyFixesTakesOnlyWordsInPlace(t *testing.T) {
	for _, tc := range []struct{ name, line, fixed, want string }{
		{"a homophone", "今天讲神经网罗", "今天讲神经网络", "今天讲神经网络"},
		{"two homophones, punctuation made full width", "先看输入层,再看神经网罗和梯度下将", "先看输入层：再看神经网络和梯度下降", "先看输入层，再看神经网络和梯度下降"},
		{"a character for a letter", "这个点叫做碧", "这个点叫做B", "这个点叫做B"},
		{"a correction inside a rewording", "再看神经网罗", "再来看看神经网络", "再看神经网络"},
		{"an added word", "这一层,它是输出层", "这一层，它就是输出层", "这一层，它是输出层"},
		{"a reworded run", "它呀,是数据教会的", "它是由数据教会的", "它呀，是数据教会的"},
		{"a run too long to be a mishearing", "训练模型的时候", "调教网络之际", "训练模型的时候"},
		{"no correction", "都听懂了吗", "", "都听懂了吗"},
		{"letters keep their commas", "三个输出呢,是A,B,C", "", "三个输出呢，是A,B,C"},
		{"an English word", "the teams own tests back that up", "the team's own tests back that up", "the team's own tests back that up"},
		{"English rewording", "it doesn't make up problems", "it does not invent problems", "it doesn't make up problems"},
		{"a line break in the reply", "今天讲神经网罗", "今天讲神经\n网络", "今天讲神经网络"},
		{"a Thai tone mark", "ฉันชอบกินขาวผัด", "ฉันชอบกินข้าวผัด", "ฉันชอบกินข้าวผัด"},
		{"Thai rewording", "ฉันชอบกินข้าวผัด", "ผมอยากทานก๋วยเตี๋ยว", "ฉันชอบกินข้าวผัด"},
		{"a Thai phrase with an English word", "เรียน python วันนี้", "เรียน Python วันนี้", "เรียน Python วันนี้"},
		{"a Hindi vowel sign", "मेरा नम राम है", "मेरा नाम राम है", "मेरा नाम राम है"},
	} {
		if got := ApplyFixes(tc.line, tc.fixed); got != tc.want {
			t.Errorf("%s: ApplyFixes(%q, %q) = %q, want %q", tc.name, tc.line, tc.fixed, got, tc.want)
		}
	}
}

// TestProofreadKeepsABatchItCannotUse proofreads 81 lines, two batches: the first comes back with two corrections, one of them put on a line it was not heard in, as a model that lost count writes; the second with a reply that is not corrections every time it is sent. The first batch's good correction is kept, the misplaced one dropped, and the second batch's line stays as whisper heard it.
func TestProofreadKeepsABatchItCannotUse(t *testing.T) {
	sends := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sends++
		var req chatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if !strings.Contains(req.Messages[0].Content, `"神经网络入门"`) {
			t.Errorf("the prompt does not name the title: %s", req.Messages[0].Content)
		}
		var lines []struct {
			ID   int    `json:"id"`
			Text string `json:"text"`
		}
		_ = json.Unmarshal([]byte(req.Messages[1].Content), &lines)
		reply := "not json"
		if len(lines) == proofreadBatch {
			reply = `[{"id": 1, "heard": "今天讲神经网罗", "text": "今天讲神经网络"}, {"id": 2, "heard": "梯度下将在后面", "text": "梯度下降在后面"}]`
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": reply}}}})
	}))
	defer server.Close()
	segments := make([]core.Segment, proofreadBatch+1)
	for i := range segments {
		segments[i].Text = "今天讲神经网罗"
	}
	segments[1].Text = "梯度下将在前面"
	got, err := ChatTranslator{BaseURL: server.URL, Model: "m"}.Proofread(context.Background(), segments, "神经网络入门")
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Text != "今天讲神经网络" || got[1].Text != "梯度下将在前面" || got[proofreadBatch].Text != "今天讲神经网罗" {
		t.Errorf("proofread lines %q, %q, %q", got[0].Text, got[1].Text, got[proofreadBatch].Text)
	}
	if segments[0].Text != "今天讲神经网罗" {
		t.Error("Proofread changed the segments it was given")
	}
	if sends != 1+maxBatchSends {
		t.Errorf("%d requests, want %d", sends, 1+maxBatchSends)
	}
}

// TestSpreadFixesCorrectsARecurringTerm checks that a correction the proofreader made in two lines is made in a third it left, while one made in a single line goes no further.
func TestSpreadFixesCorrectsARecurringTerm(t *testing.T) {
	heard := []core.Segment{{Text: "今天讲神经网罗"}, {Text: "再看神经网罗"}, {Text: "网罗一换,结果就变了"}, {Text: "往后倒一步"}, {Text: "倒一杯水"}}
	fixed := []core.Segment{{Text: "今天讲神经网络"}, {Text: "再看神经网络"}, {Text: "网罗一换，结果就变了"}, {Text: "往后退一步"}, {Text: "倒一杯水"}}
	spreadFixes(heard, fixed)
	want := []string{"今天讲神经网络", "再看神经网络", "网络一换，结果就变了", "往后退一步", "倒一杯水"}
	for i, s := range fixed {
		if s.Text != want[i] {
			t.Errorf("line %d = %q, want %q", i+1, s.Text, want[i])
		}
	}
}

// TestSpreadFixesKeepsTheWordsAroundACorrection checks that a correction made in two lines in the middle of a line spreads only where the words on both sides of it are the same: "神经网罗是" corrected twice corrects "那个网罗是" but not "张罗是", which shares only the word after it.
func TestSpreadFixesKeepsTheWordsAroundACorrection(t *testing.T) {
	heard := []core.Segment{{Text: "神经网罗是基础"}, {Text: "这个网罗是关键"}, {Text: "那个网罗是新的"}, {Text: "他张罗是为了吃饭"}}
	fixed := []core.Segment{{Text: "神经网络是基础"}, {Text: "这个网络是关键"}, {Text: "那个网罗是新的"}, {Text: "他张罗是为了吃饭"}}
	spreadFixes(heard, fixed)
	want := []string{"神经网络是基础", "这个网络是关键", "那个网络是新的", "他张罗是为了吃饭"}
	for i, s := range fixed {
		if s.Text != want[i] {
			t.Errorf("line %d = %q, want %q", i+1, s.Text, want[i])
		}
	}
}

// TestNormalizePunctuation checks which ASCII marks are written full width: those next to Chinese or Japanese characters or their punctuation, a run of marks after them as a whole, but not marks between letters and digits, as in times, numbers and addresses.
func TestNormalizePunctuation(t *testing.T) {
	for _, tc := range []struct{ line, want string }{
		{"好的,我们开始", "好的，我们开始"},
		{"你好!!", "你好！！"},
		{"真的?!", "真的？！"},
		{"「好」,", "「好」，"},
		{"（笑）,然后", "（笑），然后"},
		{"比分3:2吗", "比分3:2吗"},
		{"价格1,000元", "价格1,000元"},
		{"约1.5倍", "约1.5倍"},
		{"等等...", "等等..."},
		{"打开https://kando.app看看", "打开https://kando.app看看"},
		{"well, ok!", "well, ok!"},
	} {
		if got := NormalizePunctuation(tc.line); got != tc.want {
			t.Errorf("NormalizePunctuation(%q) = %q, want %q", tc.line, got, tc.want)
		}
	}
}
