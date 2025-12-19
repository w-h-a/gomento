package unit

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	v1mockdispatcher "github.com/w-h-a/gomento/internal/client/dispatcher/v1_mock"
	v1mockpersister "github.com/w-h-a/gomento/internal/client/persister/v1_mock"
	v1session "github.com/w-h-a/gomento/internal/service/v1_session"
)

func TestAddMessage_PersistsToSessionHistory(t *testing.T) {
	// Arrange
	p := v1mockpersister.NewV1Persister()
	d := v1mockdispatcher.NewV1Dispatcher()
	s := v1session.NewV1Service(p, d, "worker-queue")
	sessionId := uuid.New()
	ctx := context.Background()

	// Act
	err := s.AddMessage(ctx, sessionId, "user", "Hello World")
	require.NoError(t, err)

	// Assert: State
	msgs, err := p.GetMessages(ctx, sessionId)
	assert.NoError(t, err)
	assert.Len(t, msgs, 1)
	assert.Equal(t, "Hello World", msgs[0].Content)
	assert.Equal(t, "user", msgs[0].Role)
}

func TestFinishSession_PublishesDistillTask(t *testing.T) {
	// Arrange
	p := v1mockpersister.NewV1Persister()
	d := v1mockdispatcher.NewV1Dispatcher()
	s := v1session.NewV1Service(p, d, "worker-queue")
	sessionId := uuid.New()
	ctx := context.Background()

	// Act
	err := s.FinishSession(ctx, sessionId)
	require.NoError(t, err)

	// Assert: State
	assert.Len(t, d.Tasks(), 1)
	task := d.Tasks()[0]
	assert.Equal(t, v1.TaskTypeDistill, task.Type)
	assert.Equal(t, sessionId.String(), task.Payload["session_id"])
}
