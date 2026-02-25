package buffer

import (
	"time"

	v1 "github.com/w-h-a/gomento/api/domain/v1"
)

type BufferedMessage struct {
	Message  v1.Message
	Assets   map[int]*v1.Asset
	QueuedAt time.Time
}
