package v1session

import "errors"

var (
	ErrSessionNotFound = errors.New("session not found")
	ErrSpaceNotFound   = errors.New("space not found")
)
