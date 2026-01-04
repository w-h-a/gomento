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

func TestProcessJob_Extract_IncludesGlobalFileContext(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	d := v1mockdispatcher.NewV1Dispatcher()
	i := v1mockinterpreter.NewV1Interpreter()
	s := v1worker.NewV1Service(p, d, i)

	ctx := context.Background()

	// 1. Setup Global File
	fileId := uuid.New()
	_ = p.UpsertFileWithAsset(ctx, &v1.File{
		Id:       fileId,
		SpaceId:  nil,
		Path:     "src",
		Filename: "main.go",
	}, &v1.Asset{Id: uuid.New()})

	// 2. Setup Session (No Space needed for global files)
	sessionId := uuid.New()
	_ = p.CreateSession(ctx, &v1.Session{Id: sessionId})

	// 3. Create Job
	payload, _ := json.Marshal(v1.JobPayload{SessionId: sessionId})
	job := &v1.Job{
		Id:      uuid.New(),
		Type:    v1.JobTypeExtract,
		Payload: payload,
		Status:  v1.JobStatusPending,
	}
	_ = p.CreateJob(ctx, job)

	// Act
	err := s.ProcessJob(ctx, job)
	require.NoError(t, err)

	// Assert
	// Verify the interpreter received the global file
	seenFiles := i.ExtractFiles()
	assert.Len(t, seenFiles, 1)
	assert.Equal(t, fileId, seenFiles[0].Id)
	assert.Equal(t, "main.go", seenFiles[0].Filename)
}

func TestProcessJob_Extract_IncludesSpaceFileContext(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	d := v1mockdispatcher.NewV1Dispatcher()
	i := v1mockinterpreter.NewV1Interpreter()
	s := v1worker.NewV1Service(p, d, i)

	ctx := context.Background()
	spaceId := uuid.New()

	// 1. Setup Space File
	fileId := uuid.New()
	_ = p.UpsertFileWithAsset(ctx, &v1.File{
		Id:       fileId,
		SpaceId:  &spaceId,
		Path:     "docs",
		Filename: "readme.md",
	}, &v1.Asset{Id: uuid.New()})

	// 2. Setup Session (Connected to Space)
	sessionId := uuid.New()
	_ = p.CreateSession(ctx, &v1.Session{
		Id:      sessionId,
		SpaceId: &spaceId,
	})

	// 3. Create Job
	payload, _ := json.Marshal(v1.JobPayload{SessionId: sessionId})
	job := &v1.Job{
		Id:      uuid.New(),
		Type:    v1.JobTypeExtract,
		Payload: payload,
		Status:  v1.JobStatusPending,
	}
	_ = p.CreateJob(ctx, job)

	// Act
	err := s.ProcessJob(ctx, job)
	require.NoError(t, err)

	// Assert
	seenFiles := i.ExtractFiles()
	assert.Len(t, seenFiles, 1)
	assert.Equal(t, fileId, seenFiles[0].Id)
	assert.Equal(t, "readme.md", seenFiles[0].Filename)
}

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
			[]interpreter.TaskAction{insertAction},
		),
	)
	s := v1worker.NewV1Service(p, d, i)

	sessionId := uuid.New()
	ctx := context.Background()

	_ = p.CreateSession(ctx, &v1.Session{Id: sessionId})

	payload, _ := json.Marshal(v1.JobPayload{SessionId: sessionId})
	job := &v1.Job{
		Id:      uuid.New(),
		Type:    v1.JobTypeExtract,
		Payload: payload,
		Status:  v1.JobStatusPending,
	}
	_ = p.CreateJob(ctx, job)

	// Act
	err := s.ProcessJob(ctx, job)
	require.NoError(t, err)

	// Assert
	updated := p.Jobs()[job.Id]
	assert.Equal(t, v1.JobStatusSuccess, updated.Status)
	assert.Len(t, p.Skills(), 0, "Extract job should not create skills")

	tasks, _ := p.FetchCurrentTasks(ctx, sessionId, nil)
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
		v1mockinterpreter.WithActionRsp([]interpreter.TaskAction{
			{Type: interpreter.TaskActionFinish},
		}),
	)
	s := v1worker.NewV1Service(p, d, i)

	spaceId := uuid.New()
	sessionId := uuid.New()
	ctx := context.Background()

	_ = p.CreateSession(ctx, &v1.Session{Id: sessionId, SpaceId: &spaceId})

	payload, _ := json.Marshal(v1.JobPayload{SessionId: sessionId})
	job := &v1.Job{
		Id:      uuid.New(),
		Type:    v1.JobTypeDistill,
		Payload: payload,
		Status:  v1.JobStatusPending,
	}
	_ = p.CreateJob(ctx, job)

	// Act
	err := s.ProcessJob(ctx, job)
	require.NoError(t, err)

	// Assert
	updated := p.Jobs()[job.Id]
	assert.Equal(t, v1.JobStatusSuccess, updated.Status)
	assert.Len(t, p.Skills(), 1, "Distill job should create a skill")
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

	spaceId := uuid.New()
	sessionId := uuid.New()
	ctx := context.Background()

	_ = p.CreateSession(ctx, &v1.Session{Id: sessionId, SpaceId: &spaceId})

	// Add messages to be distilled
	_ = p.CreateMessageWithAssets(ctx, &v1.Message{
		SessionId: sessionId, Role: "user", Parts: []v1.Part{{Type: "text", Text: "fix nginx"}},
	}, nil)

	payload, _ := json.Marshal(v1.JobPayload{SessionId: sessionId})
	job := &v1.Job{
		Id:      uuid.New(),
		Type:    v1.JobTypeDistill,
		Payload: payload,
		Status:  v1.JobStatusPending,
	}
	_ = p.CreateJob(ctx, job)

	// Act
	err := s.ProcessJob(ctx, job)
	require.NoError(t, err)

	// Assert
	skills := p.Skills()
	assert.Len(t, skills, 1)

	var savedSkill *v1.Skill
	for _, v := range skills {
		savedSkill = v
	}

	assert.Equal(t, expectedTrigger, savedSkill.Trigger)
	assert.Equal(t, spaceId, savedSkill.SpaceId, "Skill must be linked to the session's space")
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

	// Create session linked to space
	require.NoError(t, p.CreateSession(ctx, &v1.Session{Id: sessionId, SpaceId: &spaceId}))

	// Create OLDER message
	msg1 := &v1.Message{
		Id:        uuid.New(),
		SessionId: sessionId,
		Role:      "user",
		Parts:     []v1.Part{{Type: "text", Text: "First Message"}},
		CreatedAt: time.Now().Add(-10 * time.Minute),
	}
	p.CreateMessageWithAssets(ctx, msg1, nil)

	// Create NEWER message
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

	// Assert: Verify the interpreter received messages in the correct chronological order
	history := i.DistillHistory()
	require.Len(t, history, 2)
	assert.Equal(t, "First Message", history[0].Parts[0].Text)
	assert.Equal(t, "Second Message", history[1].Parts[0].Text)
}

