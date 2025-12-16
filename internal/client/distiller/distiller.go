package distiller

import (
	"context"

	v1 "github.com/w-h-a/gomento/api/domain/v1"
)

type V1Distiller interface {
	Distill(ctx context.Context, history []v1.Message) (*v1.Skill, error)
}
