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

	CreateSpace(ctx context.Context, space *v1.Space) error
	ListSpaces(ctx context.Context) ([]v1.Space, error)
	GetSpace(ctx context.Context, id uuid.UUID) (*v1.Space, error)

	SaveSkill(ctx context.Context, skill *v1.Skill) error
	FetchCurrentSkills(ctx context.Context, spaceId uuid.UUID) ([]v1.Skill, error)
	UpdateSkill(ctx context.Context, id uuid.UUID, trigger string, sop string, embedding []float32) error
	SearchSkills(ctx context.Context, spaceId uuid.UUID, vector []float32, opts ...SearchOption) ([]v1.Skill, error)

	CreateSession(ctx context.Context, sess *v1.Session) error
	ListSessions(ctx context.Context, spaceId *uuid.UUID) ([]v1.Session, error)
	GetSession(ctx context.Context, id uuid.UUID) (*v1.Session, error)
	UpdateSessionSpace(ctx context.Context, sess *v1.Session) error

	FetchCurrentTasks(ctx context.Context, sessionId uuid.UUID, status *string) ([]v1.Task, error)
	InsertTask(ctx context.Context, sessionId uuid.UUID, afterOrder int, data []byte, status string) (*v1.Task, error)
	UpdateTask(ctx context.Context, taskId uuid.UUID, status *string, order *int, data []byte) (*v1.Task, error)
	AppendMessagesToTask(ctx context.Context, taskId uuid.UUID, messageIds []uuid.UUID) error
	AppendMessagesToThought(ctx context.Context, sessionId uuid.UUID, messageIds []uuid.UUID) error

	CreateMessageWithAssets(ctx context.Context, msg *v1.Message, assets map[int]*v1.Asset) error
	ListMessages(ctx context.Context, sessionId uuid.UUID, opts ...ListMessagesOption) ([]v1.Message, error)
	GetMessage(ctx context.Context, id uuid.UUID) (*v1.Message, error)
	UpdateMessageEmbedding(ctx context.Context, id uuid.UUID, vector []float32) error
	SearchMessages(ctx context.Context, spaceId uuid.UUID, vector []float32, opts ...SearchOption) ([]v1.Message, error)

	UpsertFileWithAsset(ctx context.Context, file *v1.File, asset *v1.Asset) error
	ListFiles(ctx context.Context, spaceId *uuid.UUID, opts ...ListFilesOption) ([]v1.File, error)
	GetFile(ctx context.Context, id uuid.UUID) (*v1.File, error)
	UpdateFileSpace(ctx context.Context, file *v1.File) error
	UpdateFileEmbedding(ctx context.Context, id uuid.UUID, vector []float32) error
	SearchFiles(ctx context.Context, spaceId uuid.UUID, vector []float32, opts ...SearchOption) ([]v1.File, error)

	SaveFileChunks(ctx context.Context, fileId uuid.UUID, chunks []v1.FileChunk) error
	SearchMatchingChunks(ctx context.Context, spaceId uuid.UUID, vector []float32, opts ...SearchOption) ([]v1.MatchingChunk, error)

	GetAssets(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]*v1.Asset, error)
}
