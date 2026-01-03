package v1space

import (
	"context"

	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/persister"
	"github.com/w-h-a/gomento/internal/service"
)

type V1Service struct {
	*service.Service
	persister persister.V1Persister
}

func (s *V1Service) Create(ctx context.Context, projectId uuid.UUID, name string) (*v1.Space, error) {
	p := &v1.Space{
		Id:        uuid.New(),
		ProjectId: projectId,
		Name:      name,
	}
	if err := s.persister.CreateSpace(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *V1Service) ListTasks(ctx context.Context, in ListTasksInput) (*ListTasksOutput, error) {
	space, err := s.persister.GetSpace(ctx, in.SpaceId)
	if err != nil {
		return nil, err
	}

	if space == nil {
		return nil, service.ErrSpaceNotFound
	}

	tasks, err := s.persister.ListTasksBySpace(ctx, in.SpaceId, in.Status)
	if err != nil {
		return nil, err
	}

	return &ListTasksOutput{
		Items: tasks,
	}, nil
}

func NewV1Service(
	p persister.V1Persister,
) *V1Service {
	s := service.New()
	return &V1Service{
		Service:   s,
		persister: p,
	}
}
