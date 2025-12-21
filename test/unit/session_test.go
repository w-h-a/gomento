package unit

import (
	"context"
	"mime/multipart"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	v1mockdispatcher "github.com/w-h-a/gomento/internal/client/dispatcher/v1_mock"
	v1mockpersister "github.com/w-h-a/gomento/internal/client/persister/v1_mock"
	"github.com/w-h-a/gomento/internal/client/uploader"
	v1mockuploader "github.com/w-h-a/gomento/internal/client/uploader/v1_mock"
	v1session "github.com/w-h-a/gomento/internal/service/v1_session"
)

func TestAddMessage_PersistsMessageAndAsset(t *testing.T) {
	// Arrange
	p := v1mockpersister.NewV1Persister()
	d := v1mockdispatcher.NewV1Dispatcher()
	u := v1mockuploader.NewV1Uploader(uploader.WithContainer("test-bucket"))
	s := v1session.NewV1Service(p, d, u, "worker-queue")

	sessionId := uuid.New()

	files := map[string]*multipart.FileHeader{
		"my_file": {Filename: "log.txt", Size: 100},
	}

	input := v1session.SendMessageInput{
		SessionId: sessionId,
		Role:      "user",
		Parts: []v1session.PartInput{
			{Type: "text", Text: "Hello World"},
			{Type: "file", FileField: "my_file"},
		},
		Files: files,
	}

	ctx := context.Background()

	// Act
	err := s.AddMessage(ctx, input)
	require.NoError(t, err)

	// Assert: Uploader
	uploads := u.Uploads()
	assert.Len(t, uploads, 1)
	assert.Contains(t, uploads, "uploads/log.txt")

	// Assert: Persister Messages
	msgs, err := p.GetMessages(ctx, sessionId)
	assert.NoError(t, err)
	assert.Len(t, msgs, 1)
	assert.Equal(t, "Hello World", msgs[0].Parts[0].Text)
	assert.NotNil(t, msgs[0].Parts[1].AssetId)
	assert.Equal(t, "user", msgs[0].Role)

	// Assert: Persister Assets
	assets := p.Assets()
	saved, exists := assets[*msgs[0].Parts[1].AssetId]
	assert.True(t, exists)
	assert.Equal(t, "test-bucket", saved.Container)
	assert.Equal(t, "uploads/log.txt", saved.Path)
	assert.Equal(t, int64(100), saved.SizeBytes)
}

func TestFinishSession_Publishes(t *testing.T) {
	// Arrange
	p := v1mockpersister.NewV1Persister()
	d := v1mockdispatcher.NewV1Dispatcher()
	u := v1mockuploader.NewV1Uploader(uploader.WithContainer("test-bucket"))
	s := v1session.NewV1Service(p, d, u, "worker-queue")
	sessionId := uuid.New()
	ctx := context.Background()

	// Act
	err := s.FinishSession(ctx, sessionId)
	require.NoError(t, err)

	// Assert: State
	assert.Len(t, d.Tasks(), 1)
	task := d.Tasks()[0]
	assert.Equal(t, v1.TaskTypeDistill, task.Type)
	assert.Equal(t, sessionId, task.Payload.SessionId)
	assert.Equal(t, v1.TaskStatusPending, task.Payload.TaskStatus)
}
