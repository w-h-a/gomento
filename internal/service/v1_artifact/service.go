package v1artifact

import (
	"context"

	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/filer"
	"github.com/w-h-a/gomento/internal/client/persister"
	"github.com/w-h-a/gomento/internal/service"
)

type V1Service struct {
	*service.Service
	persister persister.V1Persister
	filer     filer.V1Filer
}

func (s *V1Service) Create(ctx context.Context, projectId uuid.UUID) (*v1.Artifact, error) {
	a := &v1.Artifact{
		Id:        uuid.New(),
		ProjectId: projectId,
	}
	if err := s.persister.CreateArtifact(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *V1Service) UploadFile(ctx context.Context, in CreateFileInput) (*v1.File, error) {
	asset, err := s.filer.UploadReader(ctx, in.Reader, in.Filename, in.MimeType, in.Size)
	if err != nil {
		return nil, err
	}

	asset.Id = uuid.New()

	file := &v1.File{
		Id:         uuid.New(),
		ArtifactId: in.ArtifactId,
		Path:       in.Path,
		Filename:   in.Filename,
		Meta:       []byte("{}"),
	}

	if err := s.persister.UpsertFileWithAsset(ctx, file, asset); err != nil {
		return nil, err
	}

	file.Asset = asset

	return file, nil
}

func (s *V1Service) ListFiles(ctx context.Context, artifactId uuid.UUID) ([]v1.File, error) {
	return s.persister.ListFiles(ctx, artifactId)
}

func NewV1Service(p persister.V1Persister, f filer.V1Filer) *V1Service {
	s := service.New()
	return &V1Service{
		Service:   s,
		persister: p,
		filer:     f,
	}
}
