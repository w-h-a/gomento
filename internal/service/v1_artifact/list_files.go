package v1artifact

import (
	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
)

type ListFilesInput struct {
	ArtifactId uuid.UUID
	PathPrefix string
}

type ListFilesOutput struct {
	Items []v1.File `json:"items"`
}
