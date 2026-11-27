package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"kando/packages/core"
)

// translateBatch is how many lines go into one request unless ChatTranslator.BatchSize says otherwise, keeping replies within common output token limits. MinBatchSize and MaxBatchSize bound what Settings › AI 服务 offers.
const (
	translateBatch = 40
	MinBatchSize   = 10
	MaxBatchSize   = 100
)

// DefaultBatchSize is how many lines go into one request when ChatTranslator.BatchSize is 0.
const DefaultBatchSize = translateBatch

// DefaultSubtitlePrompt is the style instruction used when ChatTranslator.Prompt is empty. {{language}} becomes the target language. The JSON reply rule is appended separately and is not part of this text.
const DefaultSubtitlePrompt = "You are a professional subtitle translator. Translate every subtitle line into {{language}}.\n- Keep each translation natural, concise and suitable for on-screen subtitles.\n- Lines may be sentence fragments that continue in the next line; translate them so they read naturally in order."

const subtitleReplyRule = "The user sends a JSON array of subtitle lines in order. Reply with only a JSON array of strings with exactly the same number of elements, each the translation of the line at the same index. Do not merge, split, or skip lines."

// contextLines is how many earlier lines of the same video go with a batch when context is on and ChatTranslator.ContextLines is 0. Subtitle text is short, but an entire film can still crowd a small model. MaxContextLines bounds what Settings › 翻译 offers.
const (
	contextLines    = 80
	MaxContextLines = 200
)

// DefaultContextLines is how many earlier lines go with a batch when ChatTranslator.ContextLines is 0.
const DefaultContextLines = contextLines

// ChatTranslator translates through an OpenAI-compatible /chat/completions endpoint, such as EveryAPI, OpenAI or a local Ollama server.
type ChatTranslator struct {
	BaseURL string
	APIKey  string
	Model   string
	// Prompt is the style instruction. {{language}} is replaced with the target language. Empty uses DefaultSubtitlePrompt.
	Prompt string
	// Context includes earlier lines of the same video with each batch, at most ContextLines of them, DefaultContextLines when 0.
	Context      bool
	ContextLines int
	// BatchSize is how many lines go into one request, DefaultBatchSize when 0.
	BatchSize int
	// Attempts is how many times a request is sent while it fails with a rate limit, a server error or no usable answer: the first time and Attempts-1 retries. 0 is DefaultRetries+1.
	Attempts int
	// Credential, when set, replaces BaseURL and APIKey for each translation. renew asks for a new key after the API rejected the previous one.
	Credential func(ctx context.Context, renew bool) (Endpoint, error)
	// Client defaults to an http.Client with a two-minute timeout.
	Client *http.Client
	// Offline refuses, with ErrOffline and before anything is sent, every request to a service that is not on this computer (Local), as offline mode asks.
	Offline bool
}

// ErrNoModel and ErrNoBaseURL are what Ping and Chat return before they send anything, when the model or the endpoint is not set.
var (
	ErrNoModel   = errors.New("model is empty")
	ErrNoBaseURL = errors.New("base URL is empty")
)

// Endpoint is where translation requests go and the bearer key they carry.
type Endpoint struct {
	BaseURL string
	APIKey  string
}

// Ping checks that the chat endpoint accepts one short request with the configured model.
func (t ChatTranslator) Ping(ctx context.Context) error {
	if strings.TrimSpace(t.Model) == "" {
		return ErrNoModel
	}
	if err := t.offline(); err != nil {
		return err
	}
	endpoint := Endpoint{BaseURL: strings.TrimSpace(t.BaseURL), APIKey: t.APIKey}
	if t.Credential != nil {
		var err error
		if endpoint, err = t.Credential(ctx, false); err != nil {
			return err
		}
	}
	if strings.TrimSpace(endpoint.BaseURL) == "" {
		return ErrNoBaseURL
	}
	body, err := json.Marshal(chatRequest{
		Model:       t.Model,
		Temperature: 0,
		Messages:    []chatMessage{{Role: "user", Content: "ping"}},
	})
	if err != nil {
		return err
	}
	client := t.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	_, _, _, err = completeOnce(ctx, client, endpoint, body, nil)
	return err
}

