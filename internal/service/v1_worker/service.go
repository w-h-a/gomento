package v1worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/dispatcher"
	"github.com/w-h-a/gomento/internal/client/interpreter"
	"github.com/w-h-a/gomento/internal/client/persister"
	"github.com/w-h-a/gomento/internal/service"
)

type V1Service struct {
	*service.Service
	dispatcher  dispatcher.V1Dispatcher
	persister   persister.V1Persister
	interpreter interpreter.V1Interpreter
}

func (s *V1Service) Subscribe(ctx context.Context, cb func(context.Context, *v1.Job) error, qname string) error {
	return s.dispatcher.Subscribe(ctx, cb, dispatcher.SubscribeWithQueue(qname))
}

func (s *V1Service) Close(ctx context.Context) error {
	return s.dispatcher.Close(ctx)
}

func (s *V1Service) ProcessJob(ctx context.Context, job *v1.Job) error {
	if err := s.persister.AcquireJobLock(ctx, job.Id); err != nil {
		if errors.Is(err, persister.ErrJobLocked) {
			slog.WarnContext(ctx, "job locked", "job_id", job.Id)
			return nil
		}
		s.persister.UpdateJobStatus(ctx, job.Id, v1.JobStatusFailed)
		return fmt.Errorf("failed to acquire job lock: %w", err)
	}

	if job.Type != v1.JobTypeDistill && job.Type != v1.JobTypeExtract {
		s.persister.UpdateJobStatus(ctx, job.Id, v1.JobStatusFailed)
		return fmt.Errorf("unknown job type: %s", job.Type)
	}

	var payload v1.JobPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		s.persister.UpdateJobStatus(ctx, job.Id, v1.JobStatusFailed)
		return fmt.Errorf("invalid job payload: %w", err)
	}

	if err := s.extract(ctx, payload.SessionId); err != nil {
		if job.Type == v1.JobTypeExtract {
			s.persister.UpdateJobStatus(ctx, job.Id, v1.JobStatusFailed)
			return err
		}
	}

	if job.Type == v1.JobTypeDistill {
		if err := s.distill(ctx, payload.SessionId); err != nil {
			s.persister.UpdateJobStatus(ctx, job.Id, v1.JobStatusFailed)
			return err
		}
	}

	slog.InfoContext(ctx, "job success", "job_id", job.Id)
	return s.persister.UpdateJobStatus(ctx, job.Id, v1.JobStatusSuccess)
}

func (s *V1Service) extract(ctx context.Context, sessionId uuid.UUID) error {
	maxIterations := 3

	sess, err := s.persister.GetSession(ctx, sessionId)
	if err != nil {
		return err
	}

	if sess == nil {
		return fmt.Errorf("session %v not found", sessionId)
	}

	var files []v1.File

	artifacts, err := s.persister.ListArtifacts(ctx, sess.ProjectId)
	if err != nil {
		return err
	}

	for _, art := range artifacts {
		fs, err := s.persister.ListFiles(ctx, art.Id)
		if err != nil {
			return err
		}
		files = append(files, fs...)
	}

	for range maxIterations {
		msgs, err := s.persister.ListMessages(ctx, sessionId, persister.WithSort(persister.SortOrderAsc))
		if err != nil {
			return err
		}

		tasks, err := s.persister.FetchCurrentTasks(ctx, sessionId, nil)
		if err != nil {
			return err
		}

		actions, err := s.interpreter.Extract(ctx, msgs, files, tasks)
		if err != nil {
			return err
		}

		if len(actions) == 0 {
			break
		}

		for _, action := range actions {
			if action.Type == interpreter.TaskActionFinish {
				return nil
			}
			if err := s.executeAction(ctx, sessionId, action, msgs); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *V1Service) executeAction(ctx context.Context, sessionId uuid.UUID, action interpreter.TaskAction, msgs []v1.Message) error {
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

		return s.persister.AppendMessagesToThought(ctx, sessionId, ids)
	default:
		return nil
	}
}

func (s *V1Service) distill(ctx context.Context, sessionId uuid.UUID) error {
	sess, err := s.persister.GetSession(ctx, sessionId)
	if err != nil {
		return err
	}

	if sess.SpaceId == nil {
		slog.InfoContext(ctx, "session has no space, skipping distillation", "session_id", sessionId)
		return nil
	}

	msgs, err := s.persister.ListMessages(
		ctx,
		sessionId,
		persister.WithSort(persister.SortOrderAsc),
	)
	if err != nil {
		return err
	}

	skill, err := s.interpreter.Distill(ctx, msgs)
	if err != nil {
		return err
	}

	skill.SpaceId = *sess.SpaceId

	slog.InfoContext(ctx, "saving skill", "trigger", skill.Trigger)

	return s.persister.SaveSkill(ctx, skill)
}

func NewV1Service(
	p persister.V1Persister,
	d dispatcher.V1Dispatcher,
	i interpreter.V1Interpreter,
) *V1Service {
	s := service.New()
	return &V1Service{
		Service:     s,
		persister:   p,
		dispatcher:  d,
		interpreter: i,
	}
}
