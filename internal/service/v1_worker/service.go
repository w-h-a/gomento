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

	if job.Type != v1.JobTypeDistill {
		s.persister.UpdateJobStatus(ctx, job.Id, v1.JobStatusFailed)
		return fmt.Errorf("unknown job type: %s", job.Type)
	}

	var payload v1.DistillJobPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		s.persister.UpdateJobStatus(ctx, job.Id, v1.JobStatusFailed)
		return fmt.Errorf("invalid job payload: %w", err)
	}

	if err := s.processDistill(ctx, payload.SessionId); err != nil {
		s.persister.UpdateJobStatus(ctx, job.Id, v1.JobStatusFailed)
		return err
	}

	slog.InfoContext(ctx, "job success", "job_id", job.Id)
	return s.persister.UpdateJobStatus(ctx, job.Id, v1.JobStatusSuccess)
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

	msgs, err := s.persister.GetMessages(
		ctx,
		sessionId,
		persister.WithSort(persister.SortOrderAsc),
	)
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
