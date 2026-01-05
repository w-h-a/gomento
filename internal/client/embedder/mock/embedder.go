package mock

import (
	"context"

	"github.com/w-h-a/gomento/internal/client/embedder"
)

type mockEmbedder struct {
	options embedder.Options
	err     error
	input   string
}

func (e *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	e.input = text

	embedding := make([]float32, 1536)
	embedding[0] = 0.01

	if e.err != nil {
		return nil, e.err
	}

	return embedding, nil
}

func (e *mockEmbedder) Input() string {
	return e.input
}

func NewEmbedder(opts ...embedder.Option) *mockEmbedder {
	options := embedder.NewOptions(opts...)

	e := &mockEmbedder{
		options: options,
	}

	if err, ok := ErrorFrom(options.Context); ok {
		e.err = err
	}

	return e
}
