package llm

import (
	"context"
	"fmt"

	"github.com/sashabaranov/go-openai"
)

// Embedder 文本嵌入（用于向量检索）
type Embedder interface {
	Embed(ctx context.Context, model string, texts []string) ([][]float32, error)
}

// Embed 使用 OpenAI Embeddings API
func (o *OpenAI) Embed(ctx context.Context, model string, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	if model == "" {
		model = "text-embedding-3-small"
	}
	req := openai.EmbeddingRequest{
		Input: texts,
		Model: openai.EmbeddingModel(model),
	}
	resp, err := o.client.CreateEmbeddings(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("embeddings: %w", err)
	}
	out := make([][]float32, len(resp.Data))
	for i, d := range resp.Data {
		vec := make([]float32, len(d.Embedding))
		copy(vec, d.Embedding)
		out[i] = vec
	}
	return out, nil
}
