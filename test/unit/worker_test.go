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
	"github.com/w-h-a/gomento/internal/client/interpreter"
	v1mockinterpreter "github.com/w-h-a/gomento/internal/client/interpreter/v1_mock"
	v1mockpersister "github.com/w-h-a/gomento/internal/client/persister/v1_mock"
	v1worker "github.com/w-h-a/gomento/internal/service/v1_worker"
)

func TestProcessJob_Checkpoint_UpdatesTasksOnly(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	d := v1mockdispatcher.NewV1Dispatcher()

	insertAction := interpreter.TaskAction{
		Type:    interpreter.TaskActionInsert,
		Payload: map[string]any{"after_task_order": 0.0, "task_description": "New Task"},
	}
	i := v1mockinterpreter.NewV1Interpreter(
		v1mockinterpreter.WithActionRsp(
			[]interpreter.TaskAction{
				insertAction,
			},
		),
	)

	s := v1worker.NewV1Service(p, d, i)

	sessionId := uuid.New()
	spaceId := uuid.New()
	ctx := context.Background()

	err := p.CreateSession(ctx, &v1.Session{
		Id:        sessionId,
		SpaceId:   &spaceId,
		CreatedAt: time.Now(),
	})
	require.NoError(t, err)

	payload, _ := json.Marshal(v1.JobPayload{SessionId: sessionId})
	job := &v1.Job{
		Id:      uuid.New(),
		Type:    v1.JobTypeExtract,
		Payload: payload,
		Status:  v1.JobStatusPending,
	}
	err = p.CreateJob(ctx, job)
	require.NoError(t, err)

	// Act
	err = s.ProcessJob(ctx, job)
	require.NoError(t, err)

	// Assert
	updated := p.Jobs()[job.Id]
	assert.Equal(t, v1.JobStatusSuccess, updated.Status)

	skills := p.Skills()
	assert.Len(t, skills, 0)

	tasks, err := p.FetchCurrentTasks(ctx, sessionId, nil)
	assert.NoError(t, err)
	assert.Len(t, tasks, 3)
}

func TestProcessJob_Finalize_UpdatesTasksAndDistills(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	d := v1mockdispatcher.NewV1Dispatcher()

	i := v1mockinterpreter.NewV1Interpreter(
		v1mockinterpreter.WithActionRsp(
			[]interpreter.TaskAction{
				{
					Type: interpreter.TaskActionFinish,
				},
			},
		),
	)

	s := v1worker.NewV1Service(p, d, i)

	sessionId := uuid.New()
	spaceId := uuid.New()
	ctx := context.Background()

	err := p.CreateSession(ctx, &v1.Session{
		Id:        sessionId,
		SpaceId:   &spaceId,
		CreatedAt: time.Now(),
	})
	require.NoError(t, err)

	payload, _ := json.Marshal(v1.JobPayload{SessionId: sessionId})
	job := &v1.Job{
		Id:      uuid.New(),
		Type:    v1.JobTypeDistill,
		Payload: payload,
		Status:  v1.JobStatusPending,
	}
	err = p.CreateJob(ctx, job)
	require.NoError(t, err)

	// Act
	err = s.ProcessJob(ctx, job)
	require.NoError(t, err)

	// Assert
	updated := p.Jobs()[job.Id]
	assert.Equal(t, v1.JobStatusSuccess, updated.Status)

	skills := p.Skills()
	assert.Len(t, skills, 1)

	tasks, err := p.FetchCurrentTasks(ctx, sessionId, nil)
	assert.NoError(t, err)
	assert.Len(t, tasks, 0)
}

