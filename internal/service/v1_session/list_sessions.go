package v1session

import (
	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
)

type ListSessionsInput struct {
	SpaceId *uuid.UUID
}

type ListSessionsOutput struct {
	Items []v1.Session `json:"items"`
}
