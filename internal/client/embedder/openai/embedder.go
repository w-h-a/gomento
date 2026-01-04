package openai

import (
	"context"
	"log/slog"

	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/w-h-a/gomento/internal/client/embedder"
)

type openaiEmbedder struct {
	options embedder.Options
	embeddings.Embedder
}

func (e *openaiEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return e.EmbedQuery(ctx, text)
}

func NewEmbedder(opts ...embedder.Option) embedder.Embedder {
	options := embedder.NewOptions(opts...)

	e := &openaiEmbedder{
		options: options,
	}

	llmOpts := []openai.Option{
		openai.WithToken(options.ApiKey),
		openai.WithModel(options.Model),
	}

	llm, err := openai.New(llmOpts...)
	if err != nil {
		detail := "failed to initialize model for openai embedder"
		slog.ErrorContext(context.Background(), detail, "error", err)
		panic(detail)
	}

	emb, err := embeddings.NewEmbedder(llm)
	if err != nil {
		detail := "failed to initialize embedder for openai embedder"
		slog.ErrorContext(context.Background(), detail, "error", err)
		panic(detail)
	}

	e.Embedder = emb

	return e
}
