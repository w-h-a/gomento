package unit

import (
	"context"
	"encoding/json"
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
		SpaceId:   &spaceId,
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

	payload := v1.TaskPayload{
		Type:      v1.TaskTypeDistill,
		SessionId: sessionId,
	}
	data, _ := json.Marshal(payload)
	task := &v1.Task{
		Id:        uuid.New(),
		SessionId: sessionId,
		Data:      data,
		Status:    v1.TaskStatusPending,
	}
	err = p.CreateTask(ctx, task)
	require.NoError(t, err)

	// Act
	err = s.ProcessTask(ctx, task)
	require.NoError(t, err)

	// Assert State
	updatedTask, err := p.GetTask(ctx, task.Id)
	assert.NoError(t, err)
	assert.Equal(t, v1.TaskStatusSuccess, updatedTask.Status)

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

func TestProcessTask_ProcessingOrder(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	disp := v1mockdispatcher.NewV1Dispatcher()
	dist := v1mockdistiller.NewV1Distiller(
		v1mockdistiller.WithSkillRsp(&v1.Skill{
			Id:        uuid.New(),
			Trigger:   "test",
			SOP:       "test",
			Embedding: []float32{0.1},
		}),
	)

	s := v1worker.NewV1Service(p, disp, dist)

	sessionId := uuid.New()
	spaceId := uuid.New()
	ctx := context.Background()

	require.NoError(t, p.CreateSession(ctx, &v1.Session{Id: sessionId, SpaceId: &spaceId}))

	msg1 := &v1.Message{
		Id:        uuid.New(),
		SessionId: sessionId,
		Role:      "user",
		Parts:     []v1.Part{{Type: "text", Text: "First Message"}},
		CreatedAt: time.Now().Add(-10 * time.Minute),
	}
	p.CreateMessageWithAssets(ctx, msg1, nil)

	msg2 := &v1.Message{
		Id:        uuid.New(),
		SessionId: sessionId,
		Role:      "assistant",
		Parts:     []v1.Part{{Type: "text", Text: "Second Message"}},
		CreatedAt: time.Now().Add(-5 * time.Minute),
	}
	p.CreateMessageWithAssets(ctx, msg2, nil)

	payload := v1.TaskPayload{Type: v1.TaskTypeDistill, SessionId: sessionId}
	data, _ := json.Marshal(payload)
	task := &v1.Task{Id: uuid.New(), SessionId: sessionId, Data: data, Status: v1.TaskStatusPending}
	p.CreateTask(ctx, task)

	// Act
	err := s.ProcessTask(ctx, task)
	require.NoError(t, err)

	// Assert: Check the Observable Behavior
	assert.Len(t, dist.History(), 2)
	assert.Equal(t, "First Message", dist.History()[0].Parts[0].Text)
	assert.Equal(t, "Second Message", dist.History()[1].Parts[0].Text)
}

func TestProcessTask_EnforcesSessionLock(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	s := v1worker.NewV1Service(p, v1mockdispatcher.NewV1Dispatcher(), v1mockdistiller.NewV1Distiller())
	ctx := context.Background()
	sessionId := uuid.New()

	ghostTask := &v1.Task{
		Id:        uuid.New(),
		SessionId: sessionId,
		Status:    v1.TaskStatusRunning,
	}
	p.CreateTask(ctx, ghostTask)

	payload := v1.TaskPayload{Type: v1.TaskTypeDistill, SessionId: sessionId}
	data, _ := json.Marshal(payload)
	newTask := &v1.Task{
		Id:        uuid.New(),
		SessionId: sessionId,
		Data:      data,
		Status:    v1.TaskStatusPending,
	}
	p.CreateTask(ctx, newTask)

	// Act
	err := s.ProcessTask(ctx, newTask)

	// Assert: Should Fail due to Atomic Lock check
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session")
	assert.Contains(t, err.Error(), "locked")

	// Assert: Should be Failed in DB
	updated, _ := p.GetTask(ctx, newTask.Id)
	assert.Equal(t, v1.TaskStatusFailed, updated.Status)

	// Assert: Ghost task should be Running still
	ghost, _ := p.GetTask(ctx, ghostTask.Id)
	assert.Equal(t, v1.TaskStatusRunning, ghost.Status)
}

func TestProcessTask_SkipsIfSpaceIsNil(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	dist := v1mockdistiller.NewV1Distiller()
	s := v1worker.NewV1Service(p, v1mockdispatcher.NewV1Dispatcher(), dist)

	sessionId := uuid.New()
	ctx := context.Background()

	require.NoError(t, p.CreateSession(ctx, &v1.Session{
		Id:        sessionId,
		SpaceId:   nil,
		CreatedAt: time.Now(),
	}))

	payload := v1.TaskPayload{Type: v1.TaskTypeDistill, SessionId: sessionId}
	data, _ := json.Marshal(payload)
	task := &v1.Task{Id: uuid.New(), SessionId: sessionId, Data: data, Status: v1.TaskStatusPending}
	p.CreateTask(ctx, task)

	// Act
	err := s.ProcessTask(ctx, task)
	require.NoError(t, err)

	// Assert
	updatedTask, err := p.GetTask(ctx, task.Id)
	assert.NoError(t, err)
	assert.Equal(t, v1.TaskStatusSuccess, updatedTask.Status)
	assert.Len(t, p.Skills(), 0)
}

func TestProcessTask_IgnoresUnknownTaskTypes(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	svc := v1worker.NewV1Service(
		p,
		v1mockdispatcher.NewV1Dispatcher(),
		v1mockdistiller.NewV1Distiller(),
	)

	payload := v1.TaskPayload{Type: "unknown"}
	data, _ := json.Marshal(payload)
	task := &v1.Task{
		Id:        uuid.New(),
		SessionId: uuid.New(),
		Data:      data,
		Status:    v1.TaskStatusPending,
	}

	p.CreateTask(context.Background(), task)

	// Act
	err := svc.ProcessTask(context.Background(), task)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown task type")
}
