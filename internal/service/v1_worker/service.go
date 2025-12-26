package v1worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/dispatcher"
	"github.com/w-h-a/gomento/internal/client/distiller"
	"github.com/w-h-a/gomento/internal/client/persister"
	"github.com/w-h-a/gomento/internal/service"
)

type V1Service struct {
	*service.Service
	dispatcher dispatcher.V1Dispatcher
	persister  persister.V1Persister
	distiller  distiller.V1Distiller
}

func (s *V1Service) Subscribe(ctx context.Context, cb func(context.Context, *v1.Task) error, qname string) error {
	return s.dispatcher.Subscribe(ctx, cb, dispatcher.SubscribeWithQueue(qname))
}

func (s *V1Service) Close(ctx context.Context) error {
	return s.dispatcher.Close(ctx)
}

func (s *V1Service) ProcessTask(ctx context.Context, task *v1.Task) error {
	s.persister.UpdateTaskStatus(ctx, task.Id, v1.TaskStatusRunning)

	var payload v1.TaskPayload
	if err := json.Unmarshal(task.Data, &payload); err != nil {
		s.persister.UpdateTaskStatus(ctx, task.Id, v1.TaskStatusFailed)
		return fmt.Errorf("invalid task data: %w", err)
	}

	if payload.Type != v1.TaskTypeDistill {
		s.persister.UpdateTaskStatus(ctx, task.Id, v1.TaskStatusFailed)
		return fmt.Errorf("unknown task type: %s", payload.Type)
	}

	if err := s.processDistill(ctx, payload.SessionId); err != nil {
		s.persister.UpdateTaskStatus(ctx, task.Id, v1.TaskStatusFailed)
		return err
	}

	slog.InfoContext(ctx, "task success", "task_id", task.Id)

	return s.persister.UpdateTaskStatus(ctx, task.Id, v1.TaskStatusSuccess)
}

func (s *V1Service) processDistill(ctx context.Context, sessionId uuid.UUID) error {
	sess, err := s.persister.GetSession(ctx, sessionId)
	if err != nil {
		return err
	}

	if sess.SpaceId == nil {
		slog.InfoContext(ctx, "session has no space, skipping distillation", "session_id", sessionId)
		return nil
	}

	msgs, err := s.persister.GetMessages(ctx, sessionId)
	if err != nil {
		return err
	}

	skill, err := s.distiller.Distill(ctx, msgs)
	if err != nil {
		return err
	}

	skill.SpaceId = *sess.SpaceId

	slog.InfoContext(ctx, "saving skill", "trigger", skill.Trigger)

	return s.persister.SaveSkill(ctx, skill)
}

func NewV1Service(
	p persister.V1Persister,
	disp dispatcher.V1Dispatcher,
	dist distiller.V1Distiller,
) *V1Service {
	s := service.New()
	return &V1Service{
		Service:    s,
		persister:  p,
		dispatcher: disp,
		distiller:  dist,
	}
}
