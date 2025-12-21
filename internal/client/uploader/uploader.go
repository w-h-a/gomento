package uploader

import (
	"context"
	"mime/multipart"

	v1 "github.com/w-h-a/gomento/api/domain/v1"
)

type V1Uploader interface {
	Upload(ctx context.Context, fh *multipart.FileHeader) (*v1.Asset, error)
}
