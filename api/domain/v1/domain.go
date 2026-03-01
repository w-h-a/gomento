package v1

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	JobTypeDistill    = "distill_session"
	JobTypeExtract    = "extract_session"
	JobTypeIngestFile = "ingest_file"

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

type SessionJobPayload struct {
	SessionId     uuid.UUID `json:"session_id"`
	MessageWindow int       `json:"message_window,omitempty"`
}

type IngestFileJobPayload struct {
	FileId uuid.UUID `json:"file_id"`
}

type Space struct {
	Id        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
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

type Session struct {
	Id        uuid.UUID  `json:"id"`
	SpaceId   *uuid.UUID `json:"space_id"`
	CreatedAt time.Time  `json:"created_at"`
}

const (
	TaskStatusPending = "pending"
	TaskStatusRunning = "running"
	TaskStatusSuccess = "success"
	TaskStatusFailed  = "failed"
)

type Task struct {
	Id        uuid.UUID       `json:"id"`
	SessionId uuid.UUID       `json:"session_id"`
	TaskOrder int             `json:"task_order"`
	IsThought bool            `json:"is_thought"`
	Data      json.RawMessage `json:"data"`
	Status    string          `json:"status"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type Message struct {
	Id        uuid.UUID  `json:"id"`
	SessionId uuid.UUID  `json:"session_id"`
	TaskId    *uuid.UUID `json:"task_id,omitempty"`
	ParentId  *uuid.UUID `json:"parent_id,omitempty"`
	Role      string     `json:"role"`
	Parts     []Part     `json:"parts"`
	Embedding []float32  `json:"embedding"`
	CreatedAt time.Time  `json:"created_at"`
}

type Part struct {
	Type    string         `json:"type"` // "text", "image", "file"
	Text    string         `json:"text,omitempty"`
	AssetId *uuid.UUID     `json:"asset_id,omitempty"`
	Meta    map[string]any `json:"meta,omitempty"`
}

type MatchingChunk struct {
	File  File      `json:"file"`
	Chunk FileChunk `json:"chunk"`
	Score float32   `json:"score"`
}

type File struct {
	Id        uuid.UUID       `json:"id"`
	SpaceId   *uuid.UUID      `json:"space_id"`
	AssetId   uuid.UUID       `json:"asset_id"`
	Path      string          `json:"path"`
	Filename  string          `json:"filename"`
	Meta      json.RawMessage `json:"meta"`
	Embedding []float32       `json:"embedding"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	Asset     *Asset          `json:"asset,omitempty"`
}

type FileChunk struct {
	Id         uuid.UUID `json:"id"`
	FileId     uuid.UUID `json:"file_id"`
	ChunkIndex int       `json:"chunk_index"`
	Content    string    `json:"content"`
	Embedding  []float32 `json:"embedding"`
	CreatedAt  time.Time `json:"created_at"`
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