func (t ChatTranslator) Translate(ctx context.Context, segments []core.Segment, language string) ([]core.Segment, error) {
	if strings.TrimSpace(language) == "" {
		return nil, fmt.Errorf("target language is empty")
	}
	if err := t.offline(); err != nil {
		return nil, err
	}
	endpoint := Endpoint{BaseURL: t.BaseURL, APIKey: t.APIKey}
	if t.Credential != nil {
		var err error
		if endpoint, err = t.Credential(ctx, false); err != nil {
			return nil, err
		}
	}
	renewed := false
	out := make([]core.Segment, 0, len(segments))
	// The lines said before segments, as a film translated a part at a time gives them (WithEarlier), go with the first batch as context, as the batches before it would, and each batch goes on to the next with its translations.
	earlier := slices.Clone(earlierOf(ctx))
	title := titleOf(ctx)
	// untranslated counts the lines of batches left untranslated, failedRun the batches in a row that failed, and lastErr why the last of them did.
	var untranslated, failedRun int
	var lastErr error
	size := t.batchSize()
	batches := (len(segments) + size - 1) / size
	for start := 0; start < len(segments); start += size {
		batch := segments[start:min(start+size, len(segments))]
		progress := core.Fraction(float64(start) / float64(len(segments)))
		progress.Batch, progress.Batches, progress.BatchSize = start/size+1, batches, size
		progress.Lines, progress.Of = start, len(segments)
		texts := make([]string, len(batch))
		for i, s := range batch {
			texts[i] = s.Text
		}
		reportProgress(ctx, progress)
		var contextSoFar []Earlier
		if n := t.contextLines(); n > 0 {
			contextSoFar = earlier[max(0, len(earlier)-n):]
		}
		batchCtx := withLogSpan(ctx, batch)
		lines, err := t.translateTexts(batchCtx, endpoint, texts, language, title, contextSoFar, progress)
		var status *statusError
		if t.Credential != nil && !renewed && errors.As(err, &status) && status.code == http.StatusUnauthorized {
			renewed = true
			if endpoint, err = t.Credential(ctx, true); err != nil {
				return nil, err
			}
			lines, err = t.translateTexts(batchCtx, endpoint, texts, language, title, contextSoFar, progress)
		}
		var unfinished *unfinishedError
		if errors.As(err, &unfinished) {
			// Sent again in halves, part of the batch came back: its lines are kept and those of the part that failed left untranslated.
			logf(batchCtx, "translate: %d lines left untranslated, a half of batch %d of %d failed: %v", unfinished.lines, start/size+1, batches, unfinished.err)
			untranslated += unfinished.lines
			lastErr, err = unfinished.err, nil
		}
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			// A rejected key, model or address fails every batch the same way, as do batches that fail one after another; one batch that fails after its retries is left untranslated and the rest of the film goes on.
			failedRun++
			if !skippableBatch(err) || failedRun >= maxFailedBatches {
				logf(batchCtx, "translate: stopped at batch %d of %d, %d failed in a row: %v", start/size+1, batches, failedRun, err)
				return nil, err
			}
			logf(batchCtx, "translate: batch %d of %d left untranslated: %v", start/size+1, batches, err)
			lastErr = err
			untranslated += len(batch)
			lines = make([]string, len(batch))
		} else {
			failedRun = 0
			if empty := emptyReplies(texts, lines); empty > 0 {
				logf(batchCtx, "translate: the reply of batch %d of %d left %d lines empty", start/size+1, batches, empty)
			}
			if sentBack(texts, lines, language) {
				logf(batchCtx, "translate: batch %d of %d kept with most of its lines sent back in their own language", start/size+1, batches)
			}
		}
		for i, s := range batch {
			earlier = append(earlier, Earlier{Line: s.Text, Translation: strings.TrimSpace(lines[i])})
			s.Text = lines[i]
			out = append(out, s)
		}
	}
	if untranslated == len(segments) && lastErr != nil {
		return nil, lastErr
	}
	reportProgress(ctx, core.Fraction(1))
	if untranslated > 0 {
		return out, &UntranslatedError{Lines: untranslated, Total: len(segments), Err: lastErr}
	}
	return out, nil
}

// emptyReplies is how many of the translations of texts, lines, are empty where their line is not: a reply of the batch's length the model left lines of blank in, which stay untranslated.
func emptyReplies(texts, lines []string) int {
	n := 0
	for i, text := range texts {
		if strings.TrimSpace(lines[i]) == "" && strings.TrimSpace(text) != "" {
			n++
		}
	}
	return n
}

// transient reports whether err is a request that failed as a service does for a while: a rate limit that does not say the account is out of credit, a server error or a timeout the API answered with, or a request that timed out or lost its connection. A reply that could not be read, a key, model or address the API rejects and a source of the user's own (Tools.Engine) are not, nor lines left untranslated or unproofread while the rest came back (UntranslatedError, UnproofreadError), which are kept as they are.
func transient(err error) bool {
	var engine *engineError
	var untranslated *UntranslatedError
	var unproofread *UnproofreadError
	if err == nil || errors.As(err, &engine) || errors.Is(err, ErrOffline) || errors.As(err, &untranslated) || errors.As(err, &unproofread) {
		return false
	}
	var status *statusError
	if errors.As(err, &status) {
		return !outOfCredit(status.code, status.detail) && (status.code == http.StatusTooManyRequests || status.code == http.StatusRequestTimeout || status.code >= 500)
	}
	var urlErr *url.Error
	var netErr net.Error
	return errors.As(err, &urlErr) || errors.As(err, &netErr)
}

// maxFailedBatches is how many batches in a row may fail before the translation stops: that many are the service's trouble, not their lines'.
const maxFailedBatches = 3

// UntranslatedError is what Translate returns, with every line, when it left some lines untranslated (an empty Text) because their batch still failed after it was tried again. Lines of Total are untranslated; Err is why the last of their batches failed.
type UntranslatedError struct {
	Lines, Total int
	Err          error
}

func (e *UntranslatedError) Error() string {
	return fmt.Sprintf("%d of %d lines were left untranslated: %v", e.Lines, e.Total, e.Err)
}

func (e *UntranslatedError) Unwrap() error { return e.Err }

// skippableBatch reports whether a batch that failed with err can be left untranslated while the rest of the film goes on: a reply that could not be read or had the wrong lines, a request that timed out or lost its connection, a rate limit or a server error that outlasted its retries. A key, model or address the API rejects is not, nor an account out of credit: they fail every batch.
func skippableBatch(err error) bool {
	var status *statusError
	if errors.As(err, &status) {
		if outOfCredit(status.code, status.detail) {
			return false
		}
		return status.code == http.StatusTooManyRequests || status.code == http.StatusRequestTimeout || status.code >= 500
	}
	return true
}

