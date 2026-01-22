package v1worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/dispatcher"
	"github.com/w-h-a/gomento/internal/client/embedder"
	"github.com/w-h-a/gomento/internal/client/filer"
	"github.com/w-h-a/gomento/internal/client/interpreter"
	"github.com/w-h-a/gomento/internal/client/persister"
	"github.com/w-h-a/gomento/internal/service"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	maxFileBytes = 10 * 1024
)

type V1Service struct {
	*service.Service
	persister   persister.V1Persister
	dispatcher  dispatcher.V1Dispatcher
	filer       filer.V1Filer
	interpreter interpreter.V1Interpreter
	embedder    embedder.Embedder
	tracer      trace.Tracer
}

func (s *V1Service) Subscribe(ctx context.Context, cb func(context.Context, *v1.Job) error, qname string) error {
	return s.dispatcher.Subscribe(ctx, cb, dispatcher.SubscribeWithQueue(qname))
}

func (s *V1Service) Close(ctx context.Context) error {
	return s.dispatcher.Close(ctx)
}

func (s *V1Service) ProcessJob(ctx context.Context, job *v1.Job) error {
	ctx, span := s.tracer.Start(ctx, "worker.ProcessJob")
	defer span.End()

	span.SetAttributes(
		attribute.String("job.id", job.Id.String()),
		attribute.String("job.type", job.Type),
	)

	if err := s.persister.AcquireJobLock(ctx, job.Id); err != nil {
		if errors.Is(err, persister.ErrJobLocked) {
			slog.WarnContext(ctx, "job locked", "job_id", job.Id)
			span.AddEvent("job_locked")
			return nil
		}
		s.persister.UpdateJobStatus(ctx, job.Id, v1.JobStatusFailed)
		span.RecordError(err)
		return fmt.Errorf("failed to acquire job lock: %w", err)
	}

	if job.Type != v1.JobTypeDistill && job.Type != v1.JobTypeExtract && job.Type != v1.JobTypeIngestFile {
		s.persister.UpdateJobStatus(ctx, job.Id, v1.JobStatusFailed)
		err := fmt.Errorf("unknown job type: %s", job.Type)
		span.RecordError(err)
		return err
	}

	switch job.Type {
	case v1.JobTypeExtract:
		var payload v1.SessionJobPayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			s.persister.UpdateJobStatus(ctx, job.Id, v1.JobStatusFailed)
			span.RecordError(err)
			return fmt.Errorf("invalid job payload: %w", err)
		}
		if err := s.extract(ctx, payload.SessionId, payload.MessageWindow); err != nil {
			s.persister.UpdateJobStatus(ctx, job.Id, v1.JobStatusFailed)
			span.RecordError(err)
			return err
		}
	case v1.JobTypeDistill:
		var payload v1.SessionJobPayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			s.persister.UpdateJobStatus(ctx, job.Id, v1.JobStatusFailed)
			span.RecordError(err)
			return fmt.Errorf("invalid job payload: %w", err)
		}
		if err := s.distill(ctx, payload.SessionId, payload.MessageWindow); err != nil {
			s.persister.UpdateJobStatus(ctx, job.Id, v1.JobStatusFailed)
			span.RecordError(err)
			return err
		}
	case v1.JobTypeIngestFile:
		var payload v1.IngestFileJobPayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			s.persister.UpdateJobStatus(ctx, job.Id, v1.JobStatusFailed)
			span.RecordError(err)
			return fmt.Errorf("invalid job payload: %w", err)
		}
		if err := s.ingestFile(ctx, payload.FileId, payload.SpaceId); err != nil {
			s.persister.UpdateJobStatus(ctx, job.Id, v1.JobStatusFailed)
			span.RecordError(err)
			return err
		}
	}

	slog.InfoContext(ctx, "job success", "job_id", job.Id)
	span.AddEvent("job_success")

	return s.persister.UpdateJobStatus(ctx, job.Id, v1.JobStatusSuccess)
}

func (s *V1Service) extract(ctx context.Context, sessionId uuid.UUID, messageWindow int) error {
	ctx, span := s.tracer.Start(ctx, "worker.extract")
	defer span.End()

	sess, err := s.persister.GetSession(ctx, sessionId)
	if err != nil {
		span.RecordError(err)
		return err
	}
	if sess == nil {
		return service.ErrSessionNotFound
	}

	var files []v1.File

	globalFiles, err := s.persister.ListFiles(ctx, nil)
	if err != nil {
		span.RecordError(err)
		return err
	}
	files = append(files, globalFiles...)

	if sess.SpaceId != nil {
		spaceFiles, err := s.persister.ListFiles(ctx, sess.SpaceId)
		if err != nil {
			span.RecordError(err)
			return err
		}
		files = append(files, spaceFiles...)
	}

	finalWindow := messageWindow
	if finalWindow < 1 {
		finalWindow = 10
	}

	maxIterations := 3

	for i := range maxIterations {
		span.AddEvent("extract_iteration_start", trace.WithAttributes(
			attribute.Int("iteration", i+1),
		))

		msgs, err := s.persister.ListMessages(
			ctx,
			sessionId,
			persister.WithSort(persister.SortOrderAsc),
		)
		if err != nil {
			span.RecordError(err)
			return err
		}

		tasks, err := s.persister.FetchCurrentTasks(ctx, sessionId, nil)
		if err != nil {
			span.RecordError(err)
			return err
		}

		actions, err := s.interpreter.Extract(ctx, msgs, finalWindow, files, tasks)
		if err != nil {
			span.RecordError(err)
			return err
		}

		if len(actions) == 0 {
			break
		}

		for _, action := range actions {
			if action.Type == interpreter.TaskActionFinish {
				span.AddEvent("action_finish")
				return nil
			}
			if err := s.executeTaskAction(ctx, sessionId, action, msgs); err != nil {
				span.RecordError(err)
				return err
			}
		}
	}

	return nil
}

