package v1session

import (
	"mime/multipart"

	"github.com/google/uuid"
)

type SendMessageInput struct {
	SessionId uuid.UUID
	Role      string
	Parts     []PartInput
	Files     map[string]*multipart.FileHeader
}

type PartInput struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	FileField string         `json:"file_field,omitempty"`
	Meta      map[string]any `json:"meta,omitempty"`
}
