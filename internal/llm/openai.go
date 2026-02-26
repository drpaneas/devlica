package llm

import (
	"context"
	"fmt"

	openai "github.com/sashabaranov/go-openai"
)

type openaiProvider struct {
	client *openai.Client
	model  string
}

func newOpenAI(apiKey, model string) *openaiProvider {
	return &openaiProvider{
		client: openai.NewClient(apiKey),
		model:  model,
	}
}

func (p *openaiProvider) Complete(ctx context.Context, system, prompt string, opts *CompleteOptions) (string, error) {
	temp := float32(0.3)
	if opts != nil && opts.Temperature != nil {
		temp = *opts.Temperature
	}
	resp, err := p.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: p.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: system},
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
		Temperature: temp,
	})
	if err != nil {
		return "", fmt.Errorf("openai completion: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("openai returned no choices")
	}
	return resp.Choices[0].Message.Content, nil
}