func (s *V1Service) executeTaskAction(ctx context.Context, sessionId uuid.UUID, action interpreter.TaskAction, msgs []v1.Message) error {
	ctx, span := s.tracer.Start(ctx, "worker.executeTaskAction")
	defer span.End()

	span.SetAttributes(attribute.String("action.type", action.Type))

	p := action.Payload

	switch action.Type {
	case interpreter.TaskActionInsert:
		orderFloat, ok := p["after_task_order"].(float64)
		if !ok {
			return fmt.Errorf("invalid or missing 'after_task_order' in payload")
		}
		order := int(orderFloat)

		desc, ok := p["task_description"].(string)
		if !ok {
			return fmt.Errorf("invalid or missing 'task_description' in payload")
		}

		data, _ := json.Marshal(map[string]string{"task_description": desc})

		span.AddEvent("saving_task")

		if _, err := s.persister.InsertTask(ctx, sessionId, order, data, "pending"); err != nil {
			return err
		}

		return nil
	case interpreter.TaskActionUpdate:
		orderFloat, ok := p["task_order"].(float64)
		if !ok {
			return fmt.Errorf("invalid or missing 'task_order' in payload")
		}
		order := int(orderFloat)

		tasks, err := s.persister.FetchCurrentTasks(ctx, sessionId, nil)
		if err != nil {
			return err
		}

		var targetId uuid.UUID
		var currentData []byte
		found := false

		for _, t := range tasks {
			if t.TaskOrder == order {
				targetId = t.Id
				currentData = t.Data
				found = true
				break
			}
		}

		if !found {
			return fmt.Errorf("task order %d not found", order)
		}

		var status *string
		if s, ok := p["task_status"].(string); ok {
			status = &s
		}

		var bs []byte
		if desc, ok := p["task_description"].(string); ok {
			existing := map[string]any{}
			_ = json.Unmarshal(currentData, &existing)
			existing["task_description"] = desc
			bs, _ = json.Marshal(existing)
		}

		span.AddEvent("updating_task")

		if _, err := s.persister.UpdateTask(ctx, targetId, status, nil, bs); err != nil {
			return err
		}

		return nil
	case interpreter.TaskActionAppendTask:
		orderFloat, ok := p["task_order"].(float64)
		if !ok {
			return fmt.Errorf("invalid or missing 'task_order' in payload")
		}
		order := int(orderFloat)

		tasks, err := s.persister.FetchCurrentTasks(ctx, sessionId, nil)
		if err != nil {
			return err
		}

		var targetId uuid.UUID
		found := false

		for _, t := range tasks {
			if t.TaskOrder == order {
				targetId = t.Id
				found = true
				break
			}
		}

		if !found {
			return fmt.Errorf("task order %d not found", order)
		}

		indices, ok := p["message_ids"].([]any)
		if !ok {
			return fmt.Errorf("invalid or missing 'message_ids' in payload")
		}

		var ids []uuid.UUID
		for _, idx := range indices {
			if f, ok := idx.(float64); ok {
				i := int(f)
				if i >= 0 && i < len(msgs) {
					ids = append(ids, msgs[i].Id)
				}
			}
		}

		span.AddEvent("appending_messages_to_task")

		return s.persister.AppendMessagesToTask(ctx, targetId, ids)
	case interpreter.TaskActionAppendThought:
		indices, ok := p["message_ids"].([]any)
		if !ok {
			return fmt.Errorf("invalid or missing 'message_ids' in payload")
		}

		var ids []uuid.UUID
		for _, idx := range indices {
			if f, ok := idx.(float64); ok {
				i := int(f)
				if i >= 0 && i < len(msgs) {
					ids = append(ids, msgs[i].Id)
				}
			}
		}

		span.AddEvent("appending_messages_to_thought")

		return s.persister.AppendMessagesToThought(ctx, sessionId, ids)
	default:
		return nil
	}
}

