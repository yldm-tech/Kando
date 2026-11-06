package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"kando/packages/core"
)

// translateBatch bounds how many lines go into one request, keeping replies within common output token limits.
const translateBatch = 40

// DefaultSubtitlePrompt is the style instruction used when ChatTranslator.Prompt is empty. {{language}} becomes the target language. The JSON reply rule is appended separately and is not part of this text.
const DefaultSubtitlePrompt = "You are a professional subtitle translator. Translate every subtitle line into {{language}}.\n- Keep each translation natural, concise and suitable for on-screen subtitles.\n- Lines may be sentence fragments that continue in the next line; translate them so they read naturally in order."

const subtitleReplyRule = "The user sends a JSON array of subtitle lines in order. Reply with only a JSON array of strings with exactly the same number of elements, each the translation of the line at the same index. Do not merge, split, or skip lines."

// contextLines is how many earlier lines of the same video go with a batch when context is on. Subtitle text is short, but an entire film can still crowd a small model.
const contextLines = 80

// ChatTranslator translates through an OpenAI-compatible /chat/completions endpoint, such as EveryAPI, OpenAI or a local Ollama server.
type ChatTranslator struct {
	BaseURL string
	APIKey  string
	Model   string
	// Prompt is the style instruction. {{language}} is replaced with the target language. Empty uses DefaultSubtitlePrompt.
	Prompt string
	// Context includes earlier lines of the same video with each batch.
	Context bool
	// Credential, when set, replaces BaseURL and APIKey for each translation. renew asks for a new key after the API rejected the previous one.
	Credential func(ctx context.Context, renew bool) (Endpoint, error)
	// Client defaults to an http.Client with a two-minute timeout.
	Client *http.Client
}

// Endpoint is where translation requests go and the bearer key they carry.
type Endpoint struct {
	BaseURL string
	APIKey  string
}

// Ping checks that the chat endpoint accepts one short request with the configured model.
func (t ChatTranslator) Ping(ctx context.Context) error {
	if strings.TrimSpace(t.Model) == "" {
		return errors.New("model is empty")
	}
	endpoint := Endpoint{BaseURL: strings.TrimSpace(t.BaseURL), APIKey: t.APIKey}
	if t.Credential != nil {
		var err error
		if endpoint, err = t.Credential(ctx, false); err != nil {
			return err
		}
	}
	if strings.TrimSpace(endpoint.BaseURL) == "" {
		return errors.New("base URL is empty")
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
	_, _, err = completeOnce(ctx, client, endpoint, body)
	return err
}

func (t ChatTranslator) Translate(ctx context.Context, segments []core.Segment, language string) ([]core.Segment, error) {
	if strings.TrimSpace(language) == "" {
		return nil, fmt.Errorf("target language is empty")
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
	var earlier []string
	for start := 0; start < len(segments); start += translateBatch {
		batch := segments[start:min(start+translateBatch, len(segments))]
		texts := make([]string, len(batch))
		for i, s := range batch {
			texts[i] = s.Text
		}
		var contextSoFar []string
		if t.Context {
			contextSoFar = earlier
		}
		lines, err := t.translateTexts(ctx, endpoint, texts, language, contextSoFar)
		earlier = append(earlier, texts...)
		var status *statusError
		if t.Credential != nil && !renewed && errors.As(err, &status) && status.code == http.StatusUnauthorized {
			renewed = true
			if endpoint, err = t.Credential(ctx, true); err != nil {
				return nil, err
			}
			lines, err = t.translateTexts(ctx, endpoint, texts, language, contextSoFar)
		}
		if err != nil {
			return nil, err
		}
		for i, s := range batch {
			s.Text = lines[i]
			out = append(out, s)
		}
	}
	return out, nil
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

// statusError is a non-200 reply from the translation API.
type statusError struct {
	code   int
	status string
	detail string
}

func (e *statusError) Error() string {
	return fmt.Sprintf("translation API returned %s: %s", e.status, e.detail)
}

func subtitlePrompt(custom, language string, earlier []string) string {
	prompt := strings.TrimSpace(custom)
	if prompt == "" {
		prompt = DefaultSubtitlePrompt
	}
	prompt = strings.ReplaceAll(prompt, "{{language}}", language)
	if len(earlier) > contextLines {
		earlier = earlier[len(earlier)-contextLines:]
	}
	if len(earlier) > 0 {
		encoded, _ := json.Marshal(earlier)
		prompt += "\n\nEarlier lines from the same video, for context only. Do not translate them again:\n" + string(encoded)
	}
	return prompt + "\n\n" + subtitleReplyRule
}

func (t ChatTranslator) translateTexts(ctx context.Context, endpoint Endpoint, texts []string, language string, earlier []string) ([]string, error) {
	input, err := json.Marshal(texts)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(chatRequest{
		Model: t.Model,
		Messages: []chatMessage{
			{Role: "system", Content: subtitlePrompt(t.Prompt, language, earlier)},
			{Role: "user", Content: string(input)},
		},
	})
	if err != nil {
		return nil, err
	}

	reply, err := t.complete(ctx, endpoint, body)
	if err != nil {
		return nil, err
	}
	if len(reply.Choices) == 0 {
		return nil, fmt.Errorf("translation API returned no choices")
	}
	var lines []string
	if err := json.Unmarshal([]byte(stripCodeFence(reply.Choices[0].Message.Content)), &lines); err != nil {
		return nil, fmt.Errorf("translation is not a JSON array of strings: %w", err)
	}
	if len(lines) != len(texts) {
		return nil, fmt.Errorf("translation returned %d lines for %d", len(lines), len(texts))
	}
	return lines, nil
}

const (
	maxAttempts   = 3
	maxRetryAfter = 30 * time.Second
)

// retryBaseDelay doubles after each failed attempt unless the server sends Retry-After.
var retryBaseDelay = 2 * time.Second

// complete posts body to the chat endpoint, retrying rate limits and server errors, which gateways return when upstream capacity is briefly exhausted.
func (t ChatTranslator) complete(ctx context.Context, endpoint Endpoint, body []byte) (chatResponse, error) {
	client := t.Client
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	delay := retryBaseDelay
	for attempt := 1; ; attempt++ {
		reply, retryAfter, err := completeOnce(ctx, client, endpoint, body)
		if err == nil || retryAfter < 0 || attempt == maxAttempts {
			return reply, err
		}
		wait := delay
		if retryAfter > 0 {
			wait = min(retryAfter, maxRetryAfter)
		}
		delay *= 2
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return chatResponse{}, ctx.Err()
		case <-timer.C:
		}
	}
}

// completeOnce makes one request. A non-negative retryAfter marks a retryable failure, positive when the server said how long to wait.
func completeOnce(ctx context.Context, client *http.Client, endpoint Endpoint, body []byte) (reply chatResponse, retryAfter time.Duration, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(endpoint.BaseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return reply, -1, err
	}
	req.Header.Set("Content-Type", "application/json")
	if endpoint.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+endpoint.APIKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return reply, -1, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		err := &statusError{code: resp.StatusCode, status: resp.Status, detail: strings.TrimSpace(string(detail))}
		if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
			return reply, -1, err
		}
		retryAfter = 0
		if secs, convErr := strconv.Atoi(resp.Header.Get("Retry-After")); convErr == nil && secs > 0 {
			retryAfter = time.Duration(secs) * time.Second
		}
		return reply, retryAfter, err
	}
	if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil {
		return reply, -1, fmt.Errorf("decode translation response: %w", err)
	}
	return reply, -1, nil
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
