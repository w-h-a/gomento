package v1file

import (
	"context"
	"time"

	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/filer"
	"github.com/w-h-a/gomento/internal/client/persister"
	"github.com/w-h-a/gomento/internal/service"
)

var (
	assetPublicUrlExpire = 24 * time.Hour
)

type V1Service struct {
	*service.Service
	persister persister.V1Persister
	filer     filer.V1Filer
}

func (s *V1Service) Upload(ctx context.Context, in CreateFileInput) (*v1.File, error) {
	asset, err := s.filer.UploadReader(ctx, in.Reader, in.Filename, in.MimeType, in.Size)
	if err != nil {
		return nil, err
	}

	asset.Id = uuid.New()

	file := &v1.File{
		Id:       uuid.New(),
		SpaceId:  in.SpaceId,
		Path:     in.Path,
		Filename: in.Filename,
		Meta:     []byte("{}"),
	}

	if err := s.persister.UpsertFileWithAsset(ctx, file, asset); err != nil {
		return nil, err
	}

	file.Asset = asset

	return file, nil
}

func (s *V1Service) List(ctx context.Context, in ListFilesInput) (*ListFilesOutput, error) {
	fs, err := s.persister.ListFiles(
		ctx,
		in.SpaceId,
		persister.WithPathPrefix(in.PathPrefix),
	)
	if err != nil {
		return nil, err
	}

	return &ListFilesOutput{
		Items: fs,
	}, nil
}

func (s *V1Service) Get(ctx context.Context, id uuid.UUID, withUrl bool) (*v1.File, string, error) {
	file, err := s.persister.GetFile(ctx, id)
	if err != nil {
		return nil, "", err
	}
	if file == nil {
		return nil, "", service.ErrFileNotFound
	}

	var url string
	if withUrl && file.Asset != nil {
		url, err = s.filer.PresignGet(ctx, file.Asset.Path, assetPublicUrlExpire)
		if err != nil {
			return nil, "", err
		}
	}

	return file, url, nil
}

func (s *V1Service) ConnectToSpace(ctx context.Context, fileId uuid.UUID, spaceId uuid.UUID) error {
	file, err := s.persister.GetFile(ctx, fileId)
	if err != nil {
		return err
	}
	if file == nil {
		return service.ErrFileNotFound
	}

	space, err := s.persister.GetSpace(ctx, spaceId)
	if err != nil {
		return err
	}
	if space == nil {
		return service.ErrSpaceNotFound
	}

	file.SpaceId = &spaceId

	return s.persister.UpdateFileSpace(ctx, file)
}

func NewV1Service(p persister.V1Persister, f filer.V1Filer) *V1Service {
	s := service.New()
	return &V1Service{
		Service:   s,
		persister: p,
		filer:     f,
	}
}