// Chat sends one conversation, a system instruction and the user's message, to the same endpoint and model translations use, and returns the model's reply without the code fence a model may wrap around it. A key the API rejects is renewed once through Credential, as Translate does; rate limits, server errors, timeouts and lost connections are retried as for a translation. It is ChatWith without a token limit or headers of its own, its usage left out.
func (t ChatTranslator) Chat(ctx context.Context, system, user string) (string, error) {
	reply, err := t.ChatWith(ctx, ChatRequest{System: system, User: user})
	// Chat asks for no limit, and has always given what the model wrote before it stopped.
	if errors.Is(err, ErrReplyCutOff) {
		err = nil
	}
	return reply.Text, err
}

// ErrReplyCutOff is a reply the model stopped writing because it reached its token limit (finish_reason "length"), ChatRequest.MaxTokens or the model's own; ChatWith returns it with the text written so far and the tokens taken.
var ErrReplyCutOff = errors.New("the reply was cut off at its token limit")

// ChatRequest is one conversation ChatWith sends: the system instruction and the user's message; MaxTokens, when above 0, caps the reply's tokens (max_tokens); Header adds headers to the request, such as X-Title, which some services show on the user's bill. Header cannot replace Content-Type or Authorization, which ChatWith sets.
type ChatRequest struct {
	System, User string
	MaxTokens    int
	Header       http.Header
}

// ChatReply is the model's reply to a ChatRequest, without the code fence a model may wrap around it, and the tokens it took.
type ChatReply struct {
	Text  string
	Usage ChatUsage
}

// ChatUsage is the tokens a conversation took: those of the request (Prompt) and of the reply (Completion), as the service counted them, or, when it said nothing of them, as estimated from their characters (Estimated, EstimateTokens).
type ChatUsage struct {
	Prompt, Completion int
	Estimated          bool
}

// Total is all the tokens the conversation took.
func (u ChatUsage) Total() int { return u.Prompt + u.Completion }

// ChatWith sends the conversation r, as Chat does, and returns the reply with the tokens it took: those the service's usage says, or, when it says none, an estimate from the characters sent and received (EstimateTokens). A key the API rejects is renewed once through Credential; rate limits, server errors, timeouts and lost connections are retried as for a translation. Every request that was written to the service and got no reply it counted, a retry's, a rejected key's, one that timed out or was stopped after it went, counts the estimate of its prompt, as the service may bill it; so the usage is returned with an error too. A reply cut off at its token limit is ErrReplyCutOff, with its text and usage.
func (t ChatTranslator) ChatWith(ctx context.Context, r ChatRequest) (ChatReply, error) {
	if strings.TrimSpace(t.Model) == "" {
		return ChatReply{}, ErrNoModel
	}
	if err := t.offline(); err != nil {
		return ChatReply{}, err
	}
	endpoint := Endpoint{BaseURL: strings.TrimSpace(t.BaseURL), APIKey: t.APIKey}
	if t.Credential != nil {
		var err error
		if endpoint, err = t.Credential(ctx, false); err != nil {
			return ChatReply{}, err
		}
	}
	if strings.TrimSpace(endpoint.BaseURL) == "" {
		return ChatReply{}, ErrNoBaseURL
	}
	body, err := json.Marshal(chatRequest{
		Model:     t.Model,
		Messages:  []chatMessage{{Role: "system", Content: r.System}, {Role: "user", Content: r.User}},
		MaxTokens: max(r.MaxTokens, 0),
	})
	if err != nil {
		return ChatReply{}, err
	}
	sent := 0
	prompt := EstimateTokens(r.System) + EstimateTokens(r.User)
	// unanswered is the usage of the requests sent that got no reply the service counted: the estimate of their prompts.
	unanswered := func(n int) ChatUsage {
		if n <= 0 {
			return ChatUsage{}
		}
		return ChatUsage{Prompt: n * prompt, Estimated: true}
	}
	reply, err := t.completeWith(ctx, endpoint, body, core.Progress{}, r.Header, &sent)
	var status *statusError
	if t.Credential != nil && errors.As(err, &status) && status.code == http.StatusUnauthorized {
		if endpoint, err = t.Credential(ctx, true); err != nil {
			return ChatReply{Usage: unanswered(sent)}, err
		}
		reply, err = t.completeWith(ctx, endpoint, body, core.Progress{}, r.Header, &sent)
	}
	if err != nil {
		return ChatReply{Usage: unanswered(sent)}, err
	}
	if len(reply.Choices) == 0 {
		return ChatReply{Usage: unanswered(sent)}, errors.New("chat API returned no choices")
	}
	content := reply.Choices[0].Message.Content
	out := ChatReply{Text: stripCodeFence(content), Usage: unanswered(sent - 1)}
	if u := reply.Usage; u != nil && (u.PromptTokens > 0 || u.CompletionTokens > 0) {
		out.Usage.Prompt += u.PromptTokens
		out.Usage.Completion = u.CompletionTokens
	} else {
		out.Usage = ChatUsage{Prompt: out.Usage.Prompt + prompt, Completion: EstimateTokens(content), Estimated: true}
	}
	if reply.Choices[0].FinishReason == "length" {
		return out, ErrReplyCutOff
	}
	return out, nil
}

