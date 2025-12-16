package v1

import (
	"time"

	"github.com/google/uuid"
)

type Project struct {
	Id        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type Space struct {
	Id        uuid.UUID `json:"id"`
	ProjectId uuid.UUID `json:"project_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type Session struct {
	Id        uuid.UUID `json:"id"`
	ProjectId uuid.UUID `json:"project_id"`
	SpaceId   uuid.UUID `json:"space_id"`
	CreatedAt time.Time `json:"created_at"`
}

type Message struct {
	Id        uuid.UUID `json:"id"`
	SessionId uuid.UUID `json:"session_id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type Skill struct {
	Id        uuid.UUID `json:"id"`
	SpaceId   uuid.UUID `json:"space_id"`
	Trigger   string    `json:"trigger"`
	SOP       string    `json:"sop"`
	Embedding []float32 `json:"embedding"`
	CreatedAt time.Time `json:"created_at"`
}

const (
	TaskTypeDistill = "distill_session"
)

type Task struct {
	Id        uuid.UUID      `json:"id"`
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload"`
	CreatedAt time.Time      `json:"created_at"`
}
