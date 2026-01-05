package v1space

import (
	"context"

	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/embedder"
	"github.com/w-h-a/gomento/internal/client/persister"
	"github.com/w-h-a/gomento/internal/service"
)

type V1Service struct {
	*service.Service
	persister persister.V1Persister
	embedder  embedder.Embedder
}

func (s *V1Service) Create(ctx context.Context, name string) (*v1.Space, error) {
	p := &v1.Space{
		Id:   uuid.New(),
		Name: name,
	}
	if err := s.persister.CreateSpace(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *V1Service) ListSpaces(ctx context.Context) (*ListSpacesOutput, error) {
	spaces, err := s.persister.ListSpaces(ctx)
	if err != nil {
		return nil, err
	}

	return &ListSpacesOutput{
		Items: spaces,
	}, nil
}

func (s *V1Service) GetSpace(ctx context.Context, id uuid.UUID) (*v1.Space, error) {
	space, err := s.persister.GetSpace(ctx, id)
	if err != nil {
		return nil, err
	}
	if space == nil {
		return nil, service.ErrSpaceNotFound
	}
	return space, nil
}

func (s *V1Service) SearchSkills(ctx context.Context, spaceId uuid.UUID, query string) ([]v1.Skill, error) {
	vec, err := s.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}
	return s.persister.SearchSkills(ctx, spaceId, vec, persister.SearchWithLimit(5))
}

func (s *V1Service) SearchMessages(ctx context.Context, spaceId uuid.UUID, query string) ([]v1.Message, error) {
	vec, err := s.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}
	return s.persister.SearchMessages(ctx, spaceId, vec, persister.SearchWithLimit(10))
}

func NewV1Service(
	p persister.V1Persister,
	e embedder.Embedder,
) *V1Service {
	s := service.New()
	return &V1Service{
		Service:   s,
		persister: p,
		embedder:  e,
	}
}