// EstimateTokens is about how many tokens text takes, for a service that does not say: a token for every four characters of ASCII, as English and code run, and one for each other character, as Chinese, Japanese and Korean take about one a character; at least one for text that is not empty. It errs high for most other scripts, which is the safe side for a budget.
func EstimateTokens(text string) int {
	ascii, other := 0, 0
	for _, r := range text {
		if r < utf8.RuneSelf {
			ascii++
		} else {
			other++
		}
	}
	n := other + (ascii+3)/4
	if n == 0 && text != "" {
		n = 1
	}
	return n
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
		// FinishReason is why the model stopped: "length" at its token limit.
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	// Usage is the tokens the request and the reply took, as OpenAI-compatible services count them; nil from a service that says nothing of them.
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// statusError is a non-200 reply from the translation API.
type statusError struct {
	code   int
	status string
	detail string
}

// StatusCode is the HTTP status the API answered with.
func (e *statusError) StatusCode() int { return e.code }

func (e *statusError) Error() string {
	return fmt.Sprintf("translation API returned %s: %s", e.status, e.detail)
}

// subtitlePrompt is the system instruction a batch is sent with: the style instruction custom names, the video's title, the glossary terms its lines hold, then the lines said before it, each with its translation so far when it has one, which the batch writes names, terms and forms of address as, and the reply rule, which asks for every line in language: a small model sent lines of a script close to the target's, as Japanese for Chinese, otherwise sends whole batches back as they are, more so when the earlier lines it is shown are originals alone. The glossary goes before the earlier lines, and says what they cannot: a term it locks or prefers is written as it says even where an earlier translation wrote it otherwise.
func subtitlePrompt(custom, language, title string, earlier []Earlier, terms []core.Term) string {
	prompt := stylePrompt(custom, language)
	if title = strings.TrimSpace(title); title != "" {
		encoded, _ := json.Marshal(title)
		prompt += "\n\nThe video is titled " + string(encoded) + " (its title or its file name), for context."
	}
	if glossary := glossaryPrompt(terms); glossary != "" {
		prompt += "\n\n" + glossary
	}
	if len(earlier) > 0 {
		// Each earlier line is [line] or [line, translation], which takes about half the tokens of an object with named fields.
		pairs := make([][]string, len(earlier))
		for i, e := range earlier {
			pairs[i] = []string{e.Line}
			if e.Translation != "" {
				pairs[i] = append(pairs[i], e.Translation)
			}
		}
		encoded, _ := json.Marshal(pairs)
		prompt += "\n\nEarlier lines from the same video, each with its translation so far when it has one, for context only. Do not translate them again. Write names, terms and forms of address as these translations do, unless the glossary says otherwise:\n" + string(encoded)
	}
	return prompt + "\n\n" + replyRule(language)
}

// replyRule is the reply rule a batch translated into language ends with.
func replyRule(language string) string {
	return subtitleReplyRule + " Write every element in " + language + ", even a line that is short or a name; never send a line back as it is."
}

// stylePrompt is the style instruction custom, or DefaultSubtitlePrompt for none, naming language as the one to translate into.
func stylePrompt(custom, language string) string {
	prompt := strings.TrimSpace(custom)
	if prompt == "" {
		prompt = DefaultSubtitlePrompt
	}
	if strings.Contains(prompt, "{{language}}") {
		prompt = strings.ReplaceAll(prompt, "{{language}}", language)
	} else {
		// A custom prompt that only asks for a style would otherwise leave the target language unnamed, and the model answers in the language of the lines; naming it here also follows a job's own target language, which a language written into the prompt would not.
		prompt += "\n\nTranslate every subtitle line into " + language + "."
	}
	return prompt
}

// maxBatchSends is how many times one batch is sent while its reply is not the batch's lines: not a JSON array of them, or too few or too many.
const maxBatchSends = 3

// translateTexts sends one batch, again while the reply is not its lines, up to maxBatchSends times. progress is where the translation stands while it is sent, which a retry's wait is reported with.
func (t ChatTranslator) translateTexts(ctx context.Context, endpoint Endpoint, texts []string, language, title string, earlier []Earlier, progress core.Progress) ([]string, error) {
	return t.translateBatch(ctx, endpoint, texts, language, title, earlier, progress, false)
}

// translateBatch is translateTexts, of a half of a batch sent again in halves (translateHalves) when half is set: a half whose reply is still not its lines is not sent again in halves itself, but for a reply cut off or a few lines short, so a model that writes every reply wrong is sent the batch a bounded number of times.
func (t ChatTranslator) translateBatch(ctx context.Context, endpoint Endpoint, texts []string, language, title string, earlier []Earlier, progress core.Progress, half bool) ([]string, error) {
	// A batch sent again in halves takes the first half as context for the second, which keeps to the lines context allows when it is on.
	if n := t.contextLines(); n > 0 && len(earlier) > n {
		earlier = earlier[len(earlier)-n:]
	}
	input, err := json.Marshal(texts)
	if err != nil {
		return nil, err
	}
	system := subtitlePrompt(t.Prompt, language, title, earlier, core.TermsIn(glossaryOf(ctx), texts))
	request := func(system string) ([]byte, error) {
		return json.Marshal(chatRequest{
			Model:    t.Model,
			Messages: []chatMessage{{Role: "system", Content: system}, {Role: "user", Content: string(input)}},
		})
	}
	body, err := request(system)
	if err != nil {
		return nil, err
	}

	for send := 1; ; send++ {
		reply, err := t.complete(ctx, endpoint, body, progress)
		if err != nil {
			return nil, err
		}
		lines, err := replyLines(reply, texts)
		// A reply that sends most of its lines back in the language they are in is sent again, saying so; the last one is taken as it is, as a line the model keeps so, as a name, is not one it cannot translate.
		if err == nil && send < maxBatchSends && ctx.Err() == nil && sentBack(texts, lines, language) {
			logf(ctx, "translate: reply %d of %d sent most lines back in their own language, sending it again", send, maxBatchSends)
			if body, err = request(system + "\n\n" + fmt.Sprintf(sentBackAgain, language)); err != nil {
				return nil, err
			}
			continue
		}
		if err == nil {
			return lines, nil
		}
		// A reply that stopped before its end, at the model's token limit or where a gateway ended it, is the batch sent again in halves at once: each half's reply is half as long, and the same batch sent whole is most often cut off again. One cut off before it wrote a line, as a reasoning model's that spent its whole budget thinking, says nothing of a shorter reply fitting: it is sent again whole like any other reply that is not the batch's lines, so such a model fails the batch after maxBatchSends sends rather than be sent its every line alone.
		if len(texts) > 1 && ctx.Err() == nil && cutOff(reply) && cutOffLines(reply) > 0 {
			logf(ctx, "translate: the reply for %d lines was cut off after %d of them (finish reason %q); sending it again in halves", len(texts), cutOffLines(reply), finishReason(reply))
			return t.translateHalves(ctx, endpoint, texts, language, title, earlier, progress)
		}
		if send == maxBatchSends || ctx.Err() != nil {
			// Models occasionally omit only a few lines from a large response.
			// Retry those responses as smaller batches so one malformed reply does
			// not discard an otherwise usable translation.
			var countErr *lineCountError
			if errors.As(err, &countErr) && recoverableLineCount(countErr.got, countErr.want) {
				logf(ctx, "translate: %v after %d sends; sending it again in halves", err, send)
				return t.translateHalves(ctx, endpoint, texts, language, title, earlier, progress)
			}
			// Any other reply that set out to be the batch's lines and is not, as an array a model wrote wrong or with two lines merged into one, is the batch sent once in halves, each a shorter reply the model may write right. A reply that is no array at all, as a refusal, is the model's answer to any batch, and is not.
			if !half && len(texts) > 1 && ctx.Err() == nil && len(reply.Choices) > 0 && strings.HasPrefix(stripCodeFence(reply.Choices[0].Message.Content), "[") {
				logf(ctx, "translate: %v after %d sends; sending it again in halves", err, send)
				return t.translateHalves(ctx, endpoint, texts, language, title, earlier, progress)
			}
			return lines, err
		}
		logf(ctx, "translate: reply %d of %d was not the batch's lines (finish reason %q), sending it again: %v", send, maxBatchSends, finishReason(reply), err)
	}
}

// finishReason is why the model said reply ended ("stop", "length", "content_filter"), "" when it said nothing.
func finishReason(reply chatResponse) string {
	if len(reply.Choices) == 0 {
		return ""
	}
	return reply.Choices[0].FinishReason
}

// translateHalves translates texts as two batches, its first half then its second, which takes the first half's translations as context, as the batches before it would. A half that fails as a batch may (skippableBatch) is left untranslated while the other is kept, which it says with an *unfinishedError; both failing is the batch failing.
func (t ChatTranslator) translateHalves(ctx context.Context, endpoint Endpoint, texts []string, language, title string, earlier []Earlier, progress core.Progress) ([]string, error) {
	mid := len(texts) / 2
	left := 0
	var lastErr error
	half := func(texts []string, earlier []Earlier) ([]string, error) {
		lines, err := t.translateBatch(ctx, endpoint, texts, language, title, earlier, progress, true)
		var unfinished *unfinishedError
		switch {
		case err == nil:
			return lines, nil
		case errors.As(err, &unfinished):
			left, lastErr = left+unfinished.lines, unfinished.err
			return lines, nil
		case ctx.Err() != nil || !skippableBatch(err):
			return nil, err
		}
		left, lastErr = left+len(texts), err
		return make([]string, len(texts)), nil
	}
	first, err := half(texts[:mid], earlier)
	if err != nil {
		return nil, err
	}
	rightEarlier := slices.Clone(earlier)
	for i, text := range texts[:mid] {
		rightEarlier = append(rightEarlier, Earlier{Line: text, Translation: strings.TrimSpace(first[i])})
	}
	second, err := half(texts[mid:], rightEarlier)
	switch {
	case err != nil:
		return nil, err
	case left == len(texts):
		return nil, lastErr
	case left > 0:
		return append(first, second...), &unfinishedError{lines: left, err: lastErr}
	}
	return append(first, second...), nil
}

// unfinishedError is a batch sent again in halves (translateHalves) whose translations came back but for lines of a part that failed, left empty; err is why the last such part failed.
type unfinishedError struct {
	lines int
	err   error
}

func (e *unfinishedError) Error() string {
	return fmt.Sprintf("%d lines were left untranslated: %v", e.lines, e.err)
}

func (e *unfinishedError) Unwrap() error { return e.err }

// cutOff reports whether reply stopped before its end: the model reached its token limit (finish_reason "length"), or the reply is a JSON array that ends before it is closed, as a gateway that ends a long reply early leaves it.
func cutOff(reply chatResponse) bool {
	if len(reply.Choices) == 0 {
		return false
	}
	if reply.Choices[0].FinishReason == "length" {
		return true
	}
	text := stripCodeFence(reply.Choices[0].Message.Content)
	if !strings.HasPrefix(text, "[") {
		return false
	}
	var raw []json.RawMessage
	var syntax *json.SyntaxError
	return errors.As(json.Unmarshal([]byte(text), &raw), &syntax) && syntax.Offset >= int64(len(text))
}

// cutOffLines is how many lines a reply cut off before its end (cutOff) wrote whole and not empty before it stopped: the elements of its JSON array, from its first "[" as a reply cut off may not have closed what it began with, up to the one it stopped in.
func cutOffLines(reply chatResponse) int {
	if len(reply.Choices) == 0 {
		return 0
	}
	text := stripCodeFence(reply.Choices[0].Message.Content)
	start := strings.IndexByte(text, '[')
	if start < 0 {
		return 0
	}
	dec := json.NewDecoder(strings.NewReader(text[start:]))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('[') {
		return 0
	}
	n := 0
	for dec.More() {
		var element json.RawMessage
		if dec.Decode(&element) != nil {
			break
		}
		if s := strings.TrimSpace(string(element)); s != `""` && s != "[]" && s != "null" {
			n++
		}
	}
	return n
}

// sentBackAgain is what a batch sent again after its reply sent its lines back untranslated (sentBack) adds to its instruction; %s is the language it is translated into.
const sentBackAgain = "Your last reply sent these lines back in their own language. Translate each of them into %s."

// sentBack reports whether the translations of texts into language, lines, are most of them the lines themselves sent back: of the lines written in a script only their language writes, kana for Japanese, Hangul for Korean, more than half came back mostly in it, while language, a target Kando knows (core.TargetTag), writes none of it. A small model shown lines of a script close to the target's, as Japanese for Chinese, sends whole batches back so, retouched more often than not. A script two languages share, as Latin or Chinese characters, tells nothing, so a name or a line two scripts write alike is never taken for one sent back.
func sentBack(texts, lines []string, language string) bool {
	base := core.BaseLanguage(core.TargetTag(language))
	if base == "" {
		return false
	}
	for _, script := range ownScripts {
		if script.language == base {
			continue
		}
		written, back := 0, 0
		for i, text := range texts {
			if !strings.ContainsFunc(text, script.in) {
				continue
			}
			written++
			in, letters := 0, 0
			for _, r := range lines[i] {
				if unicode.IsLetter(r) {
					letters++
					if script.in(r) {
						in++
					}
				}
			}
			if in*2 > letters {
				back++
			}
		}
		if back > 0 && back*2 > written {
			return true
		}
	}
	return false
}

// ownScripts are the scripts only one language writes, by that language's code: a translation into another language that is mostly in one is its line sent back (sentBack).
var ownScripts = []struct {
	language string
	in       func(rune) bool
}{
	{"ja", func(r rune) bool { return unicode.In(r, unicode.Hiragana, unicode.Katakana) }},
	{"ko", func(r rune) bool { return unicode.Is(unicode.Hangul, r) }},
}

// recoverableLineCount identifies a nearly complete response worth retrying as
// smaller requests. A one-line reply for a two-line batch is usually an
// instruction failure, and splitting it would duplicate an unknown translation.
func recoverableLineCount(got, want int) bool {
	return want > 2 && got > 1 && got*2 >= want
}

// replyLines reads the translations of a batch's lines, texts, from the model's reply: a JSON array of as many strings, maybe in a code fence. An element that is an array of strings, as a model writes for a half sentence split across two lines, is that line's translation in parts, which are joined; but one that starts with the line itself is the [line, translation] of the earlier lines the batch was sent with (subtitlePrompt), copied, which is not a translation.
func replyLines(reply chatResponse, texts []string) ([]string, error) {
	n := len(texts)
	if len(reply.Choices) == 0 {
		return nil, fmt.Errorf("translation API returned no choices")
	}
	var raw []json.RawMessage
	if err := json.Unmarshal([]byte(stripCodeFence(reply.Choices[0].Message.Content)), &raw); err != nil {
		return nil, fmt.Errorf("translation is not a JSON array of strings: %w", err)
	}
	if len(raw) != n {
		return nil, &lineCountError{got: len(raw), want: n}
	}
	lines := make([]string, n)
	for i, element := range raw {
		if err := json.Unmarshal(element, &lines[i]); err == nil {
			continue
		}
		var parts []string
		if err := json.Unmarshal(element, &parts); err != nil || len(parts) > 1 && strings.TrimSpace(parts[0]) == strings.TrimSpace(texts[i]) {
			return nil, fmt.Errorf("translation is not a JSON array of strings: line %d is %s", i+1, jsonKind(element))
		}
		lines[i] = joinParts(parts)
	}
	return lines, nil
}

// jsonKind is what element, the reply for a line that is not a string, is, as an error says it without its words, which the log does not keep: an array that starts with its line (the [line, translation] of an earlier line copied), or a JSON value of another kind than a string.
func jsonKind(element json.RawMessage) string {
	var parts []string
	if json.Unmarshal(element, &parts) == nil {
		return fmt.Sprintf("an array of %d strings that starts with its line", len(parts))
	}
	switch strings.TrimSpace(string(element))[:1] {
	case "[":
		return "an array"
	case "{":
		return "an object"
	case "n":
		return "null"
	case "t", "f":
		return "a boolean"
	}
	return "a number"
}

type lineCountError struct {
	got, want int
}

func (e *lineCountError) Error() string {
	return fmt.Sprintf("translation returned %d lines for %d", e.got, e.want)
}

// joinParts joins the parts of one translated line: with nothing between Chinese or Japanese text, which writes no spaces, and with a space between words otherwise.
func joinParts(parts []string) string {
	var b strings.Builder
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if b.Len() > 0 {
			last, _ := utf8.DecodeLastRuneInString(b.String())
			first, _ := utf8.DecodeRuneInString(part)
			if !unspaced(last) && !unspaced(first) {
				b.WriteByte(' ')
			}
		}
		b.WriteString(part)
	}
	return b.String()
}

