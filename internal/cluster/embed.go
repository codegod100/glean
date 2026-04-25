package cluster

import (
	"bytes"
	"context"
	"encoding/binary"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// Embedder generates vector embeddings for text inputs. Implementations must be
// safe for concurrent use.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dimension() int
}

type OpenAIEmbedder struct {
	client    openai.Client
	model     string
	dimension int
}

type OpenAIEmbedderConfig struct {
	BaseURL   string
	APIKey    string
	Model     string
	Dimension int
}

func NewOpenAIEmbedder(cfg OpenAIEmbedderConfig) *OpenAIEmbedder {
	opts := []option.RequestOption{}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	if cfg.APIKey != "" {
		opts = append(opts, option.WithAPIKey(cfg.APIKey))
	}
	return &OpenAIEmbedder{
		client:    openai.NewClient(opts...),
		model:     cfg.Model,
		dimension: cfg.Dimension,
	}
}

func (e *OpenAIEmbedder) Dimension() int {
	return e.dimension
}

func (e *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	resp, err := e.client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Model: e.model,
		Input: openai.EmbeddingNewParamsInputUnion{
			OfArrayOfStrings: texts,
		},
	})
	if err != nil {
		return nil, err
	}

	embeddings := make([][]float32, len(resp.Data))
	for i, d := range resp.Data {
		embeddings[i] = make([]float32, len(d.Embedding))
		for j, v := range d.Embedding {
			embeddings[i][j] = float32(v)
		}
	}
	return embeddings, nil
}

func deserializeFloat32(data []byte) []float32 {
	if len(data)%4 != 0 {
		return nil
	}
	result := make([]float32, len(data)/4)
	r := bytes.NewReader(data)
	_ = binary.Read(r, binary.LittleEndian, &result)
	return result
}
