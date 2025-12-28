package v1session

import (
	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
)

type GetTasksInput struct {
	SessionId uuid.UUID
}

type GetTasksOutput struct {
	Items []v1.Task `json:"items"`
}
