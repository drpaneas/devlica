package llm

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

type anthropicProvider struct {
	client anthropic.Client
	model  string
}

func newAnthropic(apiKey, model string) *anthropicProvider {
	return &anthropicProvider{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
		model:  model,
	}
}

func (p *anthropicProvider) Complete(ctx context.Context, system, prompt string, opts *CompleteOptions) (string, error) {
	maxTokens := int64(4096)
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
