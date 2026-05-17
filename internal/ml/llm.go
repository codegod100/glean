package ml

import (
	"context"
	"fmt"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

type TextModel interface {
	DetectLanguages(ctx context.Context, texts []string) ([]string, error)
	GenerateDigest(ctx context.Context, articleTitles []string, trendingTopics []string) (title, body string, err error)
}

type llm struct {
	client openai.Client
	model  string
}

type LLMConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

func NewLLM(cfg LLMConfig) TextModel {
	opts := []option.RequestOption{}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	if cfg.APIKey != "" {
		opts = append(opts, option.WithAPIKey(cfg.APIKey))
	}
	return &llm{
		client: openai.NewClient(opts...),
		model:  cfg.Model,
	}
}

func (c *llm) DetectLanguages(ctx context.Context, texts []string) ([]string, error) {
	var b strings.Builder
	b.WriteString("For each text below, respond with ONLY the ISO 639-1 language code (e.g. en, fr, de, es, pt, it, ru, ja, zh, ko, ar). One code per line, same order as input. If uncertain, respond with 'unknown'.\n\n")
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
	for i, line := range lines {
		if i >= len(result) {
			break
		}
		code := strings.TrimSpace(line)
		code = strings.TrimPrefix(code, fmt.Sprintf("%d.", i+1))
		code = strings.TrimSpace(code)
		if IsKnownLanguage(code) || code == "unknown" {
			result[i] = code
		}
	}
	return result, nil
}

func (c *llm) GenerateDigest(ctx context.Context, articleTitles []string, trendingTopics []string) (string, string, error) {
	var b strings.Builder
	b.WriteString(`You are the editor of a sharply written daily briefing for a tech-savvy reader. Write a digest that groups the day's articles into thematic sections.

Rules:
- First line must be a newspaper-style headline (max 8 words) that captures the dominant theme or tension across the articles. Be specific — name the biggest topic, company, or conflict. Never use meta-phrases like "Daily digest", "Today's briefing", or "Your daily roundup". Just the raw headline, no quotes, no prefix.
- Then a blank line, then the body.
- Organize into 3-5 thematic sections. Each section has:
  - A bold inline heading wrapped in <strong> tags (2-4 words, e.g. <strong>Open source</strong>).
  - Followed by a colon and 2-3 sentences weaving the relevant articles into a narrative.
- When you mention an article from the numbered list, append its number in brackets right after, like: "Nvidia raised prices on the RTX 5090 [1]." Do this for every article you reference.
- Be specific — name companies, projects, and technologies. Vague summaries are useless.
- Use a confident, conversational tone. Short sentences. No filler, no hedging.
- End with a one-line note on the most interesting or under-the-radar article.
- Use <p> tags to separate sections. Each <p> contains exactly one theme.
- Don't write a theme if there aren't at least 2 articles for it. Pick the most interesting and diverse themes.

`)

	fmt.Fprintf(&b, "Here are the %d most recent unread articles (each has a title, optional feed name in brackets, URL, and content excerpt):\n\n", len(articleTitles))
	for i, t := range articleTitles {
		if i >= 50 {
			break
		}
		fmt.Fprintf(&b, "%d. %s\n", i+1, t)
	}
	if len(trendingTopics) > 0 {
		b.WriteString("\nTrending in the reader's network:\n")
		for _, t := range trendingTopics {
			fmt.Fprintf(&b, "- %s\n", t)
		}
	}

	resp, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: c.model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(b.String()),
		},
		Temperature: openai.Float(0.7),
	})
	if err != nil {
		return "", "", err
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	title, body, _ := strings.Cut(content, "\n")
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	return title, body, nil
}
