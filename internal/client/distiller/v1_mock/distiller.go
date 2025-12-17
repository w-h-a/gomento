package v1mock

import (
	"context"

	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/distiller"
)

type v1MockDistiller struct {
	options distiller.Options
}

func (d *v1MockDistiller) Distill(ctx context.Context, history []v1.Message) (*v1.Skill, error) {
	return &v1.Skill{
		Id:        uuid.New(),
		Trigger:   "how to restart redis",
		SOP:       "1. Check logs.\n2. Delete pod.",
		Embedding: []float32{1536},
	}, nil
}

func NewV1Distiller(opts ...distiller.Option) *v1MockDistiller {
	options := distiller.NewOptions(opts...)

	d := &v1MockDistiller{
		options: options,
	}

	return d
}
