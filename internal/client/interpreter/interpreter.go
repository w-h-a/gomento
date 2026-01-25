package interpreter

import (
	"context"

	v1 "github.com/w-h-a/gomento/api/domain/v1"
)

type V1Interpreter interface {
	Distill(ctx context.Context, history []v1.Message, chunks []v1.MatchingChunk, currentSkills []v1.Skill) ([]SkillAction, error)
	Extract(ctx context.Context, history []v1.Message, chunks []v1.MatchingChunk, currentTasks []v1.Task) ([]TaskAction, error)
}
