package filer

import (
	"context"
	"io"
	"mime/multipart"
	"time"

	v1 "github.com/w-h-a/gomento/api/domain/v1"
)

type V1Filer interface {
	UploadMultipart(ctx context.Context, fh *multipart.FileHeader) (*v1.Asset, error)
	UploadReader(ctx context.Context, r io.ReadSeeker, filename, contentType string, size int64) (*v1.Asset, error)
	PresignGet(ctx context.Context, path string, expire time.Duration) (string, error)
}
