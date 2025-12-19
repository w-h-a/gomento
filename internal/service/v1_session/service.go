package v1session

import (
	"context"
	"time"

	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/dispatcher"
	"github.com/w-h-a/gomento/internal/client/persister"
	"github.com/w-h-a/gomento/internal/service"
)

type V1Service struct {
	*service.Service
	persister  persister.V1Persister
	dispatcher dispatcher.V1Dispatcher
	qname      string
}

func (s *V1Service) Create(ctx context.Context, projectId uuid.UUID, spaceId uuid.UUID) (*v1.Session, error) {
	p := &v1.Session{
		Id:        uuid.New(),
		ProjectId: projectId,
		SpaceId:   spaceId,
	}
	if err := s.persister.CreateSession(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *V1Service) AddMessage(ctx context.Context, sessionId uuid.UUID, role string, content string) error {
	msg := &v1.Message{
		Id:        uuid.New(),
		SessionId: sessionId,
		Role:      role,
		Content:   content,
	}
	return s.persister.AddMessage(ctx, msg)
}

func (s *V1Service) FinishSession(ctx context.Context, sessionId uuid.UUID) error {
	payload := map[string]any{
		"session_id": sessionId.String(),
	}
	task := &v1.Task{
		Id:        uuid.New(),
		Type:      v1.TaskTypeDistill,
		Payload:   payload,
		CreatedAt: time.Now(),
	}
	return s.dispatcher.Publish(ctx, task, dispatcher.PublishWithQueue(s.qname))
}

func NewV1Service(
	p persister.V1Persister,
	d dispatcher.V1Dispatcher,
	qname string,
) *V1Service {
	s := service.New()
	return &V1Service{
		Service:    s,
		persister:  p,
		dispatcher: d,
		qname:      qname,
	}
}
