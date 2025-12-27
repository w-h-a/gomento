package interpreter

import (
	"context"

	v1 "github.com/w-h-a/gomento/api/domain/v1"
)

type V1Interpreter interface {
	Distill(ctx context.Context, history []v1.Message) (*v1.Skill, error)
	Extract(ctx context.Context, history []v1.Message, currentTasks []v1.Task) ([]TaskAction, error)
}
