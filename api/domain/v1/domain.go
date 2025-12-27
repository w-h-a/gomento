package v1

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	JobTypeDistill = "distill_session"

	JobStatusPending = "pending"
	JobStatusRunning = "running"
	JobStatusSuccess = "success"
	JobStatusFailed  = "failed"
)

type Job struct {
	Id        uuid.UUID       `json:"id"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	Status    string          `json:"status"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type DistillJobPayload struct {
	SessionId uuid.UUID `json:"session_id"`
}

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
	Id        uuid.UUID  `json:"id"`
	ProjectId uuid.UUID  `json:"project_id"`
	SpaceId   *uuid.UUID `json:"space_id"`
	CreatedAt time.Time  `json:"created_at"`
}

type Message struct {
	Id        uuid.UUID  `json:"id"`
	SessionId uuid.UUID  `json:"session_id"`
	ParentId  *uuid.UUID `json:"parent_id,omitempty"`
	Role      string     `json:"role"`
	Parts     []Part     `json:"parts"`
	CreatedAt time.Time  `json:"created_at"`
}

type Part struct {
	Type    string         `json:"type"` // "text", "image", "file"
	Text    string         `json:"text,omitempty"`
	AssetId *uuid.UUID     `json:"asset_id,omitempty"`
	Meta    map[string]any `json:"meta,omitempty"`
}

type Asset struct {
	Id        uuid.UUID `json:"id"`
	Container string    `json:"container"`
	Path      string    `json:"path"`
	ETag      string    `json:"etag"`
	SHA256    string    `json:"sha256"`
	MIME      string    `json:"mime"`
	SizeBytes int64     `json:"size_bytes"`
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
