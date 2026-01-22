package v1file

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/dispatcher"
	"github.com/w-h-a/gomento/internal/client/embedder"
	"github.com/w-h-a/gomento/internal/client/filer"
	"github.com/w-h-a/gomento/internal/client/persister"
	"github.com/w-h-a/gomento/internal/service"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var (
	assetPublicUrlExpire = 24 * time.Hour
)

type V1Service struct {
	*service.Service
	persister  persister.V1Persister
	dispatcher dispatcher.V1Dispatcher
	filer      filer.V1Filer
	embedder   embedder.Embedder
	tracer     trace.Tracer
	qname      string
}

func (s *V1Service) Upload(ctx context.Context, in CreateFileInput) (*v1.File, error) {
	ctx, span := s.tracer.Start(ctx, "file.Upload")
	defer span.End()

	span.SetAttributes(
		attribute.String("file.filename", in.Filename),
		attribute.String("file.mime_type", in.MimeType),
		attribute.Int64("file.size_bytes", in.Size),
		attribute.String("file.path", in.Path),
	)

	if in.SpaceId != nil {
		span.SetAttributes(attribute.String("space.id", in.SpaceId.String()))
	}

	asset, err := s.filer.Upload(ctx, in.Reader, in.Filename, in.MimeType, in.Size)
	if err != nil {
		span.RecordError(err)
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
		span.RecordError(err)
		return nil, err
	}

	file.Asset = asset

	span.SetAttributes(attribute.String("file.id", file.Id.String()))

	if err := s.dispatchJob(ctx, file.Id, file.SpaceId); err != nil {
		span.RecordError(err)
		return nil, err
	}

	return file, nil
}

func (s *V1Service) dispatchJob(ctx context.Context, fileId uuid.UUID, spaceId *uuid.UUID) error {
	ctx, span := s.tracer.Start(ctx, "file.dispatchJob")
	defer span.End()

	payload := v1.IngestFileJobPayload{
		FileId:  fileId,
		SpaceId: spaceId,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	job := &v1.Job{
		Id:      uuid.New(),
		Type:    v1.JobTypeIngestFile,
		Payload: data,
		Status:  v1.JobStatusPending,
	}

	span.SetAttributes(
		attribute.String("file.id", fileId.String()),
		attribute.String("job.id", job.Id.String()),
		attribute.String("job.type", v1.JobTypeIngestFile),
	)

	if spaceId != nil {
		span.SetAttributes(attribute.String("space.id", spaceId.String()))
	}

	if err := s.persister.CreateJob(ctx, job); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to persist job: %w", err)
	}

	if err := s.dispatcher.Publish(ctx, job, dispatcher.PublishWithQueue(s.qname)); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to publish job: %w", err)
	}

	return nil
}

func (s *V1Service) Download(ctx context.Context, in ReadFileInput) (io.ReadCloser, error) {
	ctx, span := s.tracer.Start(ctx, "file.Download")

	span.SetAttributes(
		attribute.String("file.id", in.FileId.String()),
		attribute.Int("read.start_line", in.StartLine),
		attribute.Int("read.end_line", in.EndLine),
	)

	file, err := s.persister.GetFile(ctx, in.FileId)
	if err != nil {
		span.RecordError(err)
		span.End()
		return nil, err
	}
	if file == nil {
		span.End()
		return nil, service.ErrFileNotFound
	}
	if file.Asset == nil {
		err := service.ErrFileNotUploaded
		span.RecordError(err)
		span.End()
		return nil, err
	}

	rc, err := s.filer.Download(ctx, file.Asset.Path)
	if err != nil {
		span.RecordError(err)
		span.End()
		return nil, err
	}

	pipeReader, pipeWriter := io.Pipe()

	go func() {
		var err error
		defer func() {
			rc.Close()
			pipeWriter.CloseWithError(err)
			span.End()
		}()

		scanner := bufio.NewScanner(rc)
		lineNum := 1

		start := max(in.StartLine, 1)

		for scanner.Scan() {
			if in.EndLine > 0 && lineNum > in.EndLine {
				break
			}

			if lineNum >= start {
				if _, writeErr := pipeWriter.Write(scanner.Bytes()); writeErr != nil {
					err = writeErr
					return
				}

				if _, writeErr := pipeWriter.Write([]byte{'\n'}); writeErr != nil {
					err = writeErr
					return
				}
			}

			lineNum++
		}

		err = scanner.Err()
	}()

	return pipeReader, nil
}

func (s *V1Service) List(ctx context.Context, in ListFilesInput) (*ListFilesOutput, error) {
	ctx, span := s.tracer.Start(ctx, "file.List")
	defer span.End()

	if in.SpaceId != nil {
		span.SetAttributes(attribute.String("space.id", in.SpaceId.String()))
	}
	if len(in.PathPrefix) > 0 {
		span.SetAttributes(attribute.String("file.path_prefix", in.PathPrefix))
	}

	fs, err := s.persister.ListFiles(
		ctx,
		in.SpaceId,
		persister.WithPathPrefix(in.PathPrefix),
	)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	span.SetAttributes(attribute.Int("result.count", len(fs)))

	return &ListFilesOutput{
		Items: fs,
	}, nil
}

func (s *V1Service) Get(ctx context.Context, id uuid.UUID, withUrl bool) (*v1.File, string, error) {
	ctx, span := s.tracer.Start(ctx, "file.Get")
	defer span.End()

	span.SetAttributes(
		attribute.String("file.id", id.String()),
		attribute.Bool("with_signed_url", withUrl),
	)

	file, err := s.persister.GetFile(ctx, id)
	if err != nil {
		span.RecordError(err)
		return nil, "", err
	}
	if file == nil {
		return nil, "", service.ErrFileNotFound
	}

	var url string
	if withUrl && file.Asset != nil {
		url, err = s.filer.PresignGet(ctx, file.Asset.Path, assetPublicUrlExpire)
		if err != nil {
			span.RecordError(err)
			return nil, "", err
		}
	}

	return file, url, nil
}

func (s *V1Service) ConnectToSpace(ctx context.Context, fileId uuid.UUID, spaceId uuid.UUID) error {
	ctx, span := s.tracer.Start(ctx, "file.ConnectToSpace")
	defer span.End()

	span.SetAttributes(
		attribute.String("file.id", fileId.String()),
		attribute.String("space.id", spaceId.String()),
	)

	file, err := s.persister.GetFile(ctx, fileId)
	if err != nil {
		span.RecordError(err)
		return err
	}
	if file == nil {
		return service.ErrFileNotFound
	}

	space, err := s.persister.GetSpace(ctx, spaceId)
	if err != nil {
		span.RecordError(err)
		return err
	}
	if space == nil {
		return service.ErrSpaceNotFound
	}

	file.SpaceId = &spaceId

	return s.persister.UpdateFileSpace(ctx, file)
}

func NewV1Service(
	p persister.V1Persister,
	d dispatcher.V1Dispatcher,
	f filer.V1Filer,
	e embedder.Embedder,
	qname string,
) *V1Service {
	s := service.New()
	return &V1Service{
		Service:    s,
		persister:  p,
		dispatcher: d,
		filer:      f,
		embedder:   e,
		tracer:     otel.Tracer("github.com/w-h-a/gomento/internal/service/v1_file"),
		qname:      qname,
	}
}
