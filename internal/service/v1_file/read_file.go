package v1file

import "github.com/google/uuid"

type ReadFileInput struct {
	FileId    uuid.UUID
	StartLine int
	EndLine   int
}