// unspaced reports whether r belongs to a script written without spaces between words, or is the full-width punctuation that goes with it.
func unspaced(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Thai) || (r >= 0x3000 && r <= 0x303F) || (r >= 0xFF00 && r <= 0xFFEF)
}

// TranslateFail is why a translation failed, read from its error by ClassifyTranslate, so the card can say it in the user's words. The empty kind is a failure it does not recognize.
type TranslateFail string

const (
	// TranslateAuth is a key the API rejected (401 or 403), even after it was renewed.
	TranslateAuth TranslateFail = "auth"
	// TranslateRejected is a request the API refused for another reason of the client's, as a model or an address it does not know.
	TranslateRejected TranslateFail = "rejected"
	// TranslateQuota is an account the API says is out of credit or over its quota (402, or a 403 or 429 that says so), which no retry mends.
	TranslateQuota TranslateFail = "quota"
	// TranslateBusy is a rate limit or a server error that outlasted the retries.
	TranslateBusy TranslateFail = "busy"
	// TranslateTimeout is a request that got no answer in time, after the retries.
	TranslateTimeout TranslateFail = "timeout"
	// TranslateNetwork is a request that could not reach the API or lost its connection, after the retries.
	TranslateNetwork TranslateFail = "network"
	// TranslateMalformed is a reply that was not the batch's lines, after the batch was sent again.
	TranslateMalformed TranslateFail = "malformed"
)

