package v1mock

import (
	"context"
	"maps"
	"mime/multipart"
	"sync"

	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/uploader"
)

type v1MockUploader struct {
	options uploader.Options
	uploads map[string]int64
	mtx     sync.RWMutex
}

func (u *v1MockUploader) Upload(ctx context.Context, fh *multipart.FileHeader) (*v1.Asset, error) {
	u.mtx.Lock()
	defer u.mtx.Unlock()
	path := "uploads/" + fh.Filename
	u.uploads[path] = fh.Size
	contentType := fh.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return &v1.Asset{
		Container: u.options.Container,
		Path:      path,
		MIME:      contentType,
		SizeBytes: fh.Size,
	}, nil
}

func (u *v1MockUploader) Uploads() map[string]int64 {
	u.mtx.RLock()
	defer u.mtx.RUnlock()
	cpy := make(map[string]int64, len(u.uploads))
	maps.Copy(cpy, u.uploads)
	return cpy
}

func NewV1Uploader(opts ...uploader.Option) *v1MockUploader {
	options := uploader.NewOptions(opts...)

	u := &v1MockUploader{
		options: options,
		uploads: map[string]int64{},
		mtx:     sync.RWMutex{},
	}

	return u
}
