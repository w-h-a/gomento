package v1

import "github.com/google/uuid"

type Chat struct {
	SessionId uuid.UUID         `json:"session_id"`
	Text      string            `json:"text"`
	Files     map[string]string `json:"files"`
}
