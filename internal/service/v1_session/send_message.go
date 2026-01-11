package v1session

import (
	"io"

	"github.com/google/uuid"
)

type SendMessageInput struct {
	SessionId uuid.UUID
	Role      string
	Parts     []PartInput
	Files     map[string]InputFile
}

type PartInput struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	FileField string         `json:"file_field,omitempty"`
	Meta      map[string]any `json:"meta,omitempty"`
}

type InputFile struct {
	Filename    string
	ContentType string
	Size        int64
	Reader      io.ReadSeeker
}
