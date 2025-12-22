package unit

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	v1mockdispatcher "github.com/w-h-a/gomento/internal/client/dispatcher/v1_mock"
	v1mockdistiller "github.com/w-h-a/gomento/internal/client/distiller/v1_mock"
	v1mockpersister "github.com/w-h-a/gomento/internal/client/persister/v1_mock"
	v1worker "github.com/w-h-a/gomento/internal/service/v1_worker"
)

func TestProcessTask_DistillsAndSavesSkill(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()

	disp := v1mockdispatcher.NewV1Dispatcher()

	expectedTrigger := "how to fix nginx"
	dist := v1mockdistiller.NewV1Distiller(
		v1mockdistiller.WithSkillRsp(&v1.Skill{
			Id:        uuid.New(),
			Trigger:   expectedTrigger,
			SOP:       "1. restart nginx",
			Embedding: make([]float32, 1536),
		}),
	)

	s := v1worker.NewV1Service(p, disp, dist)

	sessionId := uuid.New()
	spaceId := uuid.New()

	ctx := context.Background()

	err := p.CreateSession(ctx, &v1.Session{
		Id:        sessionId,
		SpaceId:   spaceId,
		CreatedAt: time.Now(),
	})
	require.NoError(t, err)

	err = p.CreateMessageWithAssets(
		ctx,
		&v1.Message{
			SessionId: sessionId,
			Role:      "user",
			Parts: []v1.Part{
				{
					Type: "text",
					Text: "nginx is broken",
				},
			},
		},
		map[int]*v1.Asset{},
	)
	require.NoError(t, err)

	err = p.CreateMessageWithAssets(
		ctx,
		&v1.Message{
			SessionId: sessionId,
			Role:      "assistant",
			Parts: []v1.Part{
				{
					Type: "text",
					Text: "try restarting it",
				},
			},
		},
		map[int]*v1.Asset{},
	)
	require.NoError(t, err)

	task := &v1.Task{
		Type: v1.TaskTypeDistill,
		Payload: v1.Payload{
			SessionId: sessionId,
		},
	}

	// Act
	err = s.ProcessTask(ctx, task)
	require.NoError(t, err)

	// Assert State
	assert.Len(t, p.Skills(), 1)

	var savedSkill *v1.Skill
	for _, s := range p.Skills() {
		savedSkill = s
	}

	// Assert Business Logic
	// 1. The Skill content came from the Distiller
	assert.Equal(t, expectedTrigger, savedSkill.Trigger)

	// 2. The Skill was correctly linked to the Session's Space
	assert.Equal(t, spaceId, savedSkill.SpaceId)
}

func TestProcessTask_IgnoresUnknownTaskTypes(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	svc := v1worker.NewV1Service(
		v1mockpersister.NewV1Persister(),
		v1mockdispatcher.NewV1Dispatcher(),
		v1mockdistiller.NewV1Distiller(),
	)

	// Act
	err := svc.ProcessTask(context.Background(), &v1.Task{
		Type: "unknown_type",
	})

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown task type")
}
