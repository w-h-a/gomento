package v1mock

import (
	"context"
	"fmt"
	"maps"
	"mime/multipart"
	"sync"
	"time"

	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/filer"
)

type v1MockFiler struct {
	options filer.Options
	uploads map[string]int64
	mtx     sync.RWMutex
}

func (f *v1MockFiler) Upload(ctx context.Context, fh *multipart.FileHeader) (*v1.Asset, error) {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	path := "uploads/" + fh.Filename
	f.uploads[path] = fh.Size
	contentType := fh.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return &v1.Asset{
		Container: f.options.Container,
		Path:      path,
		MIME:      contentType,
		SizeBytes: fh.Size,
	}, nil
}

func (f *v1MockFiler) Uploads() map[string]int64 {
	f.mtx.RLock()
	defer f.mtx.RUnlock()
	cpy := make(map[string]int64, len(f.uploads))
	maps.Copy(cpy, f.uploads)
	return cpy
}

func (f *v1MockFiler) PresignGet(ctx context.Context, path string, expire time.Duration) (string, error) {
	return fmt.Sprintf("https://mock/%s?expire=%d", path, expire), nil
}

func NewV1Filer(opts ...filer.Option) *v1MockFiler {
	options := filer.NewOptions(opts...)

	f := &v1MockFiler{
		options: options,
		uploads: map[string]int64{},
		mtx:     sync.RWMutex{},
	}

	return f
}
