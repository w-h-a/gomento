package util

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidCursor = errors.New("invalid cursor format")
)

func DecodeCursor(s string) (time.Time, uuid.UUID, error) {
	if len(s) == 0 {
		return time.Time{}, uuid.Nil, ErrInvalidCursor
	}

	bs, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return time.Time{}, uuid.Nil, ErrInvalidCursor
	}

	parts := strings.Split(string(bs), "|")
	if len(parts) != 2 {
		return time.Time{}, uuid.Nil, ErrInvalidCursor
	}

	nanos, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, uuid.Nil, ErrInvalidCursor
	}

	id, err := uuid.Parse(parts[1])
	if err != nil {
		return time.Time{}, uuid.Nil, ErrInvalidCursor
	}

	return time.Unix(0, nanos).UTC(), id, nil
}

func EncodeCursor(t time.Time, id uuid.UUID) string {
	raw := fmt.Sprintf("%d|%s", t.UTC().UnixNano(), id.String())
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}
