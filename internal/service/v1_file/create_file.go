package v1file

import (
	"io"

	"github.com/google/uuid"
)

type CreateFileInput struct {
	SpaceId  *uuid.UUID
	Path     string
	Filename string
	MimeType string
	Size     int64
	Reader   io.ReadSeeker
}
