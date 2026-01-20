package v1mock

import (
	"bytes"
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
	uploads map[string][]byte
	mtx     sync.RWMutex
}

func (f *v1MockFiler) Upload(ctx context.Context, r io.ReadSeeker, filename, contentType string, size int64) (*v1.Asset, error) {
	f.mtx.Lock()
	defer f.mtx.Unlock()

	path := "uploads/" + filename

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	f.uploads[path] = data

	if contentType == "" {
		contentType = "application/octet-stream"
	}

	return &v1.Asset{
		Container: f.options.Container,
		Path:      path,
		MIME:      contentType,
		SizeBytes: int64(len(data)),
	}, nil
}

func (f *v1MockFiler) Uploads() map[string][]byte {
	f.mtx.RLock()
	defer f.mtx.RUnlock()
	cpy := make(map[string][]byte, len(f.uploads))
	maps.Copy(cpy, f.uploads)
	return cpy
}

func (f *v1MockFiler) Download(ctx context.Context, path string) (io.ReadCloser, error) {
	f.mtx.RLock()
	defer f.mtx.RUnlock()

	data, ok := f.uploads[path]
	if !ok {
		return nil, fmt.Errorf("mock file not found: %s", path)
	}

	return io.NopCloser(bytes.NewReader(data)), nil
}

func (f *v1MockFiler) PresignGet(ctx context.Context, path string, expire time.Duration) (string, error) {
	return fmt.Sprintf("https://mock/%s?expire=%d", path, expire), nil
}

func NewV1Filer(opts ...filer.Option) *v1MockFiler {
	options := filer.NewOptions(opts...)

	f := &v1MockFiler{
		options: options,
		uploads: map[string][]byte{},
		mtx:     sync.RWMutex{},
	}

	return f
}
