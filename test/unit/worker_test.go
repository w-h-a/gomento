package unit

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	v1mockdispatcher "github.com/w-h-a/gomento/internal/client/dispatcher/v1_mock"
	mockembedder "github.com/w-h-a/gomento/internal/client/embedder/mock"
	v1mockfiler "github.com/w-h-a/gomento/internal/client/filer/v1_mock"
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
	e := mockembedder.NewEmbedder()
	s := v1worker.NewV1Service(p, d, v1mockfiler.NewV1Filer(), i, e)

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

	// 3. Add a message so extraction runs
	_ = p.CreateMessageWithAssets(ctx, &v1.Message{
		SessionId: sessionId,
		Role:      "user",
		Parts:     []v1.Part{{Type: "text", Text: "Analyze this"}},
	}, nil)

	// 4. Create Job
	payload, _ := json.Marshal(v1.SessionJobPayload{SessionId: sessionId})
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
	e := mockembedder.NewEmbedder()
	s := v1worker.NewV1Service(p, d, v1mockfiler.NewV1Filer(), i, e)

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

	// 3. Add a message
	_ = p.CreateMessageWithAssets(ctx, &v1.Message{
		SessionId: sessionId,
		Role:      "user",
		Parts:     []v1.Part{{Type: "text", Text: "Help me"}},
	}, nil)

	// 4. Create Job
	payload, _ := json.Marshal(v1.SessionJobPayload{SessionId: sessionId})
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

func TestProcessJob_Extract_UpdatesTasksOnly(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	d := v1mockdispatcher.NewV1Dispatcher()

	i := v1mockinterpreter.NewV1Interpreter(
		v1mockinterpreter.WithExtractRsp(
			[]interpreter.TaskAction{
				{
					Type:    interpreter.TaskActionInsert,
					Payload: map[string]any{"after_task_order": 0.0, "task_description": "New Task"},
				},
			},
		),
	)
	e := mockembedder.NewEmbedder()
	s := v1worker.NewV1Service(p, d, v1mockfiler.NewV1Filer(), i, e)

	sessionId := uuid.New()
	ctx := context.Background()

	_ = p.CreateSession(ctx, &v1.Session{Id: sessionId})

	_ = p.CreateMessageWithAssets(ctx, &v1.Message{
		SessionId: sessionId, Role: "user", Parts: []v1.Part{{Type: "text", Text: "do work"}},
	}, nil)

	payload, _ := json.Marshal(v1.SessionJobPayload{SessionId: sessionId})
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
	assert.Len(t, tasks, 3, "Extract job should create tasks")
}

func TestProcessJob_Distill_DistillsSkillsOnly(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	d := v1mockdispatcher.NewV1Dispatcher()
	i := v1mockinterpreter.NewV1Interpreter(
		v1mockinterpreter.WithDistillRsp(
			[]interpreter.SkillAction{
				{
					Type: interpreter.SkillActionInsert,
					Payload: map[string]any{
						"trigger": "how to fix nginx",
						"sop":     "1. restart nginx",
					},
				},
			},
		),
	)
	e := mockembedder.NewEmbedder()
	s := v1worker.NewV1Service(p, d, v1mockfiler.NewV1Filer(), i, e)

	spaceId := uuid.New()
	sessionId := uuid.New()
	ctx := context.Background()

	_ = p.CreateSession(ctx, &v1.Session{Id: sessionId, SpaceId: &spaceId})

	_ = p.CreateMessageWithAssets(ctx, &v1.Message{
		SessionId: sessionId, Role: "user", Parts: []v1.Part{{Type: "text", Text: "learn this"}},
	}, nil)

	payload, _ := json.Marshal(v1.SessionJobPayload{SessionId: sessionId})
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

	tasks, _ := p.FetchCurrentTasks(ctx, sessionId, nil)
	assert.Len(t, tasks, 0, "Distill job should not create tasks")

	skills, _ := p.FetchCurrentSkills(ctx, spaceId)
	assert.Len(t, skills, 1, "Distill job should create a skill")
}

func TestProcessJob_Distill_InsertsNewSkill(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	d := v1mockdispatcher.NewV1Dispatcher()

	expectedTrigger := "how to fix nginx"
	i := v1mockinterpreter.NewV1Interpreter(
		v1mockinterpreter.WithDistillRsp(
			[]interpreter.SkillAction{
				{
					Type: interpreter.SkillActionInsert,
					Payload: map[string]any{
						"trigger": expectedTrigger,
						"sop":     "1. restart nginx",
					},
				},
			},
		),
	)
	e := mockembedder.NewEmbedder()
	s := v1worker.NewV1Service(p, d, v1mockfiler.NewV1Filer(), i, e)

	spaceId := uuid.New()
	sessionId := uuid.New()
	ctx := context.Background()

	_ = p.CreateSession(ctx, &v1.Session{Id: sessionId, SpaceId: &spaceId})

	// Add messages to be distilled
	_ = p.CreateMessageWithAssets(ctx, &v1.Message{
		SessionId: sessionId, Role: "user", Parts: []v1.Part{{Type: "text", Text: "fix nginx"}},
	}, nil)

	payload, _ := json.Marshal(v1.SessionJobPayload{SessionId: sessionId})
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

	assert.NotEmpty(t, savedSkill.Embedding)
	assert.Equal(t, expectedTrigger, e.Input())
}

