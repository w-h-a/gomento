package v1session

import (
	"time"

	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
)

type GetMessagesInput struct {
	SessionId          uuid.UUID
	Limit              int
	Cursor             string
	WithAssetPublicUrl bool
}

type GetMessagesOutput struct {
	Items      []v1.Message            `json:"items"`
	NextCursor string                  `json:"next_cursor,omitempty"`
	HasMore    bool                    `json:"has_more"`
	PublicUrls map[uuid.UUID]PublicUrl `json:"public_urls,omitempty"`
}

type PublicUrl struct {
	Url      string    `json:"url"`
	ExpireAt time.Time `json:"expire_at"`
}
