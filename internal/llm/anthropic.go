package llm

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/vertex"
)

type anthropicProvider struct {
	client anthropic.Client
	model  string
}

func newAnthropic(apiKey, model string, useVertexAI bool, vertexRegion, vertexProjectID string) (*anthropicProvider, error) {
	clientOpts := []option.RequestOption{
		option.WithMaxRetries(5),
	}
	if useVertexAI {
		vopt, err := newVertexAuthOption(context.Background(), vertexRegion, vertexProjectID)
		if err != nil {
			return nil, err
		}
		clientOpts = append(clientOpts, vopt)
	} else {
		clientOpts = append(clientOpts, option.WithAPIKey(apiKey))
	}
	return &anthropicProvider{
		client: anthropic.NewClient(clientOpts...),
		model:  model,
	}, nil
}

func newVertexAuthOption(ctx context.Context, region, projectID string) (reqOpt option.RequestOption, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("anthropic vertex auth setup failed: %v", r)
		}
	}()
	return vertex.WithGoogleAuth(ctx, region, projectID), nil
}

func (p *anthropicProvider) Complete(ctx context.Context, system, prompt string, opts *CompleteOptions) (string, error) {
	maxTokens := int64(16384)
	if opts != nil && opts.MaxTokens > 0 {
		maxTokens = int64(opts.MaxTokens)
	}
	msg, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(p.model),
		MaxTokens: maxTokens,
		System: []anthropic.TextBlockParam{
			{Text: system},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("anthropic completion: %w", err)
	}
	// Return the first text block only; multi-block responses are not expected
	// from single-turn completions.
	for _, block := range msg.Content {
		if block.Type == "text" {
			return block.Text, nil
		}
	}
	return "", fmt.Errorf("anthropic returned no text content")
}
