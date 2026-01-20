package filer

import (
	"context"
	"io"
	"time"

	v1 "github.com/w-h-a/gomento/api/domain/v1"
)

type V1Filer interface {
	Upload(ctx context.Context, r io.ReadSeeker, filename, contentType string, size int64) (*v1.Asset, error)
	Download(ctx context.Context, path string) (io.ReadCloser, error)
	PresignGet(ctx context.Context, path string, expire time.Duration) (string, error)
}
