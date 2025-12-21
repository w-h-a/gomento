package v1worker

import (
	"context"
	"fmt"
	"log/slog"

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
	if task.Type != v1.TaskTypeDistill {
		return fmt.Errorf("unknown task type")
	}

	msgs, err := s.persister.GetMessages(ctx, task.Payload.SessionId)
	if err != nil {
		return err
	}

	sess, err := s.persister.GetSession(ctx, task.Payload.SessionId)
	if err != nil {
		return err
	}

	skill, err := s.distiller.Distill(ctx, msgs)

	skill.SpaceId = sess.SpaceId

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
