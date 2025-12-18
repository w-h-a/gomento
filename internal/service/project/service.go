package project

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

func (s *V1Service) Create(ctx context.Context, name string) (*v1.Project, error) {
	p := &v1.Project{
		Id:   uuid.New(),
		Name: name,
	}
	if err := s.persister.CreateProject(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

func NewV1Service(
	p persister.V1Persister,
	opts ...service.Option,
) *V1Service {
	s := service.New(opts...)
	return &V1Service{
		Service:   s,
		persister: p,
	}
}
