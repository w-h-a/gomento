package v1file

import (
	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
)

type ListFilesInput struct {
	SpaceId    *uuid.UUID
	PathPrefix string
}

type ListFilesOutput struct {
	Items []v1.File `json:"items"`
}
