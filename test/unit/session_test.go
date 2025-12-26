package unit

import (
	"context"
	"encoding/json"
	"mime/multipart"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	v1mockdispatcher "github.com/w-h-a/gomento/internal/client/dispatcher/v1_mock"
	"github.com/w-h-a/gomento/internal/client/filer"
	v1mockfiler "github.com/w-h-a/gomento/internal/client/filer/v1_mock"
	v1mockpersister "github.com/w-h-a/gomento/internal/client/persister/v1_mock"
	v1session "github.com/w-h-a/gomento/internal/service/v1_session"
	"github.com/w-h-a/gomento/internal/util"
)

func TestAddMessage_PersistsMessageAndAsset(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	d := v1mockdispatcher.NewV1Dispatcher()
	f := v1mockfiler.NewV1Filer(filer.WithContainer("test-bucket"))
	s := v1session.NewV1Service(p, d, f, "worker-queue")

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

	// Assert: Filer
	uploads := f.Uploads()
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
	assets, err := p.GetAssets(ctx, []uuid.UUID{*msg.Parts[1].AssetId})
	assert.NoError(t, err)
	saved, exists := assets[*msgs[0].Parts[1].AssetId]
	assert.True(t, exists)
	assert.Equal(t, "test-bucket", saved.Container)
	assert.Equal(t, "uploads/log.txt", saved.Path)
	assert.Equal(t, int64(100), saved.SizeBytes)
}

func TestAddMessage_PersistsLinkedMessages(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	d := v1mockdispatcher.NewV1Dispatcher()
	f := v1mockfiler.NewV1Filer(filer.WithContainer("test-bucket"))
	s := v1session.NewV1Service(p, d, f, "worker-queue")

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

func TestGetMessages_PaginationLogic(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	d := v1mockdispatcher.NewV1Dispatcher()
	u := v1mockfiler.NewV1Filer()
	s := v1session.NewV1Service(p, d, u, "worker-queue")

	sessionId := uuid.New()
	ctx := context.Background()

	for i := range 3 {
		msg := &v1.Message{
			Id:        uuid.New(),
			SessionId: sessionId,
			Role:      "user",
			Parts:     []v1.Part{{Type: "text", Text: "Hi"}},
			CreatedAt: time.Now().Add(time.Duration(i) * time.Minute),
		}
		p.CreateMessageWithAssets(ctx, msg, nil)
	}

	// Act
	out, err := s.GetMessages(ctx, v1session.GetMessagesInput{
		SessionId: sessionId,
		Limit:     2,
	})
	require.NoError(t, err)

	// Assert
	assert.Len(t, out.Items, 2)
	assert.True(t, out.HasMore)
	assert.NotEmpty(t, out.NextCursor)
	assert.NotEqual(t, "", out.NextCursor)
}

func TestGetMessages_EnrichesAssetsWithPresignedUrls(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	d := v1mockdispatcher.NewV1Dispatcher()
	u := v1mockfiler.NewV1Filer(filer.WithContainer("my-bucket"))
	s := v1session.NewV1Service(p, d, u, "worker-queue")

	sessionId := uuid.New()
	ctx := context.Background()

	assetId := uuid.New()
	asset := &v1.Asset{
		Id:        assetId,
		Container: "my-bucket",
		Path:      "logs/crash.txt",
	}

	msg := &v1.Message{
		Id:        uuid.New(),
		SessionId: sessionId,
		Role:      "user",
		Parts:     []v1.Part{{Type: "file", AssetId: &assetId}},
		CreatedAt: time.Now(),
	}

	p.CreateMessageWithAssets(ctx, msg, map[int]*v1.Asset{0: asset})

	// Act
	out, err := s.GetMessages(ctx, v1session.GetMessagesInput{
		SessionId:          sessionId,
		Limit:              10,
		WithAssetPublicUrl: true,
	})
	require.NoError(t, err)

	// Assert
	assert.Len(t, out.Items, 1)
	assert.Len(t, out.PublicUrls, 1)
	urlObj := out.PublicUrls[assetId]
	assert.Contains(t, urlObj.Url, "https://mock/logs/crash.txt")
	assert.WithinDuration(t, time.Now().Add(24*time.Hour), urlObj.ExpireAt, time.Minute)
}

func TestGetMessages_ReturnsErrorOnInvalidCursor(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	d := v1mockdispatcher.NewV1Dispatcher()
	u := v1mockfiler.NewV1Filer(filer.WithContainer("my-bucket"))
	s := v1session.NewV1Service(p, d, u, "worker-queue")

	ctx := context.Background()

	// Act
	out, err := s.GetMessages(ctx, v1session.GetMessagesInput{
		SessionId: uuid.New(),
		Limit:     10,
		Cursor:    "this-is-garbage-base64",
	})

	// Assert
	assert.Error(t, err)
	assert.Nil(t, out)
	assert.ErrorIs(t, err, util.ErrInvalidCursor)
}

func TestGetMessages_HandlesEmpty(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	d := v1mockdispatcher.NewV1Dispatcher()
	u := v1mockfiler.NewV1Filer(filer.WithContainer("my-bucket"))
	s := v1session.NewV1Service(p, d, u, "worker-queue")

	ctx := context.Background()

	// Act
	out, err := s.GetMessages(ctx, v1session.GetMessagesInput{
		SessionId: uuid.New(),
		Limit:     10,
	})
	require.NoError(t, err)

	// Assert
	assert.Empty(t, out.Items)
	assert.False(t, out.HasMore)
	assert.Empty(t, out.NextCursor)
}

func TestCreateSession_SupportsNullableSpace(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	d := v1mockdispatcher.NewV1Dispatcher()
	u := v1mockfiler.NewV1Filer()
	s := v1session.NewV1Service(p, d, u, "q")

	ctx := context.Background()

	// Act
	sess, err := s.Create(ctx, uuid.New(), nil)
	require.NoError(t, err)

	// Assert DB State
	assert.Nil(t, sess.SpaceId)
	persisted, err := p.GetSession(ctx, sess.Id)
	assert.Nil(t, err)
	assert.Nil(t, persisted.SpaceId)
}

func TestFinishSession_Publishes(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	d := v1mockdispatcher.NewV1Dispatcher()
	f := v1mockfiler.NewV1Filer(filer.WithContainer("test-bucket"))
	s := v1session.NewV1Service(p, d, f, "worker-queue")
	sessionId := uuid.New()
	ctx := context.Background()

	// Act
	err := s.FinishSession(ctx, sessionId)
	require.NoError(t, err)

	// Assert: Queue
	assert.Len(t, d.Tasks(), 1)
	qtask := d.Tasks()[0]
	assert.Equal(t, v1.TaskStatusPending, qtask.Status)
	assert.Equal(t, sessionId, qtask.SessionId)

	// Assert: DB
	dbTask, err := p.GetTask(ctx, qtask.Id)
	assert.NoError(t, err)
	assert.Equal(t, sessionId, dbTask.SessionId)
	assert.Equal(t, v1.TaskStatusPending, dbTask.Status)

	var payload v1.TaskPayload
	json.Unmarshal(dbTask.Data, &payload)

	assert.Equal(t, v1.TaskTypeDistill, payload.Type)
	assert.Equal(t, sessionId, payload.SessionId)
}