func TestProcessJob_DistillsAndSavesSkill(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	d := v1mockdispatcher.NewV1Dispatcher()

	expectedTrigger := "how to fix nginx"
	i := v1mockinterpreter.NewV1Interpreter(
		v1mockinterpreter.WithSkillRsp(&v1.Skill{
			Id:        uuid.New(),
			Trigger:   expectedTrigger,
			SOP:       "1. restart nginx",
			Embedding: make([]float32, 1536),
		}),
	)

	s := v1worker.NewV1Service(p, d, i)

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

	payload := v1.JobPayload{
		SessionId: sessionId,
	}
	data, _ := json.Marshal(payload)
	job := &v1.Job{
		Id:      uuid.New(),
		Type:    v1.JobTypeDistill,
		Payload: data,
		Status:  v1.JobStatusPending,
	}
	err = p.CreateJob(ctx, job)
	require.NoError(t, err)

	// Act
	err = s.ProcessJob(ctx, job)
	require.NoError(t, err)

	// Assert State
	updatedTask := p.Jobs()[job.Id]
	assert.Equal(t, v1.JobStatusSuccess, updatedTask.Status)

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

func TestProcessJob_ProcessingOrder(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	d := v1mockdispatcher.NewV1Dispatcher()
	i := v1mockinterpreter.NewV1Interpreter(
		v1mockinterpreter.WithSkillRsp(&v1.Skill{
			Id:        uuid.New(),
			Trigger:   "test",
			SOP:       "test",
			Embedding: []float32{0.1},
		}),
	)

	s := v1worker.NewV1Service(p, d, i)

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

	payload := v1.JobPayload{SessionId: sessionId}
	data, _ := json.Marshal(payload)
	job := &v1.Job{Id: uuid.New(), Type: v1.JobTypeDistill, Payload: data, Status: v1.JobStatusPending}
	p.CreateJob(ctx, job)

	// Act
	err := s.ProcessJob(ctx, job)
	require.NoError(t, err)

	// Assert: Check the Observable Behavior
	assert.Len(t, i.DistillHistory(), 2)
	assert.Equal(t, "First Message", i.DistillHistory()[0].Parts[0].Text)
	assert.Equal(t, "Second Message", i.DistillHistory()[1].Parts[0].Text)
}

func TestProcessJob_EnforcesJobLock(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	s := v1worker.NewV1Service(p, v1mockdispatcher.NewV1Dispatcher(), v1mockinterpreter.NewV1Interpreter())

	sessionId := uuid.New()
	spaceId := uuid.New()
	ctx := context.Background()

	p.CreateSession(ctx, &v1.Session{Id: sessionId, SpaceId: &spaceId})

	payload := v1.JobPayload{SessionId: sessionId}
	data, _ := json.Marshal(payload)
	job := &v1.Job{
		Id:      uuid.New(),
		Type:    v1.JobTypeDistill,
		Payload: data,
		Status:  v1.JobStatusRunning,
	}
	p.CreateJob(ctx, job)

	// Act
	err := s.ProcessJob(ctx, job)
	require.NoError(t, err)

	// Assert
	updated := p.Jobs()[job.Id]
	assert.Equal(t, v1.JobStatusRunning, updated.Status)
}

func TestProcessJob_SkipsIfSpaceIsNil(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	d := v1mockdispatcher.NewV1Dispatcher()
	i := v1mockinterpreter.NewV1Interpreter()
	s := v1worker.NewV1Service(p, d, i)

	sessionId := uuid.New()
	ctx := context.Background()

	require.NoError(t, p.CreateSession(ctx, &v1.Session{
		Id:        sessionId,
		SpaceId:   nil,
		CreatedAt: time.Now(),
	}))

	payload := v1.JobPayload{SessionId: sessionId}
	data, _ := json.Marshal(payload)
	job := &v1.Job{Id: uuid.New(), Type: v1.JobTypeDistill, Payload: data, Status: v1.JobStatusPending}
	p.CreateJob(ctx, job)

	// Act
	err := s.ProcessJob(ctx, job)
	require.NoError(t, err)

	// Assert
	updatedTask := p.Jobs()[job.Id]
	assert.Equal(t, v1.JobStatusSuccess, updatedTask.Status)
	assert.Len(t, p.Skills(), 0)
}

func TestProcessJob_IgnoresUnknownTaskTypes(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	svc := v1worker.NewV1Service(
		p,
		v1mockdispatcher.NewV1Dispatcher(),
		v1mockinterpreter.NewV1Interpreter(),
	)

	payload := v1.JobPayload{}
	data, _ := json.Marshal(payload)
	job := &v1.Job{
		Id:      uuid.New(),
		Type:    "unknown",
		Payload: data,
		Status:  v1.JobStatusPending,
	}

	p.CreateJob(context.Background(), job)

	// Act
	err := svc.ProcessJob(context.Background(), job)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown job type")
}
