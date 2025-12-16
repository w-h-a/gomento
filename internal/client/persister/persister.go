package persister

import (
	"context"

	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
)

type V1Persister interface {
	CreateProject(ctx context.Context, proj *v1.Project) error
	CreateSpace(ctx context.Context, space *v1.Space) error
	CreateSession(ctx context.Context, sess *v1.Session) error
	GetSession(ctx context.Context, id uuid.UUID) (*v1.Session, error)
	AddMessage(ctx context.Context, msg *v1.Message) error
	GetMessages(ctx context.Context, sessionId uuid.UUID) ([]v1.Message, error)
	SaveSkill(ctx context.Context, skill *v1.Skill) error
}
