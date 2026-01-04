package persister

import (
	"context"
	"errors"

	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
)

var (
	ErrJobLocked    = errors.New("job is locked or being processed")
	ErrTaskNotFound = errors.New("task not found")
)

type V1Persister interface {
	CreateJob(ctx context.Context, job *v1.Job) error
	AcquireJobLock(ctx context.Context, jobId uuid.UUID) error
	UpdateJobStatus(ctx context.Context, id uuid.UUID, status string) error

	CreateProject(ctx context.Context, proj *v1.Project) error

	CreateSpace(ctx context.Context, space *v1.Space) error
	GetSpace(ctx context.Context, id uuid.UUID) (*v1.Space, error)
	SaveSkill(ctx context.Context, skill *v1.Skill) error

	CreateSession(ctx context.Context, sess *v1.Session) error
	GetSession(ctx context.Context, id uuid.UUID) (*v1.Session, error)
	UpdateSession(ctx context.Context, sess *v1.Session) error

	FetchCurrentTasks(ctx context.Context, sessionId uuid.UUID, status *string) ([]v1.Task, error)
	InsertTask(ctx context.Context, sessionId uuid.UUID, afterOrder int, data []byte, status string) (*v1.Task, error)
	UpdateTask(ctx context.Context, taskId uuid.UUID, status *string, order *int, data []byte) (*v1.Task, error)
	AppendMessagesToTask(ctx context.Context, taskId uuid.UUID, messageIds []uuid.UUID) error
	AppendMessagesToThought(ctx context.Context, sessionId uuid.UUID, messageIds []uuid.UUID) error

	CreateMessageWithAssets(ctx context.Context, msg *v1.Message, assets map[int]*v1.Asset) error
	ListMessages(ctx context.Context, sessionId uuid.UUID, opts ...ListMessagesOption) ([]v1.Message, error)
	GetAssets(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]*v1.Asset, error)

	CreateArtifact(ctx context.Context, a *v1.Artifact) error
	UpsertFileWithAsset(ctx context.Context, file *v1.File, asset *v1.Asset) error
	ListArtifacts(ctx context.Context, projectId uuid.UUID) ([]v1.Artifact, error)
	ListFiles(ctx context.Context, artifactId uuid.UUID, opts ...ListFilesOption) ([]v1.File, error)
	GetFile(ctx context.Context, artifactId uuid.UUID, path string, filename string) (*v1.File, error)
}