func TestProcessJob_Distill_UpdatesExistingSkill(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	e := mockembedder.NewEmbedder()

	spaceId := uuid.New()
	sessionId := uuid.New()
	skillId := uuid.New()
	ctx := context.Background()

	_ = p.CreateSession(ctx, &v1.Session{Id: sessionId, SpaceId: &spaceId})

	existingSkill := &v1.Skill{
		Id:      skillId,
		SpaceId: spaceId,
		Trigger: "old trigger",
		SOP:     "old sop",
	}
	_ = p.SaveSkill(ctx, existingSkill)

	_ = p.CreateMessageWithAssets(ctx, &v1.Message{
		SessionId: sessionId, Role: "user", Parts: []v1.Part{{Type: "text", Text: "update skill"}},
	}, nil)

	newTrigger := "updated trigger"
	i := v1mockinterpreter.NewV1Interpreter(
		v1mockinterpreter.WithDistillRsp(
			[]interpreter.SkillAction{
				{
					Type: interpreter.SkillActionUpdate,
					Payload: map[string]any{
						"skill_id": skillId.String(),
						"trigger":  newTrigger,
						"sop":      "updated sop",
					},
				},
			},
		),
	)
	s := v1worker.NewV1Service(p, v1mockdispatcher.NewV1Dispatcher(), v1mockfiler.NewV1Filer(), i, e)

	payload, _ := json.Marshal(v1.SessionJobPayload{SessionId: sessionId})
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
	updatedSkill := p.Skills()[skillId]
	assert.Equal(t, skillId, updatedSkill.Id)
	assert.Equal(t, newTrigger, updatedSkill.Trigger)
	assert.Equal(t, "updated sop", updatedSkill.SOP)
	assert.Equal(t, newTrigger, e.Input())
}

func TestProcessJob_Distill_IncludesSkillContext(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	d := v1mockdispatcher.NewV1Dispatcher()
	i := v1mockinterpreter.NewV1Interpreter()
	e := mockembedder.NewEmbedder()
	s := v1worker.NewV1Service(p, d, v1mockfiler.NewV1Filer(), i, e)

	ctx := context.Background()
	spaceId := uuid.New()
	sessionId := uuid.New()

	// 1. Setup Session & Space
	_ = p.CreateSession(ctx, &v1.Session{Id: sessionId, SpaceId: &spaceId})

	// 2. Setup Existing Skill
	existingSkill := &v1.Skill{
		Id:      uuid.New(),
		SpaceId: spaceId,
		Trigger: "how to check logs",
		SOP:     "run docker logs",
	}
	_ = p.SaveSkill(ctx, existingSkill)

	// 3. Add Message
	_ = p.CreateMessageWithAssets(ctx, &v1.Message{
		SessionId: sessionId, Role: "user", Parts: []v1.Part{{Type: "text", Text: "check logs"}},
	}, nil)

	// 4. Create Job
	payload, _ := json.Marshal(v1.SessionJobPayload{SessionId: sessionId})
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
	contextSkills := i.DistillSkills()
	assert.Len(t, contextSkills, 1)
	assert.Equal(t, existingSkill.Trigger, contextSkills[0].Trigger)
	assert.Equal(t, existingSkill.Id, contextSkills[0].Id)
}

func TestProcessJob_Distills_FailsIfEmbedderFails(t *testing.T) {
	// Arrange
	p := v1mockpersister.NewV1Persister()
	d := v1mockdispatcher.NewV1Dispatcher()
	i := v1mockinterpreter.NewV1Interpreter(
		v1mockinterpreter.WithDistillRsp(
			[]interpreter.SkillAction{
				{
					Type: interpreter.SkillActionInsert,
					Payload: map[string]any{
						"trigger": "trigger",
						"sop":     "sop",
					},
				},
			},
		),
	)
	e := mockembedder.NewEmbedder(mockembedder.WithError(errors.New("openai down")))
	s := v1worker.NewV1Service(p, d, v1mockfiler.NewV1Filer(), i, e)

	ctx := context.Background()

	spaceId := uuid.New()
	sessionId := uuid.New()

	_ = p.CreateSession(ctx, &v1.Session{Id: sessionId, SpaceId: &spaceId})

	_ = p.CreateMessageWithAssets(ctx, &v1.Message{
		SessionId: sessionId, Role: "user", Parts: []v1.Part{{Type: "text", Text: "trigger error"}},
	}, nil)

	payload, _ := json.Marshal(v1.SessionJobPayload{SessionId: sessionId})
	job := &v1.Job{
		Id:      uuid.New(),
		Type:    v1.JobTypeDistill,
		Payload: payload,
		Status:  v1.JobStatusPending,
	}
	p.CreateJob(ctx, job)

	// Act
	err := s.ProcessJob(ctx, job)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "openai down")
	assert.Len(t, p.Skills(), 0)
}

