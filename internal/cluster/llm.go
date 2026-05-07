package cluster

import (
	"context"
	"fmt"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

type LLMClient struct {
	client openai.Client
	model  string
}

type LLMClientConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

func NewLLMClient(cfg LLMClientConfig) *LLMClient {
	opts := []option.RequestOption{}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	if cfg.APIKey != "" {
		opts = append(opts, option.WithAPIKey(cfg.APIKey))
	}
	return &LLMClient{
		client: openai.NewClient(opts...),
		model:  cfg.Model,
	}
}

func (c *LLMClient) DetectLanguages(ctx context.Context, texts []string) ([]string, error) {
	var b strings.Builder
	b.WriteString("For each text below, respond with ONLY the ISO 639-1 language code (e.g. en, fr, de, es, pt, it, ru, ja, zh, ko, ar). One code per line, same order as input. If uncertain, respond with 'en'.\n\n")
	for i, t := range texts {
		truncated := t
		if len(truncated) > 500 {
			truncated = truncated[:500]
		}
		fmt.Fprintf(&b, "%d. %s\n", i+1, truncated)
	}

	resp, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: c.model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(b.String()),
		},
		Temperature: openai.Float(0.0),
	})
	if err != nil {
		return nil, err
	}

	content := resp.Choices[0].Message.Content
	lines := strings.Split(strings.TrimSpace(content), "\n")
	result := make([]string, len(texts))
	for i := range result {
		result[i] = "en"
	}
	for i, line := range lines {
		if i >= len(result) {
			break
		}
		code := strings.TrimSpace(line)
		code = strings.TrimPrefix(code, fmt.Sprintf("%d.", i+1))
		code = strings.TrimSpace(code)
		if code != "" {
			result[i] = code
		}
	}
	return result, nil
}
