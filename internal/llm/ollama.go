package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type ollamaProvider struct {
	host   string
	model  string
	client *http.Client
}

func newOllama(host, model string) *ollamaProvider {
	return &ollamaProvider{
		host:  host,
		model: model,
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

type ollamaRequest struct {
	Model   string         `json:"model"`
	System  string         `json:"system,omitempty"`
	Prompt  string         `json:"prompt"`
	Stream  bool           `json:"stream"`
	Options *ollamaOptions `json:"options,omitempty"`
}

type ollamaOptions struct {
	Temperature *float32 `json:"temperature,omitempty"`
	NumPredict  int      `json:"num_predict,omitempty"`
}

type ollamaResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

func (p *ollamaProvider) Complete(ctx context.Context, system, prompt string, opts *CompleteOptions) (string, error) {
	req := ollamaRequest{
		Model:  p.model,
		System: system,
		Prompt: prompt,
		Stream: false,
	}
	if opts != nil {
		var o ollamaOptions
		if opts.Temperature != nil {
			o.Temperature = opts.Temperature
		}
		if opts.MaxTokens > 0 {
			o.NumPredict = opts.MaxTokens
		}
		if o.Temperature != nil || o.NumPredict > 0 {
			req.Options = &o
		}
	}
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshaling ollama request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.host+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating ollama request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding ollama response: %w", err)
	}
	return result.Response, nil
}