func (s *V1Service) distill(ctx context.Context, sessionId uuid.UUID, messageWindow int) error {
	ctx, span := s.tracer.Start(ctx, "worker.distill")
	defer span.End()

	sess, err := s.persister.GetSession(ctx, sessionId)
	if err != nil {
		span.RecordError(err)
		return err
	}
	if sess == nil {
		return service.ErrSessionNotFound
	}

	if sess.SpaceId == nil {
		slog.InfoContext(ctx, "session has no space, skipping distillation", "session_id", sessionId)
		span.AddEvent("distill_skipped")
		return nil
	}

	finalWindow := messageWindow
	if finalWindow < 1 {
		finalWindow = 10
	}

	msgs, err := s.persister.ListMessages(
		ctx,
		sessionId,
		persister.WithSort(persister.SortOrderAsc),
	)
	if err != nil {
		span.RecordError(err)
		return err
	}

	skills, err := s.persister.FetchCurrentSkills(ctx, *sess.SpaceId)
	if err != nil {
		span.RecordError(err)
		return err
	}

	actions, err := s.interpreter.Distill(ctx, msgs, finalWindow, skills)
	if err != nil {
		span.RecordError(err)
		return err
	}

	if len(actions) == 0 {
		return nil
	}

	for _, action := range actions {
		if action.Type == interpreter.SkillActionFinish {
			span.AddEvent("action_finish")
			return nil
		}
		if err := s.executeSkillAction(ctx, sess, action); err != nil {
			span.RecordError(err)
			return err
		}
	}

	return nil
}

func (s *V1Service) executeSkillAction(ctx context.Context, sess *v1.Session, action interpreter.SkillAction) error {
	ctx, span := s.tracer.Start(ctx, "worker.executeSkillAction")
	defer span.End()

	span.SetAttributes(attribute.String("action.type", action.Type))

	p := action.Payload

	switch action.Type {
	case interpreter.SkillActionInsert:
		trigger, ok := p["trigger"].(string)
		if !ok {
			return fmt.Errorf("invalid or missing 'trigger' in payload")
		}

		sop, ok := p["sop"].(string)
		if !ok {
			return fmt.Errorf("invalid or missing 'sop' in payload")
		}

		vec, err := s.embedder.Embed(ctx, trigger)
		if err != nil {
			span.RecordError(err)
			return err
		}

		skill := &v1.Skill{
			Id:        uuid.New(),
			SpaceId:   *sess.SpaceId,
			Trigger:   trigger,
			SOP:       sop,
			Embedding: vec,
		}

		span.AddEvent("saving_skill")

		return s.persister.SaveSkill(ctx, skill)
	case interpreter.SkillActionUpdate:
		idStr, ok := p["skill_id"].(string)
		if !ok {
			return fmt.Errorf("missing 'skill_id' in payload")
		}
		id, err := uuid.Parse(idStr)
		if err != nil {
			return fmt.Errorf("invalid 'skill_id' in payload")
		}

		trigger, ok := p["trigger"].(string)
		if !ok {
			return fmt.Errorf("invalid or missing 'trigger' in payload")
		}

		sop, ok := p["sop"].(string)
		if !ok {
			return fmt.Errorf("invalid or missing 'sop' in payload")
		}

		vec, err := s.embedder.Embed(ctx, trigger)
		if err != nil {
			span.RecordError(err)
			return err

		}

		span.AddEvent("updating_skill")

		return s.persister.UpdateSkill(ctx, id, trigger, sop, vec)
	default:
		return nil
	}
}

func (s *V1Service) ingestFile(ctx context.Context, fileId uuid.UUID, spaceId *uuid.UUID) error {
	ctx, span := s.tracer.Start(ctx, "worker.ingestFile")
	defer span.End()

	file, err := s.persister.GetFile(ctx, fileId)
	if err != nil {
		span.RecordError(err)
		return err
	}
	if file == nil {
		return service.ErrFileNotFound
	}
	if file.Asset == nil {
		return service.ErrFileNotUploaded
	}

	rc, err := s.filer.Download(ctx, file.Asset.Path)
	if err != nil {
		return err
	}
	defer rc.Close()

	// TODO: use file chunking instead
	content, err := io.ReadAll(io.LimitReader(rc, maxFileBytes))
	if err != nil {
		span.RecordError(err)
		return err
	}

	text := string(content)
	if len(text) == 0 {
		return nil
	}

	document := fmt.Sprintf("Filename: %s\n\nContent:\n%s", file.Filename, text)

	vec, err := s.embedder.Embed(ctx, document)
	if err != nil {
		span.RecordError(err)
		return err
	}

	return s.persister.UpdateFileEmbedding(ctx, file.Id, vec)
}

func NewV1Service(
	p persister.V1Persister,
	d dispatcher.V1Dispatcher,
	f filer.V1Filer,
	i interpreter.V1Interpreter,
	e embedder.Embedder,
) *V1Service {
	s := service.New()
	return &V1Service{
		Service:     s,
		persister:   p,
		dispatcher:  d,
		filer:       f,
		interpreter: i,
		embedder:    e,
		tracer:      otel.Tracer("github.com/w-h-a/gomento/internal/service/v1_worker"),
	}
}
