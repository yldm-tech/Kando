package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"lingocut/packages/core"
)

// translateBatch bounds how many lines go into one request, keeping replies within common output token limits.
const translateBatch = 40

// ChatTranslator translates through an OpenAI-compatible /chat/completions endpoint, such as OpenAI or a local Ollama server.
type ChatTranslator struct {
	BaseURL string
	APIKey  string
	Model   string
	// Client defaults to an http.Client with a two-minute timeout.
	Client *http.Client
}

func (t ChatTranslator) Translate(ctx context.Context, segments []core.Segment, language string) ([]core.Segment, error) {
	if strings.TrimSpace(language) == "" {
		return nil, fmt.Errorf("target language is empty")
	}
	out := make([]core.Segment, 0, len(segments))
	for start := 0; start < len(segments); start += translateBatch {
		batch := segments[start:min(start+translateBatch, len(segments))]
		texts := make([]string, len(batch))
		for i, s := range batch {
			texts[i] = s.Text
		}
		lines, err := t.translateTexts(ctx, texts, language)
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

func (t ChatTranslator) translateTexts(ctx context.Context, texts []string, language string) ([]string, error) {
	input, err := json.Marshal(texts)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(chatRequest{
		Model: t.Model,
		Messages: []chatMessage{
			{Role: "system", Content: fmt.Sprintf("You translate video subtitles into %s. The user sends a JSON array of subtitle lines in order. Reply with only a JSON array of strings with exactly the same number of elements, each the translation of the line at the same index. Do not merge, split, or skip lines.", language)},
			{Role: "user", Content: string(input)},
		},
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(t.BaseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if t.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+t.APIKey)
	}
	client := t.Client
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("translation API returned %s: %s", resp.Status, strings.TrimSpace(string(detail)))
	}

	var reply chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil {
		return nil, fmt.Errorf("decode translation response: %w", err)
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