// statusPattern finds the HTTP status in a statusError's text.
var statusPattern = regexp.MustCompile(`translation API returned (\d{3}) `)

// creditSigns are what services write in a 403 or 429 when the account is out of credit or over its quota rather than sending too fast: OpenAI's insufficient_quota, Moonshot's exceeded_current_quota_error, billing, balance and credit, DashScope's Arrearage, and Zhipu's 余额 (balance) and 欠费 (in arrears).
var creditSigns = []string{"insufficient_quota", "insufficient quota", "exceeded_current_quota", "billing", "balance", "credit", "arrearage", "余额", "欠费"}

// outOfCredit reports whether an API answer with status code and body detail says the account is out of credit or over its quota: always for 402 Payment Required, and for a 403 or 429 whose body says so.
func outOfCredit(code int, detail string) bool {
	switch code {
	case http.StatusPaymentRequired:
		return true
	case http.StatusForbidden, http.StatusTooManyRequests:
		detail = strings.ToLower(detail)
		for _, sign := range creditSigns {
			if strings.Contains(detail, sign) {
				return true
			}
		}
	}
	return false
}

// translateSigns are the phrases of the errors this file and net/http give, in the order they are tried: a timeout while a reply's body was read is a timeout before it is a reply that could not be decoded, and a connection cut off while it was read is the network's.
var translateSigns = []struct {
	kind    TranslateFail
	phrases []string
}{
	{TranslateTimeout, []string{"Client.Timeout", "deadline exceeded", "i/o timeout", "TLS handshake timeout", "timeout awaiting response headers"}},
	{TranslateNetwork, []string{"no such host", "connection refused", "connection reset", "network is unreachable", "no route to host", "broken pipe", "server misbehaving", "unexpected EOF", ": EOF"}},
	{TranslateMalformed, []string{"not a JSON array of strings", "lines for", "returned no choices", "decode translation response"}},
}

