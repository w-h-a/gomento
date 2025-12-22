package unit

import (
	"context"
	"mime/multipart"
	"os"
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
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	d := v1mockdispatcher.NewV1Dispatcher()
	u := v1mockuploader.NewV1Uploader(uploader.WithContainer("test-bucket"))
	s := v1session.NewV1Service(p, d, u, "worker-queue")

	sessionId := uuid.New()
	ctx := context.Background()

	// Act
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

	msg, err := s.AddMessage(ctx, input)
	require.NoError(t, err)

	// Assert: Behavior
	assert.NotNil(t, msg)
	assert.Equal(t, "user", msg.Role)
	assert.Len(t, msg.Parts, 2)
	assert.Nil(t, msg.Parts[0].AssetId)
	assert.NotNil(t, msg.Parts[1].AssetId)

	// Assert: Uploader
	uploads := u.Uploads()
	assert.Len(t, uploads, 1)
	assert.Contains(t, uploads, "uploads/log.txt")

	// Assert: Persister Messages
	msgs, err := p.GetMessages(ctx, sessionId)
	assert.NoError(t, err)
	assert.Len(t, msgs, 1)
	assert.Equal(t, "Hello World", msgs[0].Parts[0].Text)
	assert.Equal(t, *msg.Parts[1].AssetId, *msgs[0].Parts[1].AssetId)
	assert.Equal(t, "user", msgs[0].Role)

	// Assert: Persister Assets
	assets := p.Assets()
	saved, exists := assets[*msgs[0].Parts[1].AssetId]
	assert.True(t, exists)
	assert.Equal(t, "test-bucket", saved.Container)
	assert.Equal(t, "uploads/log.txt", saved.Path)
	assert.Equal(t, int64(100), saved.SizeBytes)
}

func TestAddMessage_PersistsLinkedMessages(t *testing.T) {
	// Arrange
	p := v1mockpersister.NewV1Persister()
	d := v1mockdispatcher.NewV1Dispatcher()
	u := v1mockuploader.NewV1Uploader(uploader.WithContainer("test-bucket"))
	s := v1session.NewV1Service(p, d, u, "worker-queue")

	sessionId := uuid.New()
	ctx := context.Background()

	// Act
	input1 := v1session.SendMessageInput{
		SessionId: sessionId,
		Role:      "user",
		Parts: []v1session.PartInput{
			{Type: "text", Text: "First Message"},
		},
	}
	msg1, err := s.AddMessage(ctx, input1)
	require.NoError(t, err)

	// Assert: Behavior
	assert.NotNil(t, msg1)
	assert.Nil(t, msg1.ParentId)
	assert.Equal(t, "user", msg1.Role)

	// Act
	input2 := v1session.SendMessageInput{
		SessionId: sessionId,
		Role:      "assistant",
		Parts: []v1session.PartInput{
			{Type: "text", Text: "Second Message"},
		},
	}
	msg2, err := s.AddMessage(ctx, input2)
	require.NoError(t, err)

	// Assert: Behavior
	assert.NotNil(t, msg2)
	assert.Equal(t, msg1.Id, *msg2.ParentId)
	assert.Equal(t, "assistant", msg2.Role)

	// Assert: State
	stored, err := p.GetMessages(ctx, sessionId)
	assert.NoError(t, err)
	assert.Len(t, stored, 2)
	assert.Equal(t, msg1.Id, stored[0].Id)
	assert.Equal(t, msg2.Id, stored[1].Id)
	assert.Equal(t, msg1.Id, *stored[1].ParentId)
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
