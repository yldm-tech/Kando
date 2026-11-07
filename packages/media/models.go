package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
)

// Models lists the model ids the endpoint offers, from the OpenAI-compatible GET /models, sorted and without repeats, so the AI service's model is chosen from what the service has rather than typed. A key the API rejects is renewed once through Credential, as Chat does.
func (t ChatTranslator) Models(ctx context.Context) ([]string, error) {
	endpoint := Endpoint{BaseURL: strings.TrimSpace(t.BaseURL), APIKey: t.APIKey}
	if t.Credential != nil {
		var err error
		if endpoint, err = t.Credential(ctx, false); err != nil {
			return nil, err
		}
	}
	client := t.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	models, err := listModels(ctx, client, endpoint)
	var status *statusError
	if t.Credential != nil && errors.As(err, &status) && status.code == http.StatusUnauthorized {
		if endpoint, err = t.Credential(ctx, true); err != nil {
			return nil, err
		}
		models, err = listModels(ctx, client, endpoint)
	}
	return models, err
}

func listModels(ctx context.Context, client *http.Client, endpoint Endpoint) ([]string, error) {
	if strings.TrimSpace(endpoint.BaseURL) == "" {
		return nil, errors.New("base URL is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(strings.TrimSpace(endpoint.BaseURL), "/")+"/models", nil)
	if err != nil {
		return nil, err
	}
	if endpoint.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+endpoint.APIKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, &statusError{code: resp.StatusCode, status: resp.Status, detail: strings.TrimSpace(string(detail))}
	}
	var list struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&list); err != nil {
		return nil, fmt.Errorf("decode model list: %w", err)
	}
	var ids []string
	for _, m := range list.Data {
		if id := strings.TrimSpace(m.ID); id != "" {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids), nil
}