// AIAnswered reports whether msg, an error's text, holds an AI service's answer other than 200 (a statusError, as ClassifyTranslate reads its status): what the service wrote back is in it, which may hold its prices or a link to buy credit.
func AIAnswered(msg string) bool { return statusPattern.MatchString(msg) }

// ClassifyTranslate reads why a translation failed from its error's text, which is what a failed job keeps once it has been saved and read back. It returns "" for an error it does not recognize.
func ClassifyTranslate(msg string) TranslateFail {
	if m := statusPattern.FindStringSubmatchIndex(msg); m != nil {
		code, _ := strconv.Atoi(msg[m[2]:m[3]])
		switch {
		case outOfCredit(code, msg[m[1]:]):
			return TranslateQuota
		case code == http.StatusUnauthorized || code == http.StatusForbidden:
			return TranslateAuth
		case code == http.StatusTooManyRequests || code >= 500:
			return TranslateBusy
		case code == http.StatusRequestTimeout:
			return TranslateTimeout
		}
		return TranslateRejected
	}
	for _, sign := range translateSigns {
		for _, phrase := range sign.phrases {
			if strings.Contains(msg, phrase) {
				return sign.kind
			}
		}
	}
	return ""
}

const (
	maxAttempts   = 3
	maxRetryAfter = 30 * time.Second
)

// DefaultRetries is how many times a failed request is sent again when ChatTranslator.Attempts is 0; MaxRetries bounds what Settings › AI 服务 offers.
const (
	DefaultRetries = maxAttempts - 1
	MaxRetries     = 6
)

// attempts is how many times a request is sent at most (Attempts).
func (t ChatTranslator) attempts() int {
	if t.Attempts > 0 {
		return t.Attempts
	}
	return maxAttempts
}

// batchSize is how many lines go into one request (BatchSize).
func (t ChatTranslator) batchSize() int {
	if t.BatchSize > 0 {
		return t.BatchSize
	}
	return translateBatch
}

// contextLines is how many earlier lines go with a batch (ContextLines), 0 when context is off.
func (t ChatTranslator) contextLines() int {
	switch {
	case !t.Context:
		return 0
	case t.ContextLines > 0:
		return t.ContextLines
	}
	return contextLines
}

