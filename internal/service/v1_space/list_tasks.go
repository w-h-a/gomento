package v1space

import (
	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
)

type ListTasksInput struct {
	SpaceId uuid.UUID
	Status  *string
}

type ListTasksOutput struct {
	Items []v1.Task `json:"items"`
}
