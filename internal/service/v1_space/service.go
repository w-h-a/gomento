package v1space

import (
	"context"

	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/embedder"
	"github.com/w-h-a/gomento/internal/client/persister"
	"github.com/w-h-a/gomento/internal/service"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type V1Service struct {
	*service.Service
	persister persister.V1Persister
	embedder  embedder.Embedder
	tracer    trace.Tracer
}

func (s *V1Service) Create(ctx context.Context, name string) (*v1.Space, error) {
	ctx, span := s.tracer.Start(ctx, "space.Create")
	defer span.End()

	p := &v1.Space{
		Id:   uuid.New(),
		Name: name,
	}

	span.SetAttributes(
		attribute.String("space.name", name),
		attribute.String("space.id", p.Id.String()),
	)

	if err := s.persister.CreateSpace(ctx, p); err != nil {
		span.RecordError(err)
		return nil, err
	}

	return p, nil
}

func (s *V1Service) List(ctx context.Context) (*ListSpacesOutput, error) {
	ctx, span := s.tracer.Start(ctx, "space.List")
	defer span.End()

	spaces, err := s.persister.ListSpaces(ctx)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	span.SetAttributes(attribute.Int("result.count", len(spaces)))

	return &ListSpacesOutput{
		Items: spaces,
	}, nil
}

func (s *V1Service) Get(ctx context.Context, id uuid.UUID) (*v1.Space, error) {
	ctx, span := s.tracer.Start(ctx, "space.Get")
	defer span.End()

	space, err := s.persister.GetSpace(ctx, id)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}
	if space == nil {
		return nil, service.ErrSpaceNotFound
	}

	return space, nil
}

func (s *V1Service) SearchSkills(ctx context.Context, spaceId uuid.UUID, query string) ([]v1.Skill, error) {
	ctx, span := s.tracer.Start(ctx, "space.SearchSkills")
	defer span.End()

	span.SetAttributes(
		attribute.String("space.id", spaceId.String()),
		attribute.String("search.query", query),
	)

	vec, err := s.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	skills, err := s.persister.SearchSkills(ctx, spaceId, vec, persister.SearchWithLimit(5))
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	span.SetAttributes(attribute.Int("result.count", len(skills)))

	return skills, nil
}

func (s *V1Service) SearchMessages(ctx context.Context, spaceId uuid.UUID, query string) ([]v1.Message, error) {
	ctx, span := s.tracer.Start(ctx, "space.SearchMessages")
	defer span.End()

	span.SetAttributes(
		attribute.String("space.id", spaceId.String()),
		attribute.String("search.query", query),
	)

	vec, err := s.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	messages, err := s.persister.SearchMessages(ctx, spaceId, vec, persister.SearchWithLimit(10))
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	span.SetAttributes(attribute.Int("result.count", len(messages)))

	return messages, nil
}

func (s *V1Service) SearchFiles(ctx context.Context, spaceId uuid.UUID, query string) ([]v1.File, error) {
	ctx, span := s.tracer.Start(ctx, "space.SearchFiles")
	defer span.End()

	span.SetAttributes(
		attribute.String("space.id", spaceId.String()),
		attribute.String("search.query", query),
	)

	vec, err := s.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	files, err := s.persister.SearchFiles(ctx, spaceId, vec, persister.SearchWithLimit(10))
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	span.SetAttributes(attribute.Int("result.count", len(files)))

	return files, nil
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
		tracer:    otel.Tracer("github.com/w-h-a/gomento/internal/service/v1_space"),
	}
}
