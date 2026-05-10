package ai

import (
	"context"
	"unsafe"

	vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

type Embedder interface {
	Embed(ctx context.Context, texts []string, instruction string) ([][]float32, error)
	Dimension() int
}

type embedder struct {
	client    openai.Client
	model     string
	dimension int
}

type EmbedConfig struct {
	BaseURL   string
	APIKey    string
	Model     string
	Dimension int
}

func NewEmbedder(cfg EmbedConfig) Embedder {
	opts := []option.RequestOption{}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	if cfg.APIKey != "" {
		opts = append(opts, option.WithAPIKey(cfg.APIKey))
	}
	return &embedder{
		client:    openai.NewClient(opts...),
		model:     cfg.Model,
		dimension: cfg.Dimension,
	}
}

func (e *embedder) Dimension() int {
	return e.dimension
}

func (e *embedder) Embed(ctx context.Context, texts []string, instruction string) ([][]float32, error) {
	inputs := texts
	if instruction != "" {
		inputs = make([]string, len(texts))
		for i, t := range texts {
			inputs[i] = instruction + "\n" + t
		}
	}
	resp, err := e.client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Model: e.model,
		Input: openai.EmbeddingNewParamsInputUnion{
			OfArrayOfStrings: inputs,
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

func AvgEmbeddings(blobs [][]byte, dim int) ([]byte, error) {
	sum := make([]float32, dim)
	count := 0
	for _, blob := range blobs {
		v := BytesToFloat32s(blob, dim)
		if v == nil {
			continue
		}
		for j := range sum {
			sum[j] += v[j]
		}
		count++
	}
	if count == 0 {
		return nil, nil
	}
	for j := range sum {
		sum[j] /= float32(count)
	}
	return vec.SerializeFloat32(sum)
}

func BytesToFloat32s(data []byte, expectedDim int) []float32 {
	if len(data) != expectedDim*4 {
		return nil
	}
	return unsafe.Slice((*float32)(unsafe.Pointer(&data[0])), expectedDim)
}