// retryBaseDelay doubles after each failed attempt unless the server sends Retry-After.
var retryBaseDelay = 2 * time.Second

// complete posts body to the chat endpoint, retrying rate limits and server errors, which gateways return when upstream capacity is briefly exhausted, and requests that timed out, lost their connection or came back cut off, while ctx lasts. Before each retry it reports progress with how long it waits, as the server's Retry-After asked or the backoff chose, and which retry comes; once the wait is over it reports progress as it was.
func (t ChatTranslator) complete(ctx context.Context, endpoint Endpoint, body []byte, progress core.Progress) (chatResponse, error) {
	return t.completeWith(ctx, endpoint, body, progress, nil, nil)
}

// completeWith is complete with header added to each request (ChatRequest.Header), counting in sent, when it is not nil, the requests that were written to the service whatever came of them, which the service may bill.
func (t ChatTranslator) completeWith(ctx context.Context, endpoint Endpoint, body []byte, progress core.Progress, header http.Header, sent *int) (chatResponse, error) {
	client := t.Client
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	delay := retryBaseDelay
	for attempt := 1; ; attempt++ {
		reply, retryAfter, wrote, err := completeOnce(ctx, client, endpoint, body, header)
		if wrote && sent != nil {
			*sent++
		}
		if err == nil || retryAfter < 0 || attempt >= t.attempts() {
			return reply, err
		}
		var status *statusError
		wait := delay
		if retryAfter > 0 {
			wait = min(retryAfter, maxRetryAfter)
		}
		delay *= 2
		logf(ctx, "AI request failed, retry %d of %d in %v: %v", attempt, t.attempts()-1, wait, err)
		waiting := progress
		waiting.RetryIn, waiting.Attempt, waiting.Attempts = wait, attempt, t.attempts()-1
		waiting.Unanswered = !errors.As(err, &status)
		reportProgress(ctx, waiting)
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return chatResponse{}, ctx.Err()
		case <-timer.C:
		}
		reportProgress(ctx, progress)
	}
}

// completeOnce makes one request, with header added to it, Content-Type and Authorization aside, and reports whether the request was written to the service (sent), which a request that could not connect, or whose ctx was done before it went, was not. A non-negative retryAfter marks a retryable failure, positive when the server said how long to wait: a rate limit that does not say the account is out of credit, a server error, or a request that got no usable answer, as one that timed out, lost its connection or came back cut off. Once ctx is done its error is returned as it is, not retried.
func completeOnce(ctx context.Context, client *http.Client, endpoint Endpoint, body []byte, header http.Header) (reply chatResponse, retryAfter time.Duration, sent bool, err error) {
	if err := ctx.Err(); err != nil {
		return reply, -1, false, err
	}
	var wrote atomic.Bool
	traced := httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{WroteRequest: func(info httptrace.WroteRequestInfo) {
		if info.Err == nil {
			wrote.Store(true)
		}
	}})
	req, err := http.NewRequestWithContext(traced, http.MethodPost, strings.TrimRight(endpoint.BaseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return reply, -1, false, err
	}
	defer func() { sent = wrote.Load() }()
	for name, values := range header {
		for _, v := range values {
			req.Header.Add(name, v)
		}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Del("Authorization")
	if endpoint.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+endpoint.APIKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return reply, -1, false, ctxErr
		}
		return reply, 0, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		err := &statusError{code: resp.StatusCode, status: resp.Status, detail: strings.TrimSpace(string(detail))}
		if (resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500) || outOfCredit(err.code, err.detail) {
			return reply, -1, false, err
		}
		retryAfter = 0
		if secs, convErr := strconv.Atoi(resp.Header.Get("Retry-After")); convErr == nil && secs > 0 {
			retryAfter = time.Duration(secs) * time.Second
		}
		return reply, retryAfter, false, err
	}
	if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return reply, -1, false, ctxErr
		}
		return reply, 0, false, fmt.Errorf("decode translation response: %w", err)
	}
	return reply, -1, false, nil
}

// stripCodeFence removes the ```json fence that chat models often wrap around JSON.
func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```")
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "```"))
}

// Earlier is a line said before the ones a translation is given (WithEarlier): its original text, and its translation as the film shows it now, "" while it has none.
type Earlier struct {
	Line, Translation string
}

type earlierKey struct{}

// WithEarlier returns ctx carrying lines, the lines said before the ones a translation is given, with their translations so far, which a translator that reads the lines before (ChatTranslator with context on) sends with them, as a film translated a part at a time does, so the lines it translates write names and forms of address as the film has.
func WithEarlier(ctx context.Context, lines []Earlier) context.Context {
	return context.WithValue(ctx, earlierKey{}, lines)
}

// earlierOf is the lines WithEarlier put in ctx, none when it put none.
func earlierOf(ctx context.Context) []Earlier {
	lines, _ := ctx.Value(earlierKey{}).([]Earlier)
	return lines
}

type titleKey struct{}

// WithTitle returns ctx carrying title, what the video a translation is given lines of is called (filmTitle), which ChatTranslator sends with them, as the proofreading has it, so names it holds are written as it writes them.
func WithTitle(ctx context.Context, title string) context.Context {
	return context.WithValue(ctx, titleKey{}, title)
}

// titleOf is the title WithTitle put in ctx, "" when it put none.
func titleOf(ctx context.Context) string {
	title, _ := ctx.Value(titleKey{}).(string)
	return title
}
