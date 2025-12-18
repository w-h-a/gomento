package worker

import (
	"context"
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
	persister  persister.V1Persister
	dispatcher dispatcher.V1Dispatcher
	distiller  distiller.V1Distiller
}

func (s *V1Service) ProcessTask(ctx context.Context, task *v1.Task) error {
	if task.Type != v1.TaskTypeDistill {
		return fmt.Errorf("unknown task type")
	}

	sessionId := uuid.MustParse(task.Payload["session_id"].(string))

	msgs, err := s.persister.GetMessages(ctx, sessionId)
	if err != nil {
		return err
	}

	sess, err := s.persister.GetSession(ctx, sessionId)
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
	opts ...service.Option,
) *V1Service {
	s := service.New(opts...)
	return &V1Service{
		Service:    s,
		persister:  p,
		dispatcher: disp,
		distiller:  dist,
	}
}