func TestProcessJob_IngestFile_CalculatesAndPersistsEmbedding(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	d := v1mockdispatcher.NewV1Dispatcher()
	f := v1mockfiler.NewV1Filer()
	e := mockembedder.NewEmbedder()
	s := v1worker.NewV1Service(p, d, f, v1mockinterpreter.NewV1Interpreter(), e)

	ctx := context.Background()

	fileId := uuid.New()
	content := "This is the file content"

	asset, err := f.Upload(ctx, strings.NewReader(content), "doc.txt", "text/plain", int64(len(content)))
	require.NoError(t, err)

	err = p.UpsertFileWithAsset(ctx, &v1.File{
		Id:       fileId,
		Filename: "doc.txt",
	}, asset)
	require.NoError(t, err)

	payload, _ := json.Marshal(v1.IngestFileJobPayload{
		FileId: fileId,
	})
	job := &v1.Job{
		Id:      uuid.New(),
		Type:    v1.JobTypeIngestFile,
		Payload: payload,
		Status:  v1.JobStatusPending,
	}
	err = p.CreateJob(ctx, job)
	require.NoError(t, err)

	// Act
	err = s.ProcessJob(ctx, job)
	require.NoError(t, err)

	// Assert
	updatedJob := p.Jobs()[job.Id]
	assert.Equal(t, v1.JobStatusSuccess, updatedJob.Status)

	updatedFile, err := p.GetFile(ctx, fileId)
	assert.NoError(t, err)
	assert.NotNil(t, updatedFile)
	assert.Equal(t, float32(0.01), updatedFile.Embedding[0])

	assert.Contains(t, e.Input(), content)
}

func TestProcessJob_EmbedMessage_CalculatesAndPersistsEmbedding(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	d := v1mockdispatcher.NewV1Dispatcher()
	f := v1mockfiler.NewV1Filer()
	e := mockembedder.NewEmbedder()
	s := v1worker.NewV1Service(p, d, f, v1mockinterpreter.NewV1Interpreter(), e)

	ctx := context.Background()

	msgId := uuid.New()

	err := p.CreateMessageWithAssets(ctx, &v1.Message{
		Id:    msgId,
		Role:  "user",
		Parts: []v1.Part{{Type: "text", Text: "Embed me please"}},
	}, nil)
	require.NoError(t, err)

	payload, _ := json.Marshal(v1.EmbedMessageJobPayload{
		MessageId: msgId,
	})
	job := &v1.Job{
		Id:      uuid.New(),
		Type:    v1.JobTypeEmbedMessage,
		Payload: payload,
		Status:  v1.JobStatusPending,
	}
	p.CreateJob(ctx, job)

	// Act
	err = s.ProcessJob(ctx, job)
	require.NoError(t, err)

	// Assert
	updatedJob := p.Jobs()[job.Id]
	assert.Equal(t, v1.JobStatusSuccess, updatedJob.Status)

	updatedMsg, err := p.GetMessage(ctx, msgId)
	require.NoError(t, err)
	assert.NotNil(t, updatedMsg)
	assert.Equal(t, float32(0.01), updatedMsg.Embedding[0])

	assert.Contains(t, e.Input(), "Embed me please")
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
		v1mockinterpreter.WithDistillRsp(
			[]interpreter.SkillAction{
				{
					Type: interpreter.SkillActionInsert,
					Payload: map[string]any{
						"trigger": "test",
						"sop":     "test",
					},
				},
			},
		),
	)
	e := mockembedder.NewEmbedder()
	s := v1worker.NewV1Service(p, d, v1mockfiler.NewV1Filer(), i, e)

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

	payload := v1.SessionJobPayload{SessionId: sessionId}
	data, _ := json.Marshal(payload)
	job := &v1.Job{Id: uuid.New(), Type: v1.JobTypeDistill, Payload: data, Status: v1.JobStatusPending}
	p.CreateJob(ctx, job)

	// Act
	err := s.ProcessJob(ctx, job)
	require.NoError(t, err)

	// Assert: Verify the interpreter received messages in the correct chronological order
	history := i.DistillHistory()
	assert.Len(t, history, 2)
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
	e := mockembedder.NewEmbedder()
	s := v1worker.NewV1Service(p, d, v1mockfiler.NewV1Filer(), i, e)

	sessionId := uuid.New()
	ctx := context.Background()

	// Session with NO Space
	_ = p.CreateSession(ctx, &v1.Session{
		Id:      sessionId,
		SpaceId: nil,
	})

	payload, _ := json.Marshal(v1.SessionJobPayload{SessionId: sessionId})
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
	s := v1worker.NewV1Service(p, v1mockdispatcher.NewV1Dispatcher(), v1mockfiler.NewV1Filer(), v1mockinterpreter.NewV1Interpreter(), mockembedder.NewEmbedder())

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
	s := v1worker.NewV1Service(p, v1mockdispatcher.NewV1Dispatcher(), v1mockfiler.NewV1Filer(), v1mockinterpreter.NewV1Interpreter(), mockembedder.NewEmbedder())

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
