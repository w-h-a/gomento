package buffer

import (
	"context"

	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
)

type V1Buffer interface {
	Add(ctx context.Context, msg *v1.Message, assets map[int]*v1.Asset) error
	GetRecent(ctx context.Context, sessionId uuid.UUID) ([]v1.Message, error)
	PopBatch(ctx context.Context, limit int) ([]BufferedMessage, int, error)
	Count(ctx context.Context) int
}
