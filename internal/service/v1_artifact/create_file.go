package v1artifact

import (
	"io"

	"github.com/google/uuid"
)

type CreateFileInput struct {
	ArtifactId uuid.UUID
	Path       string
	Filename   string
	MimeType   string
	Size       int64
	Reader     io.ReadSeeker
}
