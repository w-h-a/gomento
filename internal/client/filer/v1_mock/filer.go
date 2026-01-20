package v1mock

import (
	"context"
	"fmt"
	"io"
	"maps"
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

func (f *v1MockFiler) Upload(ctx context.Context, r io.ReadSeeker, filename, contentType string, size int64) (*v1.Asset, error) {
	f.mtx.Lock()
	defer f.mtx.Unlock()

	path := "uploads/" + filename

	f.uploads[path] = size

	if contentType == "" {
		contentType = "application/octet-stream"
	}

	return &v1.Asset{
		Container: f.options.Container,
		Path:      path,
		MIME:      contentType,
		SizeBytes: size,
	}, nil
}

func (f *v1MockFiler) Uploads() map[string]int64 {
	f.mtx.RLock()
	defer f.mtx.RUnlock()
	cpy := make(map[string]int64, len(f.uploads))
	maps.Copy(cpy, f.uploads)
	return cpy
}

func (f *v1MockFiler) Download(ctx context.Context, path string) (io.ReadCloser, error) {
	f.mtx.RLock()
	defer f.mtx.RUnlock()

	if _, ok := f.uploads[path]; !ok {
		return nil, fmt.Errorf("mock file not found: %s", path)
	}

	r, w := io.Pipe()
	w.Close()

	return r, nil
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