func TestProcessJob_SkipsDistillIfSpaceIsNil(t *testing.T) {
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

	// Session with NO Space
	_ = p.CreateSession(ctx, &v1.Session{
		Id:      sessionId,
		SpaceId: nil,
	})

	payload, _ := json.Marshal(v1.JobPayload{SessionId: sessionId})
	job := &v1.Job{
		Id:      uuid.New(),
		Type:    v1.JobTypeDistill,
		Payload: payload,
		Status:  v1.JobStatusPending,
	}
	_ = p.CreateJob(ctx, job)

	// Act
	err := s.ProcessJob(ctx, job)
	require.NoError(t, err)

	// Assert
	updated := p.Jobs()[job.Id]
	assert.Equal(t, v1.JobStatusSuccess, updated.Status)
	assert.Len(t, p.Skills(), 0, "Should NOT save skill if session has no space")
}

func TestProcessJob_EnforcesJobLock(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	s := v1worker.NewV1Service(p, v1mockdispatcher.NewV1Dispatcher(), v1mockinterpreter.NewV1Interpreter())

	job := &v1.Job{
		Id:     uuid.New(),
		Type:   v1.JobTypeDistill,
		Status: v1.JobStatusRunning,
	}
	_ = p.CreateJob(context.Background(), job)

	// Act
	err := s.ProcessJob(context.Background(), job)
	require.NoError(t, err)

	// Assert
	updated := p.Jobs()[job.Id]
	assert.Equal(t, v1.JobStatusRunning, updated.Status)
}

func TestProcessJob_IgnoresUnknownTaskTypes(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	s := v1worker.NewV1Service(p, v1mockdispatcher.NewV1Dispatcher(), v1mockinterpreter.NewV1Interpreter())

	job := &v1.Job{
		Id:     uuid.New(),
		Type:   "unknown_job_type",
		Status: v1.JobStatusPending,
	}
	_ = p.CreateJob(context.Background(), job)

	// Act
	err := s.ProcessJob(context.Background(), job)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown job type")
}
