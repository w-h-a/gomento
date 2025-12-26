package persister

import (
	"context"
	"errors"

	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
)

var (
	ErrSessionLocked = errors.New("session is locked by another task")
)

type V1Persister interface {
	CreateProject(ctx context.Context, proj *v1.Project) error

	CreateSpace(ctx context.Context, space *v1.Space) error
	GetSpace(ctx context.Context, id uuid.UUID) (*v1.Space, error)

	CreateSession(ctx context.Context, sess *v1.Session) error
	GetSession(ctx context.Context, id uuid.UUID) (*v1.Session, error)
	UpdateSession(ctx context.Context, sess *v1.Session) error

	AcquireSessionLock(ctx context.Context, sessionId uuid.UUID, taskId uuid.UUID) error

	CreateTask(ctx context.Context, t *v1.Task) error
	UpdateTaskStatus(ctx context.Context, id uuid.UUID, status string) error
	GetTask(ctx context.Context, id uuid.UUID) (*v1.Task, error)

	CreateMessageWithAssets(ctx context.Context, msg *v1.Message, assets map[int]*v1.Asset) error
	GetMessages(ctx context.Context, sessionId uuid.UUID, opts ...GetMessagesOption) ([]v1.Message, error)
	GetAssets(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]*v1.Asset, error)

	SaveSkill(ctx context.Context, skill *v1.Skill) error
}
